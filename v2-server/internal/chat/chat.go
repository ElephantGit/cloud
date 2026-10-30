// Package chat is the chat page's domain: conversations, the execution sessions behind them, and
// when those sessions end. It runs Agents only through the shared execution port and owns titles,
// ordering, deletion and idle reclaim, which the port leaves to pages.
//
// A conversation outlives its sessions. The fixed Node holds the workspace from Start until End,
// so a session ends when the person stops a turn, when it sits idle, when the conversation is
// deleted, or when the server restarts; the next prompt starts a new session in the same place.
// The new session does not inherit the Agent's context, and the page marks the boundary.
package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ErrNoActiveTurn reports a stop or release with no live session to act on.
var ErrNoActiveTurn = errors.New("no active turn")

// BusyError is a workspace-busy refusal. Occupant is the person's own conversation holding the
// workspace when chat knows it; otherwise another caller (or another person) holds it.
type BusyError struct{ Occupant *store.ChatConversation }

func (e *BusyError) Error() string { return execution.ErrWorkspaceBusy.Error() }

// Unwrap lets callers match the port's error.
func (e *BusyError) Unwrap() error { return execution.ErrWorkspaceBusy }

// Store is the persistence chat needs; *store.Store implements it.
type Store interface {
	CreateChatConversation(context.Context, store.NewChatConversation) (store.ChatConversation, error)
	ChatConversation(ctx context.Context, tenant, user, id string) (store.ChatConversation, error)
	ChatConversations(ctx context.Context, tenant, user string) ([]store.ChatConversation, error)
	RenameChatConversation(ctx context.Context, tenant, user, id, title string) error
	DeleteChatConversation(ctx context.Context, tenant, user, id string) error
	RecordChatTurn(ctx context.Context, conversationID, sessionID, attemptID, turnID string, at time.Time) error
	EndChatSession(ctx context.Context, sessionID, reason string, at time.Time) error
	IdleChatSessions(ctx context.Context, before time.Time) ([]store.IdleChatSession, error)
	ChatWorkspaceOccupant(ctx context.Context, tenant, user, project, workspace string) (store.ChatConversation, error)
}

// Runtime is the shared execution port and its catalog.
type Runtime interface {
	execution.Service
	execution.Catalog
}

// Options tune a Service.
type Options struct {
	// IdleAfter is how long a session may sit between turns before chat ends it to free the
	// workspace.
	IdleAfter time.Duration
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service coordinates conversations with the execution port.
type Service struct {
	store   Store
	runtime Runtime
	idle    time.Duration
	now     func() time.Time
	// locks serialize work on one conversation, so two prompts cannot both start a session and a
	// sweep cannot end a session a prompt is about to append to. v2-server is a single process,
	// which is what makes an in-memory lock sufficient.
	locks sync.Map
}

// New returns a Service. IdleAfter must be positive.
func New(s Store, r Runtime, o Options) (*Service, error) {
	if o.IdleAfter <= 0 {
		return nil, errors.New("chat: IdleAfter must be positive")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{store: s, runtime: r, idle: o.IdleAfter, now: o.Now}, nil
}

func (s *Service) lock(id string) func() {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Executors lists the Agent and model bindings a chat can start with.
func (s *Service) Executors(ctx context.Context, scope execution.Scope) ([]execution.Executor, error) {
	return s.runtime.ListExecutors(ctx, scope)
}

// Conversations lists the person's conversations.
func (s *Service) Conversations(ctx context.Context, scope execution.Scope) ([]store.ChatConversation, error) {
	return s.store.ChatConversations(ctx, scope.TenantID, scope.UserID)
}

// CreateRequest starts a conversation with its first prompt. Key is the request's idempotency
// key; it becomes the first turn's attempt ID, so a retry with the same key finds the same
// conversation and admission.
type CreateRequest struct {
	Key         string
	ProjectID   string
	WorkspaceID string
	ExecutorID  string
	Title       string
	Prompt      string
}

// Create records the conversation and admits its first turn. A conversation whose first turn is
// definitely refused is removed again, so the history never shows an empty chat; one whose
// admission is unknown stays, for the retry with the same key to settle.
func (s *Service) Create(ctx context.Context, scope execution.Scope, in CreateRequest) (store.ChatConversation, execution.Ref, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = defaultTitle(in.Prompt)
	}
	c, e := s.store.CreateChatConversation(ctx, store.NewChatConversation{
		TenantID: scope.TenantID, UserID: scope.UserID, CreateKey: in.Key,
		ProjectID: in.ProjectID, WorkspaceID: in.WorkspaceID, ExecutorID: in.ExecutorID, Title: title,
	})
	if e != nil {
		return c, execution.Ref{}, e
	}
	defer s.lock(c.ID)()
	ref, e := s.start(ctx, scope, c, in.Key, in.Prompt)
	if e != nil {
		if len(c.Sessions) == 0 && !uncertain(e) {
			if de := s.store.DeleteChatConversation(context.WithoutCancel(ctx), scope.TenantID, scope.UserID, c.ID); de != nil {
				return c, ref, errors.Join(e, de)
			}
		}
		return c, ref, e
	}
	c, e = s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, c.ID)
	return c, ref, e
}

// Send admits the next prompt: appended to the live session, or in a new session when the last
// one has ended. Key is the request's idempotency key and the turn's attempt ID.
func (s *Service) Send(ctx context.Context, scope execution.Scope, conversationID, key, prompt string) (store.ChatConversation, execution.Ref, error) {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return c, execution.Ref{}, e
	}
	var ref execution.Ref
	if live, ok := c.Live(); ok {
		ref, e = s.runtime.Append(ctx, scope, execution.AppendRequest{
			SessionID: live.ID, Caller: caller(c.ID, key), Prompt: execution.TextPrompt(prompt),
		})
		switch {
		case errors.Is(e, execution.ErrSessionEnded):
			// Ended outside chat (a restart): record it and continue in a new session.
			if e := s.store.EndChatSession(ctx, live.ID, store.EndInterrupted, s.now()); e != nil {
				return c, ref, e
			}
			ref, e = s.start(ctx, scope, c, key, prompt)
		case e == nil:
			e = s.store.RecordChatTurn(ctx, c.ID, ref.SessionID, key, ref.TurnID, s.now())
		}
	} else {
		ref, e = s.start(ctx, scope, c, key, prompt)
	}
	if e != nil {
		return c, ref, e
	}
	c, e = s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, c.ID)
	return c, ref, e
}

// start admits a prompt in a new session at the conversation's target. The caller holds the lock.
func (s *Service) start(ctx context.Context, scope execution.Scope, c store.ChatConversation, key, prompt string) (execution.Ref, error) {
	ref, e := s.runtime.Start(ctx, scope, execution.StartRequest{
		Caller:     caller(c.ID, key),
		Target:     execution.Target{ProjectID: c.ProjectID, WorkspaceID: c.WorkspaceID},
		ExecutorID: c.ExecutorID,
		Prompt:     execution.TextPrompt(prompt),
	})
	if errors.Is(e, execution.ErrWorkspaceBusy) {
		busy := &BusyError{}
		if o, oe := s.store.ChatWorkspaceOccupant(ctx, scope.TenantID, scope.UserID, c.ProjectID, c.WorkspaceID); oe == nil && o.ID != c.ID {
			busy.Occupant = &o
		} else if oe != nil && !errors.Is(oe, store.ErrNotFound) {
			return ref, errors.Join(busy, oe)
		}
		return ref, busy
	}
	if e != nil {
		return ref, e
	}
	return ref, s.store.RecordChatTurn(ctx, c.ID, ref.SessionID, key, ref.TurnID, s.now())
}

// Stop cancels the live session's latest turn. On the fixed Node that ends the whole session, so
// it is recorded as stopped once the port confirms; an unconfirmed stop changes nothing.
func (s *Service) Stop(ctx context.Context, scope execution.Scope, conversationID string) (execution.Result, error) {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return execution.Result{}, e
	}
	live, ok := c.Live()
	if !ok {
		return execution.Result{}, ErrNoActiveTurn
	}
	result, e := s.runtime.Cancel(ctx, scope, execution.Ref{SessionID: live.ID, TurnID: live.LastTurnID})
	if e != nil {
		return result, e
	}
	session, e := s.runtime.ReadSession(ctx, scope, live.ID)
	if e != nil {
		return result, fmt.Errorf("read stopped session: %w", e)
	}
	if session.EndedAt != nil {
		if e := s.store.EndChatSession(ctx, live.ID, store.EndStopped, s.now()); e != nil {
			return result, e
		}
	}
	return result, nil
}

// Release ends the live session between turns, so another conversation can use its workspace.
// A running turn is stopped with it. Without a live session there is nothing to release.
func (s *Service) Release(ctx context.Context, scope execution.Scope, conversationID string) (store.ChatConversation, error) {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return c, e
	}
	live, ok := c.Live()
	if !ok {
		return c, ErrNoActiveTurn
	}
	if e := s.runtime.End(ctx, scope, live.ID); e != nil {
		return c, fmt.Errorf("end session: %w", e)
	}
	if e := s.store.EndChatSession(ctx, live.ID, store.EndReleased, s.now()); e != nil {
		return c, e
	}
	return s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
}

// Rename changes a conversation's title.
func (s *Service) Rename(ctx context.Context, scope execution.Scope, conversationID, title string) (store.ChatConversation, error) {
	if e := s.store.RenameChatConversation(ctx, scope.TenantID, scope.UserID, conversationID, title); e != nil {
		return store.ChatConversation{}, e
	}
	return s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
}

// Delete ends the live session, then removes the conversation. If the port cannot confirm the
// session ended, the conversation stays: deleting it would hide a session still holding the
// workspace.
func (s *Service) Delete(ctx context.Context, scope execution.Scope, conversationID string) error {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return e
	}
	if live, ok := c.Live(); ok {
		if e := s.runtime.End(ctx, scope, live.ID); e != nil {
			return fmt.Errorf("end session: %w", e)
		}
	}
	if e := s.store.DeleteChatConversation(ctx, scope.TenantID, scope.UserID, conversationID); e != nil {
		return e
	}
	s.locks.Delete(conversationID)
	return nil
}

// Session returns one of the conversation's sessions, so a page can only observe its own.
func (s *Service) Session(ctx context.Context, scope execution.Scope, conversationID, sessionID string) (store.ChatSession, error) {
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return store.ChatSession{}, e
	}
	for _, x := range c.Sessions {
		if x.ID == sessionID {
			return x, nil
		}
	}
	return store.ChatSession{}, store.ErrNotFound
}

// Subscribe follows one session's events after the cursor.
func (s *Service) Subscribe(ctx context.Context, scope execution.Scope, sessionID string, after uint64) (execution.Stream, error) {
	return s.runtime.Subscribe(ctx, scope, sessionID, after)
}

func caller(conversationID, attemptID string) execution.Caller {
	return execution.Caller{Kind: execution.CallerChat, ID: conversationID, AttemptID: attemptID}
}

// uncertain reports an admission whose outcome is unknown: the same request must be retried to
// settle it, so the conversation it may belong to has to survive.
func uncertain(e error) bool {
	return errors.Is(e, execution.ErrAdmissionUnknown) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded)
}

// defaultTitle names a conversation after the start of its first prompt, as desktop does.
func defaultTitle(prompt string) string {
	t := strings.Join(strings.Fields(prompt), " ")
	if r := []rune(t); len(r) > 24 {
		t = string(r[:24])
	}
	if t == "" {
		return "新对话"
	}
	return t
}
