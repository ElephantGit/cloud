package store

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// ReadExecutionCursor returns the latest committed event position after scope/owner checks.
// A concurrent event may advance it immediately; callers reject only cursors ahead of this value.
func (s *Store) ReadExecutionCursor(ctx context.Context, scope execution.Scope, sid string) (uint64, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return 0, err
	}
	var sequence uint64
	err := s.pool.QueryRow(ctx, `SELECT event_sequence FROM v2_execution_sessions WHERE id=$1 AND tenant_id=$2::uuid AND user_id=$3::uuid`, sid, scope.TenantID, scope.UserID).Scan(&sequence)
	return sequence, executionError(err)
}

// ReadExecutionEvents returns retained events after a session-wide sequence. An ahead cursor
// is invalid. The runtime follows this durable cursor; notifications are only wake-up hints.
func (s *Store) ReadExecutionEvents(ctx context.Context, scope execution.Scope, sid string, after uint64, limit int) ([]execution.Event, error) {
	events := []execution.Event{}
	if after > math.MaxInt64 || limit < 1 || limit > 1000 {
		return nil, execution.ErrInvalidRequest
	}
	// A read-only snapshot keeps cursor validation and event rows consistent without
	// taking the session lock needed by writers and terminal result commits.
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := checkExecutionScope(ctx, tx, scope); err != nil {
			return err
		}
		var current uint64
		if err := tx.QueryRow(ctx, `SELECT event_sequence FROM v2_execution_sessions WHERE id=$1 AND tenant_id=$2::uuid AND user_id=$3::uuid`, sid, scope.TenantID, scope.UserID).Scan(&current); err != nil {
			return err
		}
		if after > current {
			return execution.ErrInvalidRequest
		}
		rows, err := tx.Query(ctx, `SELECT data FROM v2_execution_events WHERE session_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`, sid, int64(after), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var event execution.Event
			if err := json.Unmarshal(raw, &event); err != nil {
				return err
			}
			normalizeExecutionResult(event.Result)
			events = append(events, event)
		}
		return rows.Err()
	})
	return events, executionError(err)
}

// AppendExecutionEvent retains a nonterminal event, assigning its gap-free session sequence.
// Terminal events must use FinishExecutionTurn/EndExecutionSession to update state atomically.
// This runtime write checks admitted ownership, even when current membership has been revoked.
func (s *Store) AppendExecutionEvent(ctx context.Context, scope execution.Scope, event execution.Event) (execution.Event, error) {
	if event.Result != nil || (event.Kind != execution.EventTextDelta && event.Kind != execution.EventHistorySnapshot) {
		return execution.Event{}, execution.ErrInvalidRequest
	}
	err := s.executionRuntimeTransaction(ctx, scope, event.Ref.SessionID, func(tx pgx.Tx, session execution.Session) error {
		if session.EndedAt != nil {
			return execution.ErrSessionEnded
		}
		if event.Ref.TurnID != "" {
			turn, err := readExecutionTurn(ctx, tx, event.Ref)
			if err != nil {
				return err
			}
			if turn.Result != nil && event.Kind == execution.EventTextDelta {
				return execution.ErrConflict
			}
		} else if event.Kind == execution.EventTextDelta {
			return execution.ErrInvalidRequest
		}
		var err error
		event, err = insertExecutionEvent(ctx, tx, event)
		return err
	})
	return event, err
}

func readExecutionTurn(ctx context.Context, tx pgx.Tx, ref execution.Ref) (execution.Turn, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT data FROM v2_execution_turns WHERE id=$1 AND session_id=$2`, ref.TurnID, ref.SessionID).Scan(&raw); err != nil {
		return execution.Turn{}, executionError(err)
	}
	var turn execution.Turn
	err := json.Unmarshal(raw, &turn)
	normalizeExecutionResult(turn.Result)
	return turn, err
}

// The caller holds the session row lock. Counter allocation rolls back with the event.
func insertExecutionEvent(ctx context.Context, tx pgx.Tx, event execution.Event) (execution.Event, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `UPDATE v2_execution_sessions SET event_sequence=event_sequence+1 WHERE id=$1 RETURNING event_sequence,now()`, event.Ref.SessionID).Scan(&event.Sequence, &now); err != nil {
		return event, err
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return event, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v2_execution_events(session_id,sequence,data) VALUES($1,$2,$3)`, event.Ref.SessionID, int64(event.Sequence), raw)
	return event, err
}

// FinishExecutionTurn commits a result and its terminal event together. The first terminal
// result wins; cancellation or cleanup racing a completed result cannot rewrite it.
// This runtime write checks admitted ownership, even when current membership has been revoked.
func (s *Store) FinishExecutionTurn(ctx context.Context, scope execution.Scope, ref execution.Ref, result execution.Result) (execution.Result, error) {
	var actual execution.Result
	err := s.executionRuntimeTransaction(ctx, scope, ref.SessionID, func(tx pgx.Tx, _ execution.Session) error {
		var err error
		actual, err = finishExecutionTurn(ctx, tx, ref, result)
		return err
	})
	return actual, err
}

func finishExecutionTurn(ctx context.Context, tx pgx.Tx, ref execution.Ref, result execution.Result) (execution.Result, error) {
	turn, err := readExecutionTurn(ctx, tx, ref)
	if err != nil {
		return execution.Result{}, err
	}
	if turn.Result != nil {
		return *turn.Result, nil
	}
	normalizeExecutionResult(&result)
	switch result.Status {
	case execution.StatusCompleted:
		if !result.AnswerComplete {
			return execution.Result{}, execution.ErrInvalidRequest
		}
	case execution.StatusCancelled, execution.StatusFailed, execution.StatusInterrupted:
	default:
		return execution.Result{}, execution.ErrInvalidRequest
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return execution.Result{}, err
	}
	turn.Result, turn.EndedAt = &result, &now
	raw, err := json.Marshal(turn)
	if err != nil {
		return execution.Result{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE v2_execution_turns SET data=$2,ended_at=$3 WHERE id=$1`, ref.TurnID, raw, now); err != nil {
		return execution.Result{}, err
	}
	_, err = insertExecutionEvent(ctx, tx, execution.Event{Ref: ref, Kind: execution.EventTurnEnded, OccurredAt: now, Result: &result})
	return result, err
}

// EndExecutionSession marks a quiescent session ended and retains its final event once.
// The runtime must stop and finish active work first; SQL never claims execution has stopped.
// This runtime write checks admitted ownership, even when current membership has been revoked.
func (s *Store) EndExecutionSession(ctx context.Context, scope execution.Scope, sid string) error {
	return s.executionRuntimeTransaction(ctx, scope, sid, func(tx pgx.Tx, session execution.Session) error {
		return endExecutionSession(ctx, tx, session)
	})
}

func endExecutionSession(ctx context.Context, tx pgx.Tx, session execution.Session) error {
	if session.EndedAt != nil {
		return nil
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v2_execution_turns WHERE session_id=$1 AND ended_at IS NULL)`, session.ID).Scan(&active); err != nil {
		return err
	}
	if active {
		return execution.ErrConflict
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return err
	}
	session.EndedAt = &now
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE v2_execution_sessions SET data=$2,ended_at=$3 WHERE id=$1`, session.ID, raw, now); err != nil {
		return err
	}
	_, err = insertExecutionEvent(ctx, tx, execution.Event{Ref: execution.Ref{SessionID: session.ID}, Kind: execution.EventSessionEnded, OccurredAt: now})
	return err
}

// RecoverExecutions runs before admission and reads are exposed by the single-process runtime.
// It ends old live sessions without replaying prompts and never rewrites completed results.
// Process and filesystem cleanup, when applicable, belongs to the runtime before this call.
func (s *Store) RecoverExecutions(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT data FROM v2_execution_sessions WHERE ended_at IS NULL ORDER BY id FOR UPDATE`)
		if err != nil {
			return err
		}
		var sessions []execution.Session
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			var session execution.Session
			if err := json.Unmarshal(raw, &session); err != nil {
				rows.Close()
				return err
			}
			sessions = append(sessions, session)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, session := range sessions {
			var tid string
			err := tx.QueryRow(ctx, `SELECT id FROM v2_execution_turns WHERE session_id=$1 AND ended_at IS NULL`, session.ID).Scan(&tid)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil {
				var partial string
				if err := tx.QueryRow(ctx, `SELECT COALESCE(string_agg(data->>'TextDelta','' ORDER BY sequence),'') FROM v2_execution_events WHERE session_id=$1 AND data->>'Kind'='text_delta' AND data#>>'{Ref,TurnID}'=$2`, session.ID, tid).Scan(&partial); err != nil {
					return err
				}
				// A server restart supplies no protocol stop reason.
				result := execution.Result{Status: execution.StatusInterrupted, Answer: partial, FailureCode: "server_restarted"}
				if _, err := finishExecutionTurn(ctx, tx, execution.Ref{SessionID: session.ID, TurnID: tid}, result); err != nil {
					return err
				}
			}
			if err := endExecutionSession(ctx, tx, session); err != nil {
				return err
			}
		}
		return nil
	})
}
