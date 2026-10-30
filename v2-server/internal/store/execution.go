package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

type executionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CheckExecutionScope requires a current membership and active tenant and user.
// Missing and inaccessible identities deliberately share ErrNotFound.
func (s *Store) CheckExecutionScope(ctx context.Context, scope execution.Scope) error {
	return checkExecutionScope(ctx, s.pool, scope)
}

func checkExecutionScope(ctx context.Context, q executionQuerier, scope execution.Scope) error {
	var allowed bool
	err := q.QueryRow(ctx, `SELECT EXISTS (
	 SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id JOIN tenants t ON t.id=m.tenant_id
	 WHERE m.tenant_id::text=$1 AND m.user_id::text=$2
	 AND u.status='active' AND u.deleted_at IS NULL AND t.status='active')`, scope.TenantID, scope.UserID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return execution.ErrNotFound
	}
	return nil
}

func executionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return execution.ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return execution.ErrConflict
	}
	return err
}

func executionCaller(caller execution.Caller) ([]byte, error) {
	if caller.ID == "" || caller.AttemptID == "" || (caller.Kind != execution.CallerChat && caller.Kind != execution.CallerWorkflow && caller.Kind != execution.CallerSchedule) {
		return nil, execution.ErrInvalidRequest
	}
	if len(caller.Rounds) == 0 {
		caller.Rounds = nil
	}
	for _, round := range caller.Rounds {
		if round.ContainerID == "" || round.Index < 0 {
			return nil, execution.ErrInvalidRequest
		}
	}
	return json.Marshal(caller)
}

// CreateExecutionSession atomically retains a session and its first admitted turn.
// Duplicate caller identities return ErrConflict; the runtime resolves retries before dispatch.
func (s *Store) CreateExecutionSession(ctx context.Context, scope execution.Scope, session execution.Session, first execution.Turn) error {
	if session.ID == "" || session.EndedAt != nil || first.Ref.SessionID != session.ID {
		return execution.ErrInvalidRequest
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := checkExecutionScope(ctx, tx, scope); err != nil {
			return err
		}
		if session.CreatedAt.IsZero() {
			if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&session.CreatedAt); err != nil {
				return err
			}
		}
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v2_execution_sessions(id,tenant_id,user_id,data) VALUES($1,$2,$3,$4)`, session.ID, scope.TenantID, scope.UserID, data); err != nil {
			return err
		}
		return insertExecutionTurn(ctx, tx, scope, session.ID, first)
	})
	return executionError(err)
}

func insertExecutionTurn(ctx context.Context, tx pgx.Tx, scope execution.Scope, sid string, turn execution.Turn) error {
	if turn.Ref.SessionID != sid || turn.Ref.TurnID == "" || turn.Result != nil || turn.EndedAt != nil {
		return execution.ErrInvalidRequest
	}
	caller, err := executionCaller(turn.Caller)
	if err != nil {
		return err
	}
	if _, err := turn.Prompt.Text(); err != nil {
		return err
	}
	var position int64
	var now time.Time
	if err := tx.QueryRow(ctx, `UPDATE v2_execution_sessions SET turn_position=turn_position+1 WHERE id=$1 RETURNING turn_position,now()`, sid).Scan(&position, &now); err != nil {
		return err
	}
	if turn.StartedAt.IsZero() {
		turn.StartedAt = now
	}
	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v2_execution_turns(id,session_id,tenant_id,user_id,caller,position,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, turn.Ref.TurnID, sid, scope.TenantID, scope.UserID, caller, position, data)
	return err
}

// AppendExecutionTurn records a new turn only while the owning session remains live.
func (s *Store) AppendExecutionTurn(ctx context.Context, scope execution.Scope, sid string, turn execution.Turn) error {
	return s.executionSessionTransaction(ctx, scope, sid, func(tx pgx.Tx, session execution.Session) error {
		if session.EndedAt != nil {
			return execution.ErrSessionEnded
		}
		return insertExecutionTurn(ctx, tx, scope, sid, turn)
	})
}

func (s *Store) executionSessionTransaction(ctx context.Context, scope execution.Scope, sid string, fn func(pgx.Tx, execution.Session) error) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := checkExecutionScope(ctx, tx, scope); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT data FROM v2_execution_sessions WHERE id=$1 AND tenant_id::text=$2 AND user_id::text=$3 FOR UPDATE`, sid, scope.TenantID, scope.UserID).Scan(&raw); err != nil {
			return err
		}
		var session execution.Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		return fn(tx, session)
	})
	return executionError(err)
}

// ReadExecutionSession returns only the authenticated owner's retained metadata.
func (s *Store) ReadExecutionSession(ctx context.Context, scope execution.Scope, sid string) (execution.Session, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return execution.Session{}, err
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT data FROM v2_execution_sessions WHERE id=$1 AND tenant_id::text=$2 AND user_id::text=$3`, sid, scope.TenantID, scope.UserID).Scan(&raw)
	if err != nil {
		return execution.Session{}, executionError(err)
	}
	var session execution.Session
	err = json.Unmarshal(raw, &session)
	return session, err
}

type executionTurnCursor struct {
	SessionID string
	Position  int64
}

// ReadExecutionTurns reads stable admission order with a session-bound opaque cursor.
func (s *Store) ReadExecutionTurns(ctx context.Context, scope execution.Scope, sid, cursor string, limit int) (execution.TurnPage, error) {
	page := execution.TurnPage{Items: []execution.Turn{}}
	if limit < 1 || limit > 100 {
		return page, execution.ErrInvalidRequest
	}
	if _, err := s.ReadExecutionSession(ctx, scope, sid); err != nil {
		return page, err
	}
	position := int64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		var c executionTurnCursor
		if err != nil || json.Unmarshal(raw, &c) != nil || c.SessionID != sid || c.Position < 1 {
			return page, execution.ErrInvalidRequest
		}
		position = c.Position
	}
	rows, err := s.pool.Query(ctx, `SELECT position,data FROM v2_execution_turns WHERE session_id=$1 AND position>$2 ORDER BY position LIMIT $3`, sid, position, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		if len(page.Items) == limit {
			raw, _ := json.Marshal(executionTurnCursor{SessionID: sid, Position: last})
			page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
			break
		}
		var raw []byte
		if err := rows.Scan(&last, &raw); err != nil {
			return page, err
		}
		var turn execution.Turn
		if err := json.Unmarshal(raw, &turn); err != nil {
			return page, err
		}
		normalizeExecutionResult(turn.Result)
		page.Items = append(page.Items, turn)
	}
	return page, rows.Err()
}

// ReadExecutionAttempt resolves the complete Caller identity within the authenticated scope.
func (s *Store) ReadExecutionAttempt(ctx context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return execution.Turn{}, err
	}
	key, err := executionCaller(caller)
	if err != nil {
		return execution.Turn{}, err
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT data FROM v2_execution_turns WHERE tenant_id::text=$1 AND user_id::text=$2 AND caller=$3::jsonb`, scope.TenantID, scope.UserID, key).Scan(&raw); err != nil {
		return execution.Turn{}, executionError(err)
	}
	var turn execution.Turn
	err = json.Unmarshal(raw, &turn)
	normalizeExecutionResult(turn.Result)
	return turn, err
}

// RawMessage distinguishes a nil value from the JSON literal null in Go. Both mean
// unavailable usage at this boundary, so retained results expose the same value as live ones.
func normalizeExecutionResult(result *execution.Result) {
	if result != nil && bytes.Equal(bytes.TrimSpace(result.Usage), []byte("null")) {
		result.Usage = nil
	}
}
