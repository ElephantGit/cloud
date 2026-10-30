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

// Why chat ended an execution session; the conversation can continue in a new one.
const (
	EndStopped     = "stopped"
	EndReleased    = "released"
	EndIdle        = "idle"
	EndDeleted     = "deleted"
	EndInterrupted = "interrupted"
)

// ChatConversation is one chat in the history column. Sessions are its execution sessions in
// order; only the last one can still be live.
type ChatConversation struct {
	ID          string        `json:"id"`
	ProjectID   string        `json:"projectId"`
	WorkspaceID string        `json:"workspaceId"`
	ExecutorID  string        `json:"executorId"`
	Title       string        `json:"title"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	Sessions    []ChatSession `json:"sessions"`
}

// Live returns the conversation's last session while chat still treats it as live.
func (c ChatConversation) Live() (ChatSession, bool) {
	if n := len(c.Sessions); n > 0 && c.Sessions[n-1].EndedAt == nil {
		return c.Sessions[n-1], true
	}
	return ChatSession{}, false
}

// ChatSession is one execution session behind a conversation. The last attempt and turn identify
// the most recent prompt, which stop and idle reclaim act on.
type ChatSession struct {
	ID            string     `json:"id"`
	Ordinal       int        `json:"ordinal"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt"`
	EndReason     string     `json:"endReason"`
	LastAttemptID string     `json:"-"`
	LastTurnID    string     `json:"-"`
	LastPromptAt  time.Time  `json:"-"`
}

// NewChatConversation is what creating a conversation records before its first admission.
type NewChatConversation struct {
	TenantID, UserID, CreateKey        string
	ProjectID, WorkspaceID, ExecutorID string
	Title                              string
}

// IdleChatSession is a live session whose last prompt is old enough to consider reclaiming.
type IdleChatSession struct {
	TenantID, UserID, ConversationID string
	Session                          ChatSession
}

// Member reports whether the user actively belongs to the active tenant.
func (s *Store) Member(ctx context.Context, tenant, user string) (bool, error) {
	return s.ScheduleMember(ctx, tenant, user)
}

const conversationColumns = `id, project_id, workspace_id, executor_id, title, created_at, updated_at`

// CreateChatConversation inserts the conversation, or returns the one an earlier request with the
// same key created. A key reused with different fields returns ErrKeyReused.
func (s *Store) CreateChatConversation(ctx context.Context, in NewChatConversation) (ChatConversation, error) {
	var c ChatConversation
	e := s.pool.QueryRow(ctx, `
		INSERT INTO v2_chat_conversations (tenant_id, user_id, create_key, project_id, workspace_id, executor_id, title)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, user_id, create_key) DO NOTHING
		RETURNING `+conversationColumns,
		in.TenantID, in.UserID, in.CreateKey, in.ProjectID, in.WorkspaceID, in.ExecutorID, in.Title).
		Scan(&c.ID, &c.ProjectID, &c.WorkspaceID, &c.ExecutorID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if e == nil {
		c.Sessions = []ChatSession{}
		return c, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return c, fmt.Errorf("create conversation: %w", e)
	}
	var id string
	if e := s.pool.QueryRow(ctx, `
		SELECT id FROM v2_chat_conversations
		WHERE tenant_id = $1 AND user_id = $2 AND create_key = $3
		  AND project_id = $4 AND workspace_id = $5 AND executor_id = $6`,
		in.TenantID, in.UserID, in.CreateKey, in.ProjectID, in.WorkspaceID, in.ExecutorID).Scan(&id); errors.Is(e, pgx.ErrNoRows) {
		return c, ErrKeyReused
	} else if e != nil {
		return c, fmt.Errorf("find conversation by key: %w", e)
	}
	return s.ChatConversation(ctx, in.TenantID, in.UserID, id)
}

// ChatConversation reads one of the user's conversations with its sessions, or ErrNotFound.
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
		SELECT c.id, c.project_id, c.workspace_id, c.executor_id, c.title, c.created_at, c.updated_at,
		       s.session_id, s.ordinal, s.started_at, s.ended_at, s.end_reason, s.last_attempt_id, s.last_turn_id, s.last_prompt_at
		FROM v2_chat_conversations c LEFT JOIN v2_chat_sessions s ON s.conversation_id = c.id
		WHERE `+where+`
		ORDER BY c.created_at, c.id, s.ordinal`, args...)
	if e != nil {
		return nil, fmt.Errorf("list conversations: %w", e)
	}
	defer rows.Close()
	out := []ChatConversation{}
	for rows.Next() {
		var c ChatConversation
		var sid, reason, attempt, turn *string
		var ordinal *int
		var started, prompted *time.Time
		var ended *time.Time
		if e := rows.Scan(&c.ID, &c.ProjectID, &c.WorkspaceID, &c.ExecutorID, &c.Title, &c.CreatedAt, &c.UpdatedAt,
			&sid, &ordinal, &started, &ended, &reason, &attempt, &turn, &prompted); e != nil {
			return nil, fmt.Errorf("list conversations: %w", e)
		}
		if n := len(out); n == 0 || out[n-1].ID != c.ID {
			c.Sessions = []ChatSession{}
			out = append(out, c)
		}
		if sid != nil {
			last := &out[len(out)-1]
			last.Sessions = append(last.Sessions, ChatSession{
				ID: *sid, Ordinal: *ordinal, StartedAt: *started, EndedAt: ended, EndReason: *reason,
				LastAttemptID: *attempt, LastTurnID: *turn, LastPromptAt: *prompted,
			})
		}
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

// DeleteChatConversation removes the conversation and its session rows. The execution history
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

// RecordChatTurn records an admitted turn. A new session ID appends the conversation's next
// session; a known one (Append, or a retried admission) moves its last attempt forward.
func (s *Store) RecordChatTurn(ctx context.Context, conversationID, sessionID, attemptID, turnID string, at time.Time) error {
	_, e := s.pool.Exec(ctx, `
		INSERT INTO v2_chat_sessions (conversation_id, ordinal, session_id, started_at, last_attempt_id, last_turn_id, last_prompt_at)
		SELECT $1, COALESCE(MAX(ordinal), 0) + 1, $2, $5, $3, $4, $5 FROM v2_chat_sessions WHERE conversation_id = $1
		ON CONFLICT (session_id) DO UPDATE SET last_attempt_id = excluded.last_attempt_id,
			last_turn_id = excluded.last_turn_id, last_prompt_at = excluded.last_prompt_at
		WHERE v2_chat_sessions.conversation_id = $1`, conversationID, sessionID, attemptID, turnID, at)
	if e != nil {
		return fmt.Errorf("record chat turn: %w", e)
	}
	_, e = s.pool.Exec(ctx, `UPDATE v2_chat_conversations SET updated_at = $2 WHERE id = $1`, conversationID, at)
	if e != nil {
		return fmt.Errorf("touch conversation: %w", e)
	}
	return nil
}

// EndChatSession records that a session ended. A reason marked while ending wins over `reason`,
// and the first recorded end wins over later ones, so a sweep cannot relabel a stop as idle.
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

// IdleChatSessions lists live sessions whose last prompt was admitted before the cutoff.
func (s *Store) IdleChatSessions(ctx context.Context, before time.Time) ([]IdleChatSession, error) {
	rows, e := s.pool.Query(ctx, `
		SELECT c.tenant_id, c.user_id, c.id, s.session_id, s.ordinal, s.started_at, s.last_attempt_id, s.last_turn_id, s.last_prompt_at
		FROM v2_chat_sessions s JOIN v2_chat_conversations c ON c.id = s.conversation_id
		WHERE s.ended_at IS NULL AND s.last_prompt_at < $1
		ORDER BY s.last_prompt_at`, before)
	if e != nil {
		return nil, fmt.Errorf("list idle chat sessions: %w", e)
	}
	defer rows.Close()
	var out []IdleChatSession
	for rows.Next() {
		var v IdleChatSession
		x := &v.Session
		if e := rows.Scan(&v.TenantID, &v.UserID, &v.ConversationID, &x.ID, &x.Ordinal, &x.StartedAt, &x.LastAttemptID, &x.LastTurnID, &x.LastPromptAt); e != nil {
			return nil, fmt.Errorf("list idle chat sessions: %w", e)
		}
		out = append(out, v)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("list idle chat sessions: %w", e)
	}
	return out, nil
}

// ChatWorkspaceOccupant finds the user's conversation whose live session targets the workspace,
// so a busy answer can name it. Other users' conversations are never disclosed.
func (s *Store) ChatWorkspaceOccupant(ctx context.Context, tenant, user, project, workspace string) (ChatConversation, error) {
	var id string
	e := s.pool.QueryRow(ctx, `
		SELECT c.id FROM v2_chat_conversations c JOIN v2_chat_sessions s ON s.conversation_id = c.id
		WHERE c.tenant_id = $1 AND c.user_id = $2 AND c.project_id = $3 AND c.workspace_id = $4 AND s.ended_at IS NULL
		ORDER BY s.last_prompt_at DESC LIMIT 1`, tenant, user, project, workspace).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return ChatConversation{}, ErrNotFound
	}
	if e != nil {
		return ChatConversation{}, fmt.Errorf("find workspace occupant: %w", e)
	}
	return s.ChatConversation(ctx, tenant, user, id)
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
