package executionruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// handoffAgent separates durable turn publication from the driver's return. This models
// the real Node ACK/cleanup window without pretending to provide real-Node evidence.
type handoffAgent struct{ started chan *handoffCall }

type handoffCall struct {
	publish, published, release, cancelled, finished chan struct{}
	publishOnce, releaseOnce                         sync.Once
	err                                              error
}

func (c *handoffCall) publishResult() { c.publishOnce.Do(func() { close(c.publish) }) }
func (c *handoffCall) finish()        { c.releaseOnce.Do(func() { close(c.release) }) }
func (c *handoffCall) unblock()       { c.publishResult(); c.finish() }

func (a *handoffAgent) Run(ctx context.Context, _ execution.Scope, _ execution.Session, _ execution.Turn, emit func(context.Context, execution.Event) error) (execution.Result, error) {
	call := &handoffCall{publish: make(chan struct{}), published: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{}), finished: make(chan struct{})}
	a.started <- call
	defer close(call.finished)
	select {
	case <-call.publish:
	case <-ctx.Done():
	}
	result := execution.Result{Status: execution.StatusCompleted, Answer: "published before driver return", AnswerComplete: true, StopReason: "end_turn"}
	call.err = emit(ctx, execution.Event{Kind: execution.EventTurnEnded, Result: &result})
	close(call.published)
	select {
	case <-call.release:
	case <-ctx.Done():
		close(call.cancelled)
		<-call.release
	}
	return result, call.err
}

func (*handoffAgent) End(context.Context, string) error { return nil }
func (*handoffAgent) Close(context.Context) error       { return nil }

func nextHandoffCall(t *testing.T, f *runtimeFixture, agent *handoffAgent) *handoffCall {
	t.Helper()
	select {
	case call := <-agent.started:
		t.Cleanup(call.unblock)
		return call
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
		return nil
	}
}

func handoffRuntime(t *testing.T) (*runtimeFixture, *handoffAgent, execution.Ref, *handoffCall) {
	t.Helper()
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	agent := &handoffAgent{started: make(chan *handoffCall, 8)}
	var err error
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.CreateProject(f.ctx, f.scope, store.Project{ID: "project", Name: "Demo", RepositoryURL: "https://example.com/demo.git", Commit: "fixed", MirrorPath: "/fake/unused"}); err != nil {
		t.Fatal(err)
	}
	ref, err := f.service.Start(f.ctx, f.scope, execution.StartRequest{Caller: modelRequest("chat", "first", "").Caller, Target: execution.Target{ProjectID: "project"}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("first")})
	if err != nil {
		t.Fatal(err)
	}
	return f, agent, ref, nextHandoffCall(t, f, agent)
}

func observePublishedHandoff(t *testing.T, f *runtimeFixture, ref execution.Ref, call *handoffCall) {
	t.Helper()
	call.publishResult()
	select {
	case <-call.published:
		if call.err != nil {
			t.Fatal(call.err)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	stream, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	event, err := stream.Next(f.ctx)
	if err != nil || event.Ref != ref || event.Kind != execution.EventTurnEnded || event.Result == nil {
		t.Fatalf("observer did not receive the retained terminal event: %+v %v", event, err)
	}
	select {
	case <-call.finished:
		t.Fatal("driver returned before the test opened its cleanup gate")
	default:
	}
}

func TestRuntimeAppendWaitsForPublishedTurnCleanup(t *testing.T) {
	f, agent, ref, first := handoffRuntime(t)
	next := execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("chat", "next", "").Caller, Prompt: execution.TextPrompt("next")}
	if _, err := f.service.Append(f.ctx, f.scope, next); !errors.Is(err, execution.ErrWorkspaceBusy) {
		t.Fatalf("a genuinely running turn did not return Busy: %v", err)
	}
	observePublishedHandoff(t, f, ref, first)
	deadline, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := f.service.Append(deadline, f.scope, next); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("published terminal must wait for driver cleanup and honor request deadline, got %v", err)
	}
	if _, err := f.service.ReadAttempt(f.ctx, f.scope, next.Caller); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("timed-out handoff admitted another turn: %v", err)
	}
	replay := execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("chat", "first", "").Caller, Prompt: execution.TextPrompt("first")}
	if actual, err := f.service.Append(f.ctx, f.scope, replay); err != nil || actual != ref {
		t.Fatalf("existing attempt was blocked by cleanup: %+v %v", actual, err)
	}
	type response struct {
		ref execution.Ref
		err error
	}
	responses := make(chan response, 2)
	for range 2 {
		go func() {
			ref, err := f.service.Append(f.ctx, f.scope, next)
			responses <- response{ref, err}
		}()
	}
	first.finish()
	var accepted execution.Ref
	for range 2 {
		select {
		case response := <-responses:
			if response.err != nil || response.ref.SessionID != ref.SessionID || response.ref.TurnID == ref.TurnID || (accepted != (execution.Ref{}) && response.ref != accepted) {
				t.Fatalf("concurrent handoff did not retain one next attempt: %+v previous=%+v", response, accepted)
			}
			accepted = response.ref
		case <-f.ctx.Done():
			t.Fatal(f.ctx.Err())
		}
	}
	second := nextHandoffCall(t, f, agent)
	select {
	case <-first.finished:
	default:
		t.Fatal("next driver turn overlapped the prior cleanup")
	}
	select {
	case <-agent.started:
		t.Fatal("a duplicate concurrent Append dispatched another driver turn")
	default:
	}
	second.unblock()
	observeTurn(t, f, accepted, 0)
}

func TestRuntimeAppendPublishedHandoffRechecksClose(t *testing.T) {
	f, _, ref, first := handoffRuntime(t)
	observePublishedHandoff(t, f, ref, first)
	appendResult := make(chan error, 1)
	go func() {
		_, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("chat", "next", "").Caller, Prompt: execution.TextPrompt("next")})
		appendResult <- err
	}()
	closeResult := make(chan error, 1)
	go func() { closeResult <- f.service.Close(f.ctx) }()
	select {
	case <-first.cancelled:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	first.finish()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	select {
	case err := <-appendResult:
		if !errors.Is(err, execution.ErrUnavailable) {
			t.Fatalf("handoff accepted a turn after Close: %v", err)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
}
