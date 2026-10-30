package chat_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/chat"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

// port is a small in-memory model of the fixed-Node execution rules chat depends on: every
// Start makes its own session (and directory), a session runs one turn at a time, idempotent
// turns by caller, and cancelling a running turn ends the whole session.
type port struct {
	mu       sync.Mutex
	next     int
	starts   int
	sessions map[string]*fakeSession // by session ID
	attempts map[string]execution.Ref
	turns    map[string]*execution.Turn // by turn ID
	ended    []string
	// endErr, when set, fails End without ending the session.
	endErr error
	// startErr, when set, fails the next Start.
	startErr error
	// onCancel runs inside Cancel, before it returns.
	onCancel func()
}

type fakeSession struct {
	target  execution.Target
	ended   bool
	running string // the running turn's ID, if any
}

func newPort() *port {
	return &port{sessions: map[string]*fakeSession{}, attempts: map[string]execution.Ref{}, turns: map[string]*execution.Turn{}}
}

func attemptKey(c execution.Caller) string { return string(c.Kind) + "/" + c.ID + "/" + c.AttemptID }

func (p *port) runtime() *executiontest.Stub {
	return &executiontest.Stub{
		StartFunc: func(_ context.Context, _ execution.Scope, req execution.StartRequest) (execution.Ref, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if e := p.startErr; e != nil {
				p.startErr = nil
				return execution.Ref{}, e
			}
			if ref, ok := p.attempts[attemptKey(req.Caller)]; ok {
				return ref, nil
			}
			p.starts++
			p.next++
			sid := fmt.Sprintf("s%d", p.next)
			p.sessions[sid] = &fakeSession{target: req.Target}
			return p.admit(sid, req.Caller), nil
		},
		AppendFunc: func(_ context.Context, _ execution.Scope, req execution.AppendRequest) (execution.Ref, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if ref, ok := p.attempts[attemptKey(req.Caller)]; ok {
				return ref, nil
			}
			s, ok := p.sessions[req.SessionID]
			switch {
			case !ok:
				return execution.Ref{}, execution.ErrNotFound
			case s.ended:
				return execution.Ref{}, execution.ErrSessionEnded
			case s.running != "":
				return execution.Ref{}, execution.ErrWorkspaceBusy
			}
			return p.admit(req.SessionID, req.Caller), nil
		},
		CancelFunc: func(_ context.Context, _ execution.Scope, ref execution.Ref) (execution.Result, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.onCancel != nil {
				p.mu.Unlock()
				p.onCancel()
				p.mu.Lock()
			}
			s, t := p.sessions[ref.SessionID], p.turns[ref.TurnID]
			if s.running != ref.TurnID {
				// A terminal turn is a no-op: its result stands and the session stays live.
				return *t.Result, nil
			}
			s.running, s.ended = "", true
			r := execution.Result{Status: execution.StatusCancelled}
			t.Result = &r
			return r, nil
		},
		EndFunc: func(_ context.Context, _ execution.Scope, sid string) error {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.endErr != nil {
				return p.endErr
			}
			p.sessions[sid].ended = true
			p.ended = append(p.ended, sid)
			return nil
		},
		ReadSessionFunc: func(_ context.Context, _ execution.Scope, sid string) (execution.Session, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			s := execution.Session{ID: sid}
			if p.sessions[sid].ended {
				now := time.Now()
				s.EndedAt = &now
			}
			return s, nil
		},
	}
}

func (p *port) admit(sid string, c execution.Caller) execution.Ref {
	p.next++
	ref := execution.Ref{SessionID: sid, TurnID: fmt.Sprintf("t%d", p.next)}
	p.attempts[attemptKey(c)] = ref
	p.turns[ref.TurnID] = &execution.Turn{Ref: ref, Caller: c}
	p.sessions[sid].running = ref.TurnID
	return ref
}

// finish completes a running turn.
func (p *port) finish(ref execution.Ref) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turns[ref.TurnID].Result = &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true}
	p.sessions[ref.SessionID].running = ""
}

type fixture struct {
	svc   *chat.Service
	port  *port
	scope execution.Scope
}

func setup(t *testing.T) fixture {
	t.Helper()
	s := storetest.Open(t)
	user, tenant := storetest.Login(t, s, "chat-"+t.Name())
	p := newPort()
	now := func() time.Time { return time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC) }
	rt := p.runtime()
	svc := chat.New(s, rt, rt, chat.Options{Now: now})
	return fixture{svc: svc, port: p, scope: execution.Scope{TenantID: tenant, UserID: user}}
}

func (f fixture) create(t *testing.T, key string) (store.ChatConversation, execution.Ref) {
	t.Helper()
	c, ref, e := f.svc.Create(context.Background(), f.scope, chat.CreateRequest{
		Key: key, ProjectID: "demo", ExecutorID: "fixture", Prompt: "  梳理 v2 接口缺口\n然后列出来  ",
	})
	if e != nil {
		t.Fatalf("create %s: %v", key, e)
	}
	return c, ref
}

func TestCreateRetryAndAppend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, first := f.create(t, "k1")
	if c.Title != "梳理 v2 接口缺口 然后列出来" || c.Session == nil || c.Session.ID != first.SessionID {
		t.Fatalf("created conversation = %+v", c)
	}
	again, ref := f.create(t, "k1")
	if again.ID != c.ID || ref != first || f.port.starts != 1 {
		t.Fatalf("retried create must return the same conversation and turn: %+v %+v", again, ref)
	}
	f.port.finish(first)
	c, next, e := f.svc.Send(ctx, f.scope, c.ID, "k2", "继续")
	if e != nil || next.SessionID != first.SessionID || next.TurnID == first.TurnID || c.Session.ID != first.SessionID {
		t.Fatalf("send must append to the conversation's session: %+v %+v %v", c, next, e)
	}
	if _, _, e := f.svc.Create(ctx, f.scope, chat.CreateRequest{Key: "k1", ProjectID: "other", ExecutorID: "fixture", Prompt: "x"}); !errors.Is(e, store.ErrKeyReused) {
		t.Fatalf("reused key with other fields: %v", e)
	}
}

func TestConversationsInOneProjectDoNotContend(t *testing.T) {
	f := setup(t)
	a, ra := f.create(t, "k1")
	b, rb := f.create(t, "k2")
	if a.ID == b.ID || ra.SessionID == rb.SessionID {
		t.Fatalf("each conversation must get its own session: %+v %+v", ra, rb)
	}
}

func TestSendWhileTurnRunsIsBusy(t *testing.T) {
	f := setup(t)
	c, _ := f.create(t, "k1")
	if _, _, e := f.svc.Send(context.Background(), f.scope, c.ID, "k2", "再问"); !errors.Is(e, execution.ErrWorkspaceBusy) {
		t.Fatalf("send while the previous turn runs = %v", e)
	}
}

func TestRefusedFirstTurnLeavesNoConversation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.port.startErr = execution.ErrUnavailable
	if _, _, e := f.svc.Create(ctx, f.scope, chat.CreateRequest{Key: "k1", ProjectID: "demo", ExecutorID: "fixture", Prompt: "hi"}); !errors.Is(e, execution.ErrUnavailable) {
		t.Fatal(e)
	}
	if list, e := f.svc.Conversations(ctx, f.scope); e != nil || len(list) != 0 {
		t.Fatalf("a refused first turn must not leave a conversation: %+v %v", list, e)
	}
}

func TestUnknownOutcomeKeepsConversation(t *testing.T) {
	f := setup(t)
	f.port.startErr = context.DeadlineExceeded
	_, _, e := f.svc.Create(context.Background(), f.scope, chat.CreateRequest{Key: "k1", ProjectID: "demo", ExecutorID: "fixture", Prompt: "hi"})
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	c, _ := f.create(t, "k1")
	if c.Session == nil {
		t.Fatalf("the retry with the same key must settle the same conversation: %+v", c)
	}
}

func TestStopEndsTheConversation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, _ := f.create(t, "k1")
	// What a page reading the conversation on session_ended would see, mid-Cancel.
	var during string
	f.port.onCancel = func() {
		list, _ := f.svc.Conversations(ctx, f.scope)
		during = list[0].Session.EndReason
	}
	r, e := f.svc.Stop(ctx, f.scope, c.ID)
	if e != nil || r.Status != execution.StatusCancelled || during != store.EndStopped {
		t.Fatalf("stop = %+v %v, end reason during Cancel %q", r, e, during)
	}
	if _, e := f.svc.Stop(ctx, f.scope, c.ID); !errors.Is(e, chat.ErrNoActiveTurn) {
		t.Fatalf("second stop = %v", e)
	}
	c, _, e = f.svc.Send(ctx, f.scope, c.ID, "k2", "再来")
	if !errors.Is(e, execution.ErrSessionEnded) || f.port.starts != 1 {
		t.Fatalf("send after stop must be refused without a new session: %v, starts %d", e, f.port.starts)
	}
	if !c.Ended() || c.Session.EndReason != store.EndStopped {
		t.Fatalf("session after stop = %+v", c.Session)
	}
}

func TestStopAfterTheTurnFinishedKeepsTheSession(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, ref := f.create(t, "k1")
	f.port.finish(ref)
	if r, e := f.svc.Stop(ctx, f.scope, c.ID); e != nil || r.Status != execution.StatusCompleted {
		t.Fatalf("stop of a finished turn = %+v %v", r, e)
	}
	c, _, e := f.svc.Send(ctx, f.scope, c.ID, "k2", "继续")
	if e != nil || c.Ended() || c.Session.EndReason != "" {
		t.Fatalf("the session must stay live with no end reason: %+v %v", c.Session, e)
	}
}

func TestEndedOutsideChatIsInterrupted(t *testing.T) {
	f := setup(t)
	c, first := f.create(t, "k1")
	f.port.sessions[first.SessionID].ended = true // a restart ended it
	_, _, e := f.svc.Send(context.Background(), f.scope, c.ID, "k2", "还在吗")
	if !errors.Is(e, execution.ErrSessionEnded) || f.port.starts != 1 {
		t.Fatalf("send after restart = %v, starts %d", e, f.port.starts)
	}
	if list, _ := f.svc.Conversations(context.Background(), f.scope); !list[0].Ended() || list[0].Session.EndReason != store.EndInterrupted {
		t.Fatalf("session after restart = %+v", list[0].Session)
	}
}

func TestDeleteEndsSessionFirst(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, ref := f.create(t, "k1")
	f.port.endErr = execution.ErrStopUnconfirmed
	if e := f.svc.Delete(ctx, f.scope, c.ID); !errors.Is(e, execution.ErrStopUnconfirmed) {
		t.Fatalf("delete with unconfirmed end = %v", e)
	}
	if list, _ := f.svc.Conversations(ctx, f.scope); len(list) != 1 || list[0].Session.EndReason != "" {
		t.Fatalf("a conversation whose Agent may still run must stay, with no end reason: %+v", list)
	}
	f.port.endErr = nil
	if e := f.svc.Delete(ctx, f.scope, c.ID); e != nil {
		t.Fatal(e)
	}
	if list, _ := f.svc.Conversations(ctx, f.scope); len(list) != 0 || f.port.ended[0] != ref.SessionID {
		t.Fatalf("after delete: %+v ended %v", list, f.port.ended)
	}
	if e := f.svc.Delete(ctx, f.scope, c.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("second delete = %v", e)
	}
}

func TestOtherUsersConversationsAreHidden(t *testing.T) {
	f := setup(t)
	c, _ := f.create(t, "k1")
	other := execution.Scope{TenantID: f.scope.TenantID, UserID: "00000000-0000-0000-0000-000000000000"}
	if _, _, e := f.svc.Send(context.Background(), other, c.ID, "k2", "x"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("send by another user = %v", e)
	}
	if _, e := f.svc.Subscribe(context.Background(), other, c.ID, 0); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("subscribe by another user = %v", e)
	}
}
