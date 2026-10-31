package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// This is a consumer test: the Stub does not prove Node directory creation or deletion policy.
func TestDistinctOccurrencesStartIndependentSessionsInSameProject(t *testing.T) {
	f := newConversationFixture(t, agentProject, json.RawMessage(agentTarget))
	ctx := context.Background()
	other, err := f.records.CreateScheduleConfig(ctx, f.scope.TenantID, f.scope.UserID, store.ScheduleInput{Name: "second task", ProjectID: agentProject, Target: json.RawMessage(agentTarget), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	at := other.CreatedAt.Add(time.Hour)
	second, err := f.records.RecordOccurrence(ctx, f.scope.TenantID, f.scope.UserID, other.ID, store.Trigger{Kind: "manual", Key: "second", At: at, Version: 1}, at)
	if err != nil {
		t.Fatal(err)
	}
	callers := map[string]bool{}
	f.executor.StartFunc = func(_ context.Context, scope execution.Scope, req execution.StartRequest) (execution.Ref, error) {
		if scope != f.scope || req.Target.ProjectID != agentProject || req.Caller.ID != req.Caller.AttemptID || callers[req.Caller.ID] {
			t.Fatalf("reused session identity: %+v", req)
		}
		callers[req.Caller.ID] = true
		return execution.Ref{SessionID: req.Caller.ID, TurnID: req.Caller.ID}, nil
	}
	f.executor.AppendFunc = func(context.Context, execution.Scope, execution.AppendRequest) (execution.Ref, error) {
		t.Fatal("schedule continued an existing conversation")
		return execution.Ref{}, nil
	}
	for _, o := range []store.Occurrence{f.occ, second} {
		if err = f.adapter.Dispatch(ctx, f.scope, o); err != nil {
			t.Fatal(err)
		}
	}
	if len(callers) != 2 {
		t.Fatalf("same project blocked another task: %v", callers)
	}
	for _, o := range []store.Occurrence{f.occ, second} {
		current, err := f.records.GetOccurrence(ctx, f.scope.TenantID, f.scope.UserID, o.ID)
		if err != nil || current.Status != "running" || current.Released {
			t.Fatalf("independent active session: %+v %v", current, err)
		}
	}
}

func TestCancellationAlwaysEndsDedicatedScheduleSession(t *testing.T) {
	for _, mode := range []string{"model", "agent"} {
		t.Run(mode, func(t *testing.T) {
			var f *modelFixture
			if mode == "agent" {
				f = newConversationFixture(t, agentProject, json.RawMessage(agentTarget))
			} else {
				f = newModelFixture(t, "fixture-model")
			}
			ctx := context.Background()
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				return f.ref, nil
			}
			f.executor.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
				return f.ref, nil
			}
			if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
				t.Fatal(err)
			}
			cancels, ends := 0, 0
			f.executor.CancelFunc = func(_ context.Context, _ execution.Scope, ref execution.Ref) (execution.Result, error) {
				cancels++
				if ref != f.ref {
					t.Fatal("wrong turn cancelled")
				}
				return execution.Result{Status: execution.StatusCancelled}, nil
			}
			// Model Cancel only stops the turn; Agent Cancel already ends the session.
			// In either case a schedule asks idempotent End and waits for its confirmation.
			f.executor.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
				ends++
				if id != f.ref.SessionID {
					t.Fatal("wrong session ended")
				}
				if ends == 1 {
					return execution.ErrStopUnconfirmed
				}
				return nil
			}
			if err := f.adapter.Cancel(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrStopUnconfirmed) {
				t.Fatal(err)
			}
			if o := f.read(t); o.Status != "cancelled" || o.Released {
				t.Fatalf("premature release: %+v", o)
			}
			if err := f.adapter.Reconcile(ctx, f.scope, f.occ); err != nil {
				t.Fatal(err)
			}
			if o := f.read(t); o.Status != "cancelled" || !o.Released || cancels != 1 || ends != 2 {
				t.Fatalf("lost result or repeated cancel: %+v %d/%d", o, cancels, ends)
			}
		})
	}
}
