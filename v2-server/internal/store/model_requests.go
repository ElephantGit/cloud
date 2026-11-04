package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/requestmetrics"
)

// BeginModelRequest registers dispatch against the admitted session and turn, never browser IDs.
func (s *Store) BeginModelRequest(ctx context.Context, ref execution.Ref, a requestmetrics.Attempt) (requestmetrics.Attempt, error) {
	a.StartedAt = time.Now().UTC()
	a.Outcome = "started"
	raw, err := json.Marshal(a)
	if err != nil {
		return a, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO v2_model_requests(session_id,turn_id,data)
 SELECT s.id,t.id,$3 FROM v2_execution_sessions s JOIN v2_execution_turns t ON t.session_id=s.id
 WHERE s.id=$1 AND t.id=$2 AND s.ended_at IS NULL AND t.ended_at IS NULL RETURNING id::text`, ref.SessionID, ref.TurnID, raw).Scan(&a.ID)
	return a, executionError(err)
}

// FinishModelRequest replaces started once, and accepts only an identical repeated completion.
func (s *Store) FinishModelRequest(ctx context.Context, a requestmetrics.Attempt) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE v2_model_requests SET data=$2 WHERE id=$1::uuid AND (data->>'outcome'='started' OR data=$2::jsonb)`, a.ID, raw)
	if err == nil && tag.RowsAffected() != 1 {
		return execution.ErrConflict
	}
	return err
}

// DashboardRequests reads one bounded request page; membership and parent visibility are checked twice.
func (s *Store) DashboardRequests(ctx context.Context, scope execution.Scope, id, after string) ([]requestmetrics.Attempt, string, error) {
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, "", err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,data FROM v2_model_requests WHERE session_id=$1 AND ($2='' OR (created_at,id)>(SELECT created_at,id FROM v2_model_requests WHERE id::text=$2 AND session_id=$1)) ORDER BY created_at,id LIMIT 26`, id, after)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []requestmetrics.Attempt{}
	for rows.Next() {
		var a requestmetrics.Attempt
		var raw []byte
		var key string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, "", err
		}
		a.ID = key
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > 25 {
		items = items[:25]
		next = items[24].ID
	}
	if after != "" && len(items) == 0 {
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v2_model_requests WHERE session_id=$1 AND id::text=$2)`, id, after).Scan(&exists)
		if err != nil {
			return nil, "", err
		}
		if !exists {
			return nil, "", execution.ErrInvalidRequest
		}
	}
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

// DashboardStatistics aggregates all attempts in SQL, independently of the displayed request page.
func (s *Store) DashboardStatistics(ctx context.Context, scope execution.Scope, id string) (map[string]any, error) {
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, err
	}
	// Counts, sums and field coverage must describe the same set of attempts during a live run.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out := map[string]any{"schemaVersion": 1, "source": "model_gateway", "scope": "recorded_http_attempts", "inputSemantics": "unknown"}
	counts := map[string]any{}
	var total int64
	var first, last *time.Time
	if err := tx.QueryRow(ctx, `SELECT count(*),min(created_at),max(created_at) FROM v2_model_requests WHERE session_id=$1`, id).Scan(&total, &first, &last); err != nil {
		return nil, err
	}
	out["firstRecordedAt"], out["lastRecordedAt"] = first, last
	counts["total"] = total
	for _, state := range []string{"started", "succeeded", "failed", "cancelled", "interrupted", "incomplete"} {
		var n int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM v2_model_requests WHERE session_id=$1 AND data->>'outcome'=$2`, id, state).Scan(&n); err != nil {
			return nil, err
		}
		counts[state] = n
	}
	out["counts"] = counts
	for _, group := range []string{"usage", "timing"} {
		values := map[string]any{}
		fields := []string{"inputTokens", "outputTokens", "totalTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens"}
		if group == "timing" {
			fields = []string{"meanDurationMs", "p90DurationMs", "meanFirstOutputMs"}
		}
		for _, field := range fields {
			expression := "data->'usage'->>'" + field + "'"
			aggregate := "sum(v)"
			if group == "timing" {
				expression = "data->>'durationMs'"
				aggregate = "avg(v)"
				if field == "meanFirstOutputMs" {
					expression = "data->>'firstOutputMs'"
				}
				if field == "p90DurationMs" {
					aggregate = "percentile_disc(0.9) WITHIN GROUP(ORDER BY v)"
				}
			}
			var known, final int64
			var value *float64
			sql := `SELECT count(v),count(v) FILTER(WHERE data->>'terminalObserved'='true' AND data->'usage'->>'availability'='complete'),CASE WHEN ` + aggregate + `<=9007199254740991 THEN (` + aggregate + `)::float8 ELSE NULL END FROM (SELECT data,(` + expression + `)::numeric v FROM v2_model_requests WHERE session_id=$1) x`
			if err := tx.QueryRow(ctx, sql, id).Scan(&known, &final, &value); err != nil {
				return nil, err
			}
			availability := "unavailable"
			if value != nil && known > 0 {
				availability = "partial"
				if known == total && (group == "timing" || final == total) {
					availability = "complete"
				}
			}
			values[field] = map[string]any{"value": value, "knownCount": known, "totalCount": total, "availability": availability}
		}
		out[group] = values
	}
	var semantics *string
	err = tx.QueryRow(ctx, `SELECT CASE WHEN count(DISTINCT data->'usage'->>'inputSemantics')=1 THEN min(data->'usage'->>'inputSemantics') END FROM v2_model_requests WHERE session_id=$1`, id).Scan(&semantics)
	if err != nil {
		return nil, err
	}
	if semantics != nil {
		out["inputSemantics"] = *semantics
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, err
	}
	return out, nil
}
