package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

const agentProject = "00000000-0000-0000-0000-000000000123"

const agentTarget = `{"kind":"chat","prompt":"frozen prompt","executorId":"catalog-executor"}`

func TestProjectConversationUsesFrozenBindingAndWaitsForCleanup(t *testing.T) {
	f := newConversationFixture(t, agentProject, json.RawMessage(agentTarget))
	ctx := context.Background()
	var err error
	starts, ends := 0, 0
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		t.Fatal("project conversation fell back to model-only execution")
		return execution.Ref{}, nil
	}
	f.executor.StartFunc = func(_ context.Context, scope execution.Scope, req execution.StartRequest) (execution.Ref, error) {
		starts++
		prompt, err := req.Prompt.Text()
		if err != nil || scope != f.scope || !reflect.DeepEqual(req.Caller, occurrenceCaller(f.occ)) || req.Target.ProjectID != agentProject || req.ExecutorID != "catalog-executor" || prompt != "frozen prompt" {
			t.Fatalf("wrong frozen selection: scope=%+v request=%+v prompt=%q err=%v", scope, req, prompt, err)
		}
		// A configuration edit after admission begins must not change its frozen selection.
		_, err = f.records.UpdateScheduleConfig(ctx, f.scope.TenantID, f.scope.UserID, f.occ.ScheduleID, store.ScheduleInput{
			Name: "edited", Version: 1, Target: json.RawMessage(`{"kind":"chat","prompt":"new prompt","model":"new-model"}`),
			Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return f.ref, nil
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return f.turn(&execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "Agent answer"}), nil
	}
	f.executor.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
		ends++
		if id != f.ref.SessionID {
			t.Fatal("ended another session")
		}
		if ends == 1 {
			return execution.ErrStopUnconfirmed
		}
		return nil
	}
	if err = f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	if err = f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil || starts != 1 {
		t.Fatalf("duplicate admission: starts=%d err=%v", starts, err)
	}
	if err = f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatal(err)
	}
	if o := f.read(t); o.Status != "succeeded" || o.Released {
		t.Fatalf("result released Workspace before End: %+v", o)
	}
	at := f.occ.ScheduledAt.Add(time.Minute)
	overlap, err := f.records.RecordOccurrence(ctx, f.scope.TenantID, f.scope.UserID, f.occ.ScheduleID, store.Trigger{Kind: "manual", Key: "while-cleaning", At: at, Version: 2}, at)
	if err != nil || overlap.Status != "skipped" || overlap.Reason != "previous_unfinished" {
		t.Fatalf("cleanup allowed overlapping manual run: %+v %v", overlap, err)
	}
	if err = f.adapter.Reconcile(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	if o := f.read(t); !o.Released || o.Status != "succeeded" || starts != 1 || ends != 2 {
		t.Fatalf("cleanup retried execution or lost result: %+v starts=%d ends=%d", o, starts, ends)
	}
}

func TestProjectAdmissionBusyAndConflictNeverFallBackOrReplay(t *testing.T) {
	for _, mode := range []string{"busy", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			f := newConversationFixture(t, agentProject, json.RawMessage(agentTarget))
			ctx := context.Background()
			starts := 0
			f.executor.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
				starts++
				if mode == "busy" {
					return execution.Ref{}, execution.ErrWorkspaceBusy
				}
				return execution.Ref{}, execution.ErrConflict
			}
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				t.Fatal("failed Agent admission fell back to direct model")
				return execution.Ref{}, nil
			}
			if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err == nil {
				t.Fatal("admission error hidden")
			}
			if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil || starts != 1 {
				t.Fatalf("admission retried: %d %v", starts, err)
			}
			o := f.read(t)
			if mode == "busy" {
				if o.Status != "skipped" || o.Reason != "execution_busy" || !o.Released {
					t.Fatal(o)
				}
				return
			}
			f.executor.ReadAttemptFunc = func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
				if scope != f.scope || caller.ID != o.ID || caller.AttemptID != o.ID {
					t.Fatal("reconciliation changed identity")
				}
				return execution.Turn{}, execution.ErrUnavailable
			}
			coordinator, err := NewCoordinator(f.records, f.adapter, func() time.Time { return f.occ.ScheduledAt })
			if err != nil {
				t.Fatal(err)
			}
			if err = coordinator.Recover(ctx, f.scope); !errors.Is(err, execution.ErrUnavailable) {
				t.Fatal(err)
			}
			if o = f.read(t); o.Status != "reconciling" || o.Released || starts != 1 {
				t.Fatalf("restart replayed or released unknown admission: %+v starts=%d", o, starts)
			}
		})
	}
}

func TestConversationInvalidSelectionNeverCallsExecution(t *testing.T) {
	for _, tc := range []struct{ name, project, target, reason string }{
		{"missing executor", agentProject, `{"kind":"chat","prompt":"hello","agent":"display-name","model":"display-name"}`, "executor_required"},
		{"executor without project", "", `{"kind":"chat","prompt":"hello","model":"model","executorId":"executor"}`, "project_required"},
		{"workflow is not chat", agentProject, `{"kind":"workflow","prompt":"hello","executorId":"executor"}`, "unsupported_schedule_target"},
		{"empty prompt", agentProject, `{"kind":"chat","prompt":"","executorId":"executor"}`, "invalid_prompt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConversationFixture(t, tc.project, json.RawMessage(tc.target))
			f.executor.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
				t.Fatal("invalid target called Start")
				return execution.Ref{}, nil
			}
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				t.Fatal("invalid target called StartModel")
				return execution.Ref{}, nil
			}
			if err := f.adapter.Dispatch(context.Background(), f.scope, f.occ); err != nil {
				t.Fatal(err)
			}
			if o := f.read(t); o.Status != "skipped" || o.Reason != tc.reason || !o.Released {
				t.Fatal(o)
			}
		})
	}
}
