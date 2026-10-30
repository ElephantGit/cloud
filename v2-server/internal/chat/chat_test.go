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

// port is a small in-memory model of the fixed-Node execution rules chat depends on: one live
// session per workspace, idempotent admission by caller, cancel ends the session, End releases.
type port struct {
	mu       sync.Mutex
	next     int
	sessions map[string]*fakeSession // by session ID
	attempts map[string]execution.Ref
	turns    map[string]*execution.Turn // by turn ID
	ended    []string
	// endErr, when set, fails End without releasing.
	endErr error
	// startErr, when set, fails the next Start.
	startErr error
}

type fakeSession struct {
	target execution.Target
	ended  bool
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
			for _, s := range p.sessions {
				if !s.ended && s.target == req.Target {
					return execution.Ref{}, execution.ErrWorkspaceBusy
				}
			}
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
			if !ok {
				return execution.Ref{}, execution.ErrNotFound
			}
			if s.ended {
				return execution.Ref{}, execution.ErrSessionEnded
			}
			return p.admit(req.SessionID, req.Caller), nil
		},
		CancelFunc: func(_ context.Context, _ execution.Scope, ref execution.Ref) (execution.Result, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.sessions[ref.SessionID].ended = true
			r := execution.Result{Status: execution.StatusCancelled}
			p.turns[ref.TurnID].Result = &r
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
		ReadAttemptFunc: func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			ref, ok := p.attempts[attemptKey(c)]
			if !ok {
				return execution.Turn{}, execution.ErrNotFound
			}
			return *p.turns[ref.TurnID], nil
		},
	}
}

func (p *port) admit(sid string, c execution.Caller) execution.Ref {
	p.next++
	ref := execution.Ref{SessionID: sid, TurnID: fmt.Sprintf("t%d", p.next)}
	p.attempts[attemptKey(c)] = ref
	p.turns[ref.TurnID] = &execution.Turn{Ref: ref, Caller: c}
	return ref
}

// finish completes a turn at the given time.
func (p *port) finish(turnID string, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.turns[turnID].EndedAt = &at
	p.turns[turnID].Result = &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true}
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type fixture struct {
	svc   *chat.Service
	port  *port
	clock *clock
	scope execution.Scope
}

func setup(t *testing.T) fixture {
	t.Helper()
	s := storetest.Open(t)
	user, tenant := storetest.Login(t, s, "chat-"+t.Name())
	p := newPort()
	c := &clock{t: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)}
	svc, e := chat.New(s, struct {
		execution.Service
		execution.Catalog
	}{p.runtime(), p.runtime()}, chat.Options{IdleAfter: 10 * time.Minute, Now: c.now})
	if e != nil {
		t.Fatal(e)
	}
	return fixture{svc: svc, port: p, clock: c, scope: execution.Scope{TenantID: tenant, UserID: user}}
}

func (f fixture) create(t *testing.T, key, workspace string) (store.ChatConversation, execution.Ref) {
	t.Helper()
	c, ref, e := f.svc.Create(context.Background(), f.scope, chat.CreateRequest{
		Key: key, ProjectID: "demo", WorkspaceID: workspace, ExecutorID: "fixture", Prompt: "  梳理 v2 接口缺口\n然后列出来  ",
	})
	if e != nil {
		t.Fatalf("create %s: %v", key, e)
	}
	return c, ref
}

func TestCreateRetryAndAppend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, first := f.create(t, "k1", "")
	if c.Title != "梳理 v2 接口缺口 然后列出来" || len(c.Sessions) != 1 || c.Sessions[0].ID != first.SessionID {
		t.Fatalf("created conversation = %+v", c)
	}
	again, ref := f.create(t, "k1", "")
	if again.ID != c.ID || ref != first || len(again.Sessions) != 1 {
		t.Fatalf("retried create must return the same conversation and admission: %+v %+v", again, ref)
	}
	f.clock.add(time.Minute)
	c, next, e := f.svc.Send(ctx, f.scope, c.ID, "k2", "继续")
	if e != nil || next.SessionID != first.SessionID || next.TurnID == first.TurnID || len(c.Sessions) != 1 {
		t.Fatalf("send must append to the live session: %+v %+v %v", c, next, e)
	}
	if _, _, e := f.svc.Create(ctx, f.scope, chat.CreateRequest{Key: "k1", ProjectID: "other", ExecutorID: "fixture", Prompt: "x"}); !errors.Is(e, store.ErrKeyReused) {
		t.Fatalf("reused key with other fields: %v", e)
	}
}

func TestBusyNamesOccupantAndLeavesNoEmptyConversation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	occupant, _ := f.create(t, "k1", "")
	_, _, e := f.svc.Create(ctx, f.scope, chat.CreateRequest{Key: "k2", ProjectID: "demo", ExecutorID: "fixture", Prompt: "第二个"})
	var busy *chat.BusyError
	if !errors.As(e, &busy) || !errors.Is(e, execution.ErrWorkspaceBusy) || busy.Occupant == nil || busy.Occupant.ID != occupant.ID {
		t.Fatalf("busy create = %v (%+v)", e, busy)
	}
	list, e := f.svc.Conversations(ctx, f.scope)
	if e != nil || len(list) != 1 {
		t.Fatalf("a refused first turn must not leave a conversation: %+v %v", list, e)
	}
	// Another worktree is independent.
	f.create(t, "k3", "wt-1")
}

func TestUnknownAdmissionKeepsConversation(t *testing.T) {
	f := setup(t)
	f.port.startErr = execution.ErrAdmissionUnknown
	_, _, e := f.svc.Create(context.Background(), f.scope, chat.CreateRequest{Key: "k1", ProjectID: "demo", ExecutorID: "fixture", Prompt: "hi"})
	if !errors.Is(e, execution.ErrAdmissionUnknown) {
		t.Fatal(e)
	}
	c, _ := f.create(t, "k1", "")
	if len(c.Sessions) != 1 {
		t.Fatalf("the retry with the same key must settle the same conversation: %+v", c)
	}
}

func TestStopThenSendStartsNewSession(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, first := f.create(t, "k1", "")
	r, e := f.svc.Stop(ctx, f.scope, c.ID)
	if e != nil || r.Status != execution.StatusCancelled {
		t.Fatalf("stop = %+v %v", r, e)
	}
	if _, e := f.svc.Stop(ctx, f.scope, c.ID); !errors.Is(e, chat.ErrNoActiveTurn) {
		t.Fatalf("second stop = %v", e)
	}
	c, next, e := f.svc.Send(ctx, f.scope, c.ID, "k2", "再来")
	if e != nil || next.SessionID == first.SessionID {
		t.Fatalf("send after stop = %+v %v", next, e)
	}
	if len(c.Sessions) != 2 || c.Sessions[0].EndReason != store.EndStopped || c.Sessions[0].EndedAt == nil || c.Sessions[1].Ordinal != 2 {
		t.Fatalf("sessions after stop = %+v", c.Sessions)
	}
}

func TestReleaseFreesWorkspace(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	held, ref := f.create(t, "k1", "")
	f.port.finish(ref.TurnID, f.clock.now())
	if _, _, e := f.svc.Create(ctx, f.scope, chat.CreateRequest{Key: "k2", ProjectID: "demo", ExecutorID: "fixture", Prompt: "b"}); !errors.Is(e, execution.ErrWorkspaceBusy) {
		t.Fatalf("an idle live session must still hold the workspace: %v", e)
	}
	c, e := f.svc.Release(ctx, f.scope, held.ID)
	if e != nil || c.Sessions[0].EndReason != store.EndReleased {
		t.Fatalf("release = %+v %v", c, e)
	}
	if _, e := f.svc.Release(ctx, f.scope, held.ID); !errors.Is(e, chat.ErrNoActiveTurn) {
		t.Fatalf("second release = %v", e)
	}
	f.create(t, "k2", "")
}

func TestEndedOutsideChatIsInterrupted(t *testing.T) {
	f := setup(t)
	c, first := f.create(t, "k1", "")
	f.port.sessions[first.SessionID].ended = true // a restart ended it
	c, next, e := f.svc.Send(context.Background(), f.scope, c.ID, "k2", "还在吗")
	if e != nil || next.SessionID == first.SessionID || c.Sessions[0].EndReason != store.EndInterrupted {
		t.Fatalf("send after restart = %+v %+v %v", c.Sessions, next, e)
	}
}

func TestReclaimIdle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	running, _ := f.create(t, "k1", "")
	done, doneRef := f.create(t, "k2", "wt-1")
	recent, recentRef := f.create(t, "k3", "wt-2")
	f.port.finish(doneRef.TurnID, f.clock.now().Add(time.Minute))
	f.clock.add(9 * time.Minute)
	_, recentRef, _ = f.svc.Send(ctx, f.scope, recent.ID, "k4", "还有")
	f.port.finish(recentRef.TurnID, f.clock.now())
	f.clock.add(3 * time.Minute) // 12 min after the first prompts, 3 after the recent one

	n, e := f.svc.ReclaimIdle(ctx)
	if e != nil || n != 1 || len(f.port.ended) != 1 || f.port.ended[0] != doneRef.SessionID {
		t.Fatalf("reclaim = %d %v, ended %v", n, e, f.port.ended)
	}
	list, _ := f.svc.Conversations(ctx, f.scope)
	for _, c := range list {
		_, live := c.Live()
		if want := c.ID != done.ID; live != want {
			t.Fatalf("conversation %s live = %v (running %s)", c.ID, live, running.ID)
		}
		if c.ID == done.ID && c.Sessions[0].EndReason != store.EndIdle {
			t.Fatalf("idle end reason = %q", c.Sessions[0].EndReason)
		}
	}
}

func TestDeleteEndsSessionFirst(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, ref := f.create(t, "k1", "")
	f.port.endErr = execution.ErrStopUnconfirmed
	if e := f.svc.Delete(ctx, f.scope, c.ID); !errors.Is(e, execution.ErrStopUnconfirmed) {
		t.Fatalf("delete with unconfirmed end = %v", e)
	}
	if list, _ := f.svc.Conversations(ctx, f.scope); len(list) != 1 {
		t.Fatal("a conversation whose session may still hold the workspace must stay")
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
	c, _ := f.create(t, "k1", "")
	other := execution.Scope{TenantID: f.scope.TenantID, UserID: "00000000-0000-0000-0000-000000000000"}
	if _, _, e := f.svc.Send(context.Background(), other, c.ID, "k2", "x"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("send by another user = %v", e)
	}
	if _, e := f.svc.Session(context.Background(), f.scope, c.ID, "unknown"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("unknown session = %v", e)
	}
}
