package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrKeyReused reports an idempotency key already used for a different chat request.
var ErrKeyReused = errors.New("idempotency key reused")

// Why a conversation's execution session ended. Either way the conversation is read-only from
// then on: the fixed Node cannot resume an ended session, and chat never starts a replacement.
const (
	EndStopped     = "stopped"
	EndDeleted     = "deleted"
	EndInterrupted = "interrupted"
)

// ChatConversation is one chat in the history column. Session is nil only until the first turn
// is accepted; a conversation has exactly one execution session for its whole life.
type ChatConversation struct {
	ID         string       `json:"id"`
	ProjectID  string       `json:"projectId"`
	ExecutorID string       `json:"executorId"`
	Title      string       `json:"title"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	Session    *ChatSession `json:"session"`
}

// Live returns the conversation's session while it can still take turns.
func (c ChatConversation) Live() (ChatSession, bool) {
	if c.Session != nil && c.Session.EndedAt == nil {
		return *c.Session, true
	}
	return ChatSession{}, false
}

// Ended reports whether the conversation's session has ended, making it read-only.
func (c ChatConversation) Ended() bool { return c.Session != nil && c.Session.EndedAt != nil }

// ChatSession is the execution session behind a conversation. The last turn identifies the most
// recent prompt, which stop acts on.
type ChatSession struct {
	ID            string     `json:"id"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt"`
	EndReason     string     `json:"endReason"`
	LastAttemptID string     `json:"-"`
	LastTurnID    string     `json:"-"`
}

// NewChatConversation is what creating a conversation records before its first turn.
type NewChatConversation struct {
	TenantID, UserID, CreateKey string
	ProjectID, ExecutorID       string
	Title                       string
}

// Member reports whether the user actively belongs to the active tenant.
func (s *Store) Member(ctx context.Context, tenant, user string) (bool, error) {
	return s.ScheduleMember(ctx, tenant, user)
}

const conversationColumns = `id, project_id, executor_id, title, created_at, updated_at`

// CreateChatConversation inserts the conversation, or returns the one an earlier request with the
// same key created. A key reused with different fields returns ErrKeyReused.
func (s *Store) CreateChatConversation(ctx context.Context, in NewChatConversation) (ChatConversation, error) {
	var c ChatConversation
	e := s.pool.QueryRow(ctx, `
		INSERT INTO v2_chat_conversations (tenant_id, user_id, create_key, project_id, executor_id, title)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, user_id, create_key) DO NOTHING
		RETURNING `+conversationColumns,
		in.TenantID, in.UserID, in.CreateKey, in.ProjectID, in.ExecutorID, in.Title).
		Scan(&c.ID, &c.ProjectID, &c.ExecutorID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if e == nil {
		return c, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return c, fmt.Errorf("create conversation: %w", e)
	}
	var id string
	if e := s.pool.QueryRow(ctx, `
		SELECT id FROM v2_chat_conversations
		WHERE tenant_id = $1 AND user_id = $2 AND create_key = $3 AND project_id = $4 AND executor_id = $5`,
		in.TenantID, in.UserID, in.CreateKey, in.ProjectID, in.ExecutorID).Scan(&id); errors.Is(e, pgx.ErrNoRows) {
		return c, ErrKeyReused
	} else if e != nil {
		return c, fmt.Errorf("find conversation by key: %w", e)
	}
	return s.ChatConversation(ctx, in.TenantID, in.UserID, id)
}

// ChatConversation reads one of the user's conversations with its session, or ErrNotFound.
func (s *Store) ChatConversation(ctx context.Context, tenant, user, id string) (ChatConversation, error) {
	list, e := s.chatConversations(ctx, `c.tenant_id = $1 AND c.user_id = $2 AND c.id = $3`, tenant, user, id)
	if e != nil {
		return ChatConversation{}, e
	}
	if len(list) == 0 {
		return ChatConversation{}, ErrNotFound
	}
	return list[0], nil
}

// ChatConversations lists the user's conversations in the tenant, oldest first.
func (s *Store) ChatConversations(ctx context.Context, tenant, user string) ([]ChatConversation, error) {
	return s.chatConversations(ctx, `c.tenant_id = $1 AND c.user_id = $2`, tenant, user)
}

func (s *Store) chatConversations(ctx context.Context, where string, args ...any) ([]ChatConversation, error) {
	rows, e := s.pool.Query(ctx, `
		SELECT c.id, c.project_id, c.executor_id, c.title, c.created_at, c.updated_at,
		       s.session_id, s.started_at, s.ended_at, s.end_reason, s.last_attempt_id, s.last_turn_id
		FROM v2_chat_conversations c LEFT JOIN v2_chat_sessions s ON s.conversation_id = c.id
		WHERE `+where+`
		ORDER BY c.created_at, c.id`, args...)
	if e != nil {
		return nil, fmt.Errorf("list conversations: %w", e)
	}
	defer rows.Close()
	out := []ChatConversation{}
	for rows.Next() {
		var c ChatConversation
		var sid, reason, attempt, turn *string
		var started, ended *time.Time
		if e := rows.Scan(&c.ID, &c.ProjectID, &c.ExecutorID, &c.Title, &c.CreatedAt, &c.UpdatedAt,
			&sid, &started, &ended, &reason, &attempt, &turn); e != nil {
			return nil, fmt.Errorf("list conversations: %w", e)
		}
		if sid != nil {
			c.Session = &ChatSession{
				ID: *sid, StartedAt: *started, EndedAt: ended, EndReason: *reason,
				LastAttemptID: *attempt, LastTurnID: *turn,
			}
		}
		out = append(out, c)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("list conversations: %w", e)
	}
	return out, nil
}

// RenameChatConversation changes the title of one of the user's conversations.
func (s *Store) RenameChatConversation(ctx context.Context, tenant, user, id, title string) error {
	tag, e := s.pool.Exec(ctx, `
		UPDATE v2_chat_conversations SET title = $4, updated_at = now()
		WHERE tenant_id = $1 AND user_id = $2 AND id = $3`, tenant, user, id, title)
	if e != nil {
		return fmt.Errorf("rename conversation: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteChatConversation removes the conversation and its session row. The execution history
// stays with the execution service; only the page's record of it goes.
func (s *Store) DeleteChatConversation(ctx context.Context, tenant, user, id string) error {
	tag, e := s.pool.Exec(ctx, `DELETE FROM v2_chat_conversations WHERE tenant_id = $1 AND user_id = $2 AND id = $3`, tenant, user, id)
	if e != nil {
		return fmt.Errorf("delete conversation: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrSessionMismatch reports a turn recorded against a session other than the conversation's
// own: a conversation never gets a second session.
var ErrSessionMismatch = errors.New("conversation already has another session")

// RecordChatTurn records an accepted turn. The first one binds the conversation to its session;
// later ones (Append, or a retried Start) move its last attempt forward.
func (s *Store) RecordChatTurn(ctx context.Context, conversationID, sessionID, attemptID, turnID string, at time.Time) error {
	tag, e := s.pool.Exec(ctx, `
		INSERT INTO v2_chat_sessions (conversation_id, session_id, started_at, last_attempt_id, last_turn_id)
		VALUES ($1, $2, $5, $3, $4)
		ON CONFLICT (conversation_id) DO UPDATE SET last_attempt_id = excluded.last_attempt_id,
			last_turn_id = excluded.last_turn_id
		WHERE v2_chat_sessions.session_id = excluded.session_id`, conversationID, sessionID, attemptID, turnID, at)
	if e != nil {
		return fmt.Errorf("record chat turn: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionMismatch
	}
	_, e = s.pool.Exec(ctx, `UPDATE v2_chat_conversations SET updated_at = $2 WHERE id = $1`, conversationID, at)
	if e != nil {
		return fmt.Errorf("touch conversation: %w", e)
	}
	return nil
}

// EndChatSession records that a session ended. A reason marked while ending wins over `reason`,
// and the first recorded end wins over later ones, so a restart cannot relabel a stop.
func (s *Store) EndChatSession(ctx context.Context, sessionID, reason string, at time.Time) error {
	_, e := s.pool.Exec(ctx, `
		UPDATE v2_chat_sessions SET ended_at = $3, end_reason = COALESCE(NULLIF(end_reason, ''), $2)
		WHERE session_id = $1 AND ended_at IS NULL`, sessionID, reason, at)
	if e != nil {
		return fmt.Errorf("end chat session: %w", e)
	}
	return nil
}

// MarkChatSessionEnding records why chat is about to end a live session, before it asks the
// execution port to, so the reason is readable as soon as the session_ended event is.
func (s *Store) MarkChatSessionEnding(ctx context.Context, sessionID, reason string) error {
	if _, e := s.pool.Exec(ctx, `UPDATE v2_chat_sessions SET end_reason = $2 WHERE session_id = $1 AND ended_at IS NULL`, sessionID, reason); e != nil {
		return fmt.Errorf("mark chat session ending: %w", e)
	}
	return nil
}

// ClearChatSessionEnding withdraws an end reason when the port did not end the session.
func (s *Store) ClearChatSessionEnding(ctx context.Context, sessionID string) error {
	if _, e := s.pool.Exec(ctx, `UPDATE v2_chat_sessions SET end_reason = '' WHERE session_id = $1 AND ended_at IS NULL`, sessionID); e != nil {
		return fmt.Errorf("clear chat session ending: %w", e)
	}
	return nil
}

// ChatProjectOrder returns the user's saved project order; none saved is an empty list.
func (s *Store) ChatProjectOrder(ctx context.Context, tenant, user string) ([]string, error) {
	ids := []string{}
	e := s.pool.QueryRow(ctx, `SELECT project_ids FROM v2_chat_project_order WHERE tenant_id = $1 AND user_id = $2`, tenant, user).Scan(&ids)
	if errors.Is(e, pgx.ErrNoRows) {
		return []string{}, nil
	}
	if e != nil {
		return nil, fmt.Errorf("read project order: %w", e)
	}
	return ids, nil
}

// SetChatProjectOrder replaces the user's project order.
func (s *Store) SetChatProjectOrder(ctx context.Context, tenant, user string, ids []string) error {
	_, e := s.pool.Exec(ctx, `
		INSERT INTO v2_chat_project_order (tenant_id, user_id, project_ids) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO UPDATE SET project_ids = excluded.project_ids`, tenant, user, ids)
	if e != nil {
		return fmt.Errorf("save project order: %w", e)
	}
	return nil
}
