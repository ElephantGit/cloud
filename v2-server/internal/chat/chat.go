// Package chat is the chat page's domain: conversations and the execution session behind each.
// It runs Agents only through the shared execution port and owns titles, ordering and deletion,
// which the port leaves to pages.
//
// A conversation runs in exactly one execution session, in a directory the port makes for it, so
// conversations never contend with each other, with schedules or with workflows. The session stays
// live between turns until the person stops a turn (the fixed Node then ends the whole session),
// deletes the conversation, or the server restarts. After that the conversation is read-only:
// chat never starts a replacement session or replays history, and the page offers a new chat.
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

// ErrNoActiveTurn reports a stop with no live session to act on.
var ErrNoActiveTurn = errors.New("no active turn")

// Store is the persistence chat needs; *store.Store implements it.
type Store interface {
	CreateChatConversation(context.Context, store.NewChatConversation) (store.ChatConversation, error)
	ChatConversation(ctx context.Context, tenant, user, id string) (store.ChatConversation, error)
	ChatConversations(ctx context.Context, tenant, user string) ([]store.ChatConversation, error)
	RenameChatConversation(ctx context.Context, tenant, user, id, title string) error
	DeleteChatConversation(ctx context.Context, tenant, user, id string) error
	RecordChatTurn(ctx context.Context, conversationID, sessionID, attemptID, turnID string, at time.Time) error
	EndChatSession(ctx context.Context, sessionID, reason string, at time.Time) error
	MarkChatSessionEnding(ctx context.Context, sessionID, reason string) error
	ClearChatSessionEnding(ctx context.Context, sessionID string) error
}

// Options tune a Service.
type Options struct {
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service coordinates conversations with the execution port.
type Service struct {
	store   Store
	runtime execution.Service
	catalog execution.Catalog
	now     func() time.Time
	// locks serialize work on one conversation, so two first prompts cannot both start a session.
	// v2-server is a single process, which is what makes an in-memory lock sufficient.
	locks sync.Map
}

// New returns a Service that runs Agents on runtime and lists the choices from catalog: the
// shared execution service and catalog v2-server injects into every page.
func New(s Store, runtime execution.Service, catalog execution.Catalog, o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{store: s, runtime: runtime, catalog: catalog, now: o.Now}
}

func (s *Service) lock(id string) func() {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Executors lists the Agent and model bindings a chat can start with.
func (s *Service) Executors(ctx context.Context, scope execution.Scope) ([]execution.Executor, error) {
	return s.catalog.ListExecutors(ctx, scope)
}

// Conversations lists the person's conversations.
func (s *Service) Conversations(ctx context.Context, scope execution.Scope) ([]store.ChatConversation, error) {
	return s.store.ChatConversations(ctx, scope.TenantID, scope.UserID)
}

// CreateRequest starts a conversation with its first prompt. Key is the request's idempotency
// key; it becomes the first turn's attempt ID, so a retry with the same key finds the same
// conversation and turn.
type CreateRequest struct {
	Key        string
	ProjectID  string
	ExecutorID string
	Title      string
	Prompt     string
}

// Create records the conversation and starts its session with the first turn. A conversation
// whose first turn is definitely refused is removed again, so the history never shows an empty
// chat; one whose outcome is unknown stays, for the retry with the same key to settle.
func (s *Service) Create(ctx context.Context, scope execution.Scope, in CreateRequest) (store.ChatConversation, execution.Ref, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = defaultTitle(in.Prompt)
	}
	c, e := s.store.CreateChatConversation(ctx, store.NewChatConversation{
		TenantID: scope.TenantID, UserID: scope.UserID, CreateKey: in.Key,
		ProjectID: in.ProjectID, ExecutorID: in.ExecutorID, Title: title,
	})
	if e != nil {
		return c, execution.Ref{}, e
	}
	defer s.lock(c.ID)()
	ref, e := s.start(ctx, scope, c, in.Key, in.Prompt)
	if e != nil {
		if c.Session == nil && !uncertain(e) {
			if de := s.store.DeleteChatConversation(context.WithoutCancel(ctx), scope.TenantID, scope.UserID, c.ID); de != nil {
				return c, ref, errors.Join(e, de)
			}
		}
		return c, ref, e
	}
	c, e = s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, c.ID)
	return c, ref, e
}

// Send appends the next prompt to the conversation's session. Key is the request's idempotency
// key and the turn's attempt ID. An ended conversation returns execution.ErrSessionEnded; one
// still answering the previous prompt returns execution.ErrWorkspaceBusy, and the page queues.
func (s *Service) Send(ctx context.Context, scope execution.Scope, conversationID, key, prompt string) (store.ChatConversation, execution.Ref, error) {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return c, execution.Ref{}, e
	}
	var ref execution.Ref
	live, ok := c.Live()
	switch {
	case c.Ended():
		return c, ref, execution.ErrSessionEnded
	case !ok:
		// The first turn's outcome was unknown and never settled; this prompt starts the session.
		ref, e = s.start(ctx, scope, c, key, prompt)
	default:
		ref, e = s.runtime.Append(ctx, scope, execution.AppendRequest{
			SessionID: live.ID, Caller: caller(c.ID, key), Prompt: execution.TextPrompt(prompt),
		})
		if errors.Is(e, execution.ErrSessionEnded) {
			// Ended outside chat (a restart): record it so the page shows the conversation as ended.
			return c, ref, errors.Join(e, s.store.EndChatSession(ctx, live.ID, store.EndInterrupted, s.now()))
		}
		if e == nil {
			e = s.store.RecordChatTurn(ctx, c.ID, ref.SessionID, key, ref.TurnID, s.now())
		}
	}
	if e != nil {
		return c, ref, e
	}
	c, e = s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, c.ID)
	return c, ref, e
}

// ending runs a call that may end the live session, with the reason recorded first so a page
// reading the conversation on session_ended sees why. A failed call withdraws the reason.
func (s *Service) ending(ctx context.Context, sessionID, reason string, call func() error) error {
	if e := s.store.MarkChatSessionEnding(ctx, sessionID, reason); e != nil {
		return e
	}
	if e := call(); e != nil {
		return errors.Join(e, s.store.ClearChatSessionEnding(context.WithoutCancel(ctx), sessionID))
	}
	return nil
}

// start starts the conversation's session with a prompt. The caller holds the lock.
func (s *Service) start(ctx context.Context, scope execution.Scope, c store.ChatConversation, key, prompt string) (execution.Ref, error) {
	ref, e := s.runtime.Start(ctx, scope, execution.StartRequest{
		Caller:     caller(c.ID, key),
		Target:     execution.Target{ProjectID: c.ProjectID},
		ExecutorID: c.ExecutorID,
		Prompt:     execution.TextPrompt(prompt),
	})
	if e != nil {
		return ref, e
	}
	return ref, s.store.RecordChatTurn(ctx, c.ID, ref.SessionID, key, ref.TurnID, s.now())
}

// Stop cancels the session's latest turn. On the fixed Node that ends the whole session, so it is
// recorded as stopped once the port confirms; an unconfirmed stop changes nothing.
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
	var result execution.Result
	var session execution.Session
	e = s.ending(ctx, live.ID, store.EndStopped, func() error {
		var e error
		if result, e = s.runtime.Cancel(ctx, scope, execution.Ref{SessionID: live.ID, TurnID: live.LastTurnID}); e != nil {
			return e
		}
		if session, e = s.runtime.ReadSession(ctx, scope, live.ID); e != nil {
			return fmt.Errorf("read stopped session: %w", e)
		}
		return nil
	})
	if e != nil {
		return result, e
	}
	if session.EndedAt == nil {
		// The turn had already finished, so cancelling it ended nothing: the session stays live.
		return result, s.store.ClearChatSessionEnding(ctx, live.ID)
	}
	return result, s.store.EndChatSession(ctx, live.ID, store.EndStopped, s.now())
}

// Rename changes a conversation's title.
func (s *Service) Rename(ctx context.Context, scope execution.Scope, conversationID, title string) (store.ChatConversation, error) {
	if e := s.store.RenameChatConversation(ctx, scope.TenantID, scope.UserID, conversationID, title); e != nil {
		return store.ChatConversation{}, e
	}
	return s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
}

// Delete ends the live session, then removes the conversation. If the port cannot confirm the
// session ended, the conversation stays: deleting it would hide an Agent that is still running.
func (s *Service) Delete(ctx context.Context, scope execution.Scope, conversationID string) error {
	defer s.lock(conversationID)()
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return e
	}
	if live, ok := c.Live(); ok {
		if e := s.ending(ctx, live.ID, store.EndDeleted, func() error { return s.runtime.End(ctx, scope, live.ID) }); e != nil {
			return fmt.Errorf("end session: %w", e)
		}
	}
	if e := s.store.DeleteChatConversation(ctx, scope.TenantID, scope.UserID, conversationID); e != nil {
		return e
	}
	s.locks.Delete(conversationID)
	return nil
}

// Subscribe follows the conversation's session events after the cursor, so a page can only
// observe its own conversations.
func (s *Service) Subscribe(ctx context.Context, scope execution.Scope, conversationID string, after uint64) (execution.Stream, error) {
	c, e := s.store.ChatConversation(ctx, scope.TenantID, scope.UserID, conversationID)
	if e != nil {
		return nil, e
	}
	if c.Session == nil {
		return nil, store.ErrNotFound
	}
	return s.runtime.Subscribe(ctx, scope, c.Session.ID, after)
}

func caller(conversationID, attemptID string) execution.Caller {
	return execution.Caller{Kind: execution.CallerChat, ID: conversationID, AttemptID: attemptID}
}

// uncertain reports a call whose outcome is unknown: the same request must be retried to settle
// it, so the conversation it may belong to has to survive.
func uncertain(e error) bool {
	return errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded)
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
