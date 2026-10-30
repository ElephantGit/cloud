package executionruntime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// failureAgent controls only the stop-confirmation boundary. It is not Node evidence.
type failureAgent struct {
	result         execution.Result
	runErr, endErr error
	publishFirst   bool
	endEntered     chan string
	allowEnd       chan struct{}
	once           sync.Once
}

func (a *failureAgent) release() { a.once.Do(func() { close(a.allowEnd) }) }
func (a *failureAgent) Run(ctx context.Context, _ execution.Scope, _ execution.Session, _ execution.Turn, emit func(context.Context, execution.Event) error) (execution.Result, error) {
	if a.publishFirst {
		if err := emit(ctx, execution.Event{Kind: execution.EventTurnEnded, Result: &a.result}); err != nil {
			return execution.Result{}, err
		}
	}
	return a.result, a.runErr
}
func (a *failureAgent) End(ctx context.Context, sid string) error {
	select {
	case a.endEntered <- sid:
	default:
	}
	select {
	case <-a.allowEnd:
		return a.endErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (a *failureAgent) Close(context.Context) error { return nil }

func TestRuntimeAgentFailureRequiresStopBeforeSessionEndAndDirectoryReuse(t *testing.T) {
	for _, tc := range []struct {
		name           string
		result         execution.Result
		runErr, endErr error
		publishFirst   bool
	}{
		{name: "failed_result", result: execution.Result{Status: execution.StatusFailed, Answer: "partial", FailureCode: "fixture_failed"}},
		{name: "run_error_after_durable_success", result: execution.Result{Status: execution.StatusCompleted, Answer: "complete", AnswerComplete: true, StopReason: "end_turn"}, runErr: errors.New("fixture transport ended"), publishFirst: true},
		{name: "stop_unconfirmed", result: execution.Result{Status: execution.StatusFailed, Answer: "partial", FailureCode: "fixture_failed"}, endErr: execution.ErrStopUnconfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := runtimeDatabase(t, nil)
			if err := f.service.Close(f.ctx); err != nil {
				t.Fatal(err)
			}
			a := &failureAgent{result: tc.result, runErr: tc.runErr, endErr: tc.endErr, publishFirst: tc.publishFirst, endEntered: make(chan string, 8), allowEnd: make(chan struct{})}
			t.Cleanup(a.release)
			var err error
			f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models, Agent: a})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.CreateProject(f.ctx, f.scope, store.Project{ID: "project", Name: "Demo", RepositoryURL: "https://example.com/demo.git", Commit: "fixed", MirrorPath: "/fake/unused"}); err != nil {
				t.Fatal(err)
			}
			req := execution.StartRequest{Caller: execution.Caller{Kind: execution.CallerWorkflow, ID: "workflow", NodeID: "first", AttemptID: "first"}, Target: execution.Target{ProjectID: "project"}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("run")}
			ref, err := f.service.Start(f.ctx, f.scope, req)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case sid := <-a.endEntered:
				if sid != ref.SessionID {
					t.Fatalf("stopped wrong Agent: %s", sid)
				}
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			f.service.mu.Lock()
			w := f.service.workers[ref.TurnID]
			f.service.mu.Unlock()
			if w == nil {
				t.Fatal("worker released before physical stop confirmation")
			}
			session, err := f.service.ReadSession(f.ctx, f.scope, ref.SessionID)
			if err != nil || session.EndedAt != nil {
				t.Fatalf("unconfirmed stop advertised ended: %+v err=%v", session, err)
			}
			turn, err := f.service.ReadAttempt(f.ctx, f.scope, req.Caller)
			if err != nil || turn.Result == nil || !reflect.DeepEqual(*turn.Result, tc.result) {
				t.Fatalf("first durable result=%+v err=%v", turn, err)
			}
			next := req
			next.Caller.NodeID = "next"
			next.Caller.AttemptID = "next"
			if _, err := f.service.Start(f.ctx, f.scope, next); !errors.Is(err, execution.ErrWorkspaceBusy) {
				t.Fatalf("directory reused before stop confirmation: %v", err)
			}
			if _, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: next.Caller, Prompt: next.Prompt}); !errors.Is(err, execution.ErrSessionEnded) {
				t.Fatalf("broken physical session accepted Append: %v", err)
			}
			a.release()
			select {
			case <-w.done:
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			session, err = f.service.ReadSession(f.ctx, f.scope, ref.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			events, err := f.db.ReadExecutionEvents(f.ctx, f.scope, ref.SessionID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			if tc.endErr != nil {
				if session.EndedAt != nil || len(events) != 1 || events[0].Kind != execution.EventTurnEnded {
					t.Fatalf("failed physical stop published session end: session=%+v events=%+v", session, events)
				}
				if _, err := f.service.Start(f.ctx, f.scope, next); !errors.Is(err, execution.ErrUnavailable) {
					t.Fatalf("stop failure did not fence workflow admission: %v", err)
				}
				if _, err := f.service.StartModel(f.ctx, f.scope, modelRequest("another", "attempt", "hello")); !errors.Is(err, execution.ErrUnavailable) {
					t.Fatalf("failed runtime accepted more work: %v", err)
				}
				if _, err := f.service.ReadAttempt(f.ctx, f.scope, next.Caller); !errors.Is(err, execution.ErrNotFound) {
					t.Fatalf("rejected workflow was recorded: %v", err)
				}
			} else {
				if session.EndedAt == nil || len(events) != 2 || events[1].Kind != execution.EventSessionEnded {
					t.Fatalf("confirmed stop did not end session: session=%+v events=%+v", session, events)
				}
				nextRef, err := f.service.Start(f.ctx, f.scope, next)
				if err != nil {
					t.Fatal(err)
				}
				observeTurn(t, f, nextRef, 0)
				nextSession, err := f.service.ReadSession(f.ctx, f.scope, nextRef.SessionID)
				if err != nil || nextSession.WorkspaceID != session.WorkspaceID {
					t.Fatalf("confirmed directory not reusable: %+v err=%v", nextSession, err)
				}
			}
			persisted, err := f.service.ReadAttempt(f.ctx, f.scope, req.Caller)
			if err != nil || persisted.Result == nil || !reflect.DeepEqual(*persisted.Result, tc.result) {
				t.Fatalf("cleanup overwrote the original result: %+v err=%v", persisted, err)
			}
		})
	}
}
