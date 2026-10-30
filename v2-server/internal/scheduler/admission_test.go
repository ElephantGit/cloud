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

// These tests use real occurrence storage; SealAttempt outcomes are injected. They do
// not prove the execution service's PostgreSQL admission/closure serialization.
func TestConversationSealRecoveryOutcomes(t *testing.T) {
	ref := execution.Ref{SessionID: "session-1", TurnID: "turn-1"}
	for _, project := range []string{"", agentProject} {
		for _, tc := range []struct {
			name       string
			resolution execution.AdmissionResolution
			err        error
			wantErr    error
			status     string
		}{
			{"not admitted", execution.AdmissionResolution{State: execution.AdmissionNotAdmitted}, nil, nil, "skipped"},
			{"accepted", execution.AdmissionResolution{State: execution.AdmissionAccepted, Ref: ref}, nil, nil, "running"},
			{"unknown", execution.AdmissionResolution{}, execution.ErrAdmissionUnknown, execution.ErrAdmissionUnknown, "reconciling"},
			{"unavailable", execution.AdmissionResolution{}, execution.ErrUnavailable, execution.ErrUnavailable, "reconciling"},
			{"unauthorized", execution.AdmissionResolution{}, execution.ErrNotFound, execution.ErrNotFound, "reconciling"},
			{"deadline", execution.AdmissionResolution{}, context.DeadlineExceeded, context.DeadlineExceeded, "reconciling"},
			{"cancelled", execution.AdmissionResolution{}, context.Canceled, context.Canceled, "reconciling"},
			{"zero", execution.AdmissionResolution{}, nil, execution.ErrAdmissionUnknown, "reconciling"},
			{"invalid state", execution.AdmissionResolution{State: "future"}, nil, execution.ErrAdmissionUnknown, "reconciling"},
			{"accepted without turn", execution.AdmissionResolution{State: execution.AdmissionAccepted, Ref: execution.Ref{SessionID: "session-1"}}, nil, execution.ErrAdmissionUnknown, "reconciling"},
			{"accepted without session", execution.AdmissionResolution{State: execution.AdmissionAccepted, Ref: execution.Ref{TurnID: "turn-1"}}, nil, execution.ErrAdmissionUnknown, "reconciling"},
			{"not admitted with ref", execution.AdmissionResolution{State: execution.AdmissionNotAdmitted, Ref: ref}, nil, execution.ErrConflict, "reconciling"},
			{"not admitted with error", execution.AdmissionResolution{State: execution.AdmissionNotAdmitted}, execution.ErrAdmissionUnknown, execution.ErrAdmissionUnknown, "reconciling"},
			{"accepted with error", execution.AdmissionResolution{State: execution.AdmissionAccepted, Ref: ref}, execution.ErrUnavailable, execution.ErrUnavailable, "reconciling"},
		} {
			t.Run(project+"/"+tc.name, func(t *testing.T) {
				target := json.RawMessage(`{"kind":"chat","prompt":"frozen prompt","model":"fixture-model"}`)
				if project != "" {
					target = json.RawMessage(agentTarget)
				}
				f := newConversationFixture(t, project, target)
				ctx := context.Background()
				starts, seals, reads := 0, 0, 0
				f.executor.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
					if project == "" {
						t.Error("model target used Start")
					}
					starts++
					return execution.Ref{}, execution.ErrAdmissionUnknown
				}
				f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
					if project != "" {
						t.Error("project target used StartModel")
					}
					starts++
					return execution.Ref{}, execution.ErrAdmissionUnknown
				}
				if err := f.adapter.Dispatch(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrAdmissionUnknown) {
					t.Fatal(err)
				}
				f.executor.ReadAttemptFunc = func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
					reads++
					if scope != f.scope || !reflect.DeepEqual(caller, occurrenceCaller(f.occ)) {
						t.Fatal("read identity changed")
					}
					if tc.status == "running" && reads > 1 {
						return f.turn(nil), nil
					}
					return execution.Turn{}, execution.ErrNotFound
				}
				f.executor.SealAttemptFunc = func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.AdmissionResolution, error) {
					seals++
					if scope != f.scope || !reflect.DeepEqual(caller, occurrenceCaller(f.occ)) {
						t.Fatal("seal identity changed")
					}
					return tc.resolution, tc.err
				}
				f.executor.EndFunc = func(context.Context, execution.Scope, string) error {
					t.Fatal("admission proof used as cleanup")
					return nil
				}
				coordinator, err := NewCoordinator(f.records, f.adapter, func() time.Time { return f.occ.ScheduledAt })
				if err != nil {
					t.Fatal(err)
				}
				err = coordinator.Recover(ctx, f.scope)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("recovery error=%v want=%v", err, tc.wantErr)
				}
				o := f.read(t)
				if o.Status != tc.status || o.Released != (tc.status == "skipped") || starts != 1 || seals != 1 {
					t.Fatalf("recovery=%+v starts=%d seals=%d", o, starts, seals)
				}
				if tc.status == "running" {
					got, err := storedRef(o)
					if err != nil || got != ref {
						t.Fatalf("lost accepted ref: %+v %v", got, err)
					}
				} else if o.ExecutionRef != "" {
					t.Fatal("unconfirmed response bound a ref")
				}
				if tc.status == "skipped" && o.Reason != "admission_not_accepted" {
					t.Fatal(o)
				}
				// Repeating the manual key must reconcile the same occurrence, never submit again.
				_, err = coordinator.Manual(ctx, f.scope, o.ScheduleID, "first")
				if !errors.Is(err, tc.wantErr) || starts != 1 {
					t.Fatalf("manual replay: %v starts=%d", err, starts)
				}
				if tc.status == "reconciling" && seals != 2 {
					t.Fatal("unknown outcome did not retry SealAttempt")
				}
				if tc.status != "reconciling" && seals != 1 {
					t.Fatal("resolved admission was sealed again")
				}
				next, err := f.records.RecordOccurrence(ctx, f.scope.TenantID, f.scope.UserID, o.ScheduleID, store.Trigger{Kind: "manual", Key: "next", At: o.ScheduledAt, Version: 1}, o.ScheduledAt)
				if err != nil {
					t.Fatal(err)
				}
				if tc.status == "skipped" {
					if next.Status != "recorded" {
						t.Fatalf("confirmed closure retained occupancy: %+v", next)
					}
				} else if next.Status != "skipped" || next.Reason != "previous_unfinished" {
					t.Fatalf("unresolved/accepted allowed overlap: %+v", next)
				}
			})
		}
	}
}

func TestSealAcceptedBindingSurvivesReadFailuresAndRestart(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		return execution.Ref{}, execution.ErrAdmissionUnknown
	}
	if err := f.adapter.Dispatch(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrAdmissionUnknown) {
		t.Fatal(err)
	}
	seals := 0
	f.executor.SealAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.AdmissionResolution, error) {
		seals++
		return execution.AdmissionResolution{State: execution.AdmissionAccepted, Ref: f.ref}, nil
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{}, execution.ErrNotFound
	}
	if err := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrNotFound) {
		t.Fatal(err)
	}
	if o := f.read(t); o.Status != "running" || o.ExecutionRef == "" || o.Released {
		t.Fatalf("accepted binding lost: %+v", o)
	}
	coordinator, err := NewCoordinator(f.records, f.adapter, func() time.Time { return f.occ.ScheduledAt })
	if err != nil {
		t.Fatal(err)
	}
	if err = coordinator.Recover(ctx, f.scope); !errors.Is(err, execution.ErrNotFound) {
		t.Fatal(err)
	}
	if seals != 1 {
		t.Fatal("bound execution sealed on restart")
	}
	for _, mismatch := range []string{"caller", "ref"} {
		f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
			turn := f.turn(&execution.Result{Status: execution.StatusCompleted, AnswerComplete: true})
			if mismatch == "caller" {
				turn.Caller.ID = "other"
			} else {
				turn.Ref.TurnID = "other"
			}
			return turn, nil
		}
		if err := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrConflict) {
			t.Fatalf("mismatch=%s err=%v", mismatch, err)
		}
		if f.read(t).Released {
			t.Fatal("mismatch released occupancy")
		}
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return f.turn(&execution.Result{Status: execution.StatusCompleted, AnswerComplete: true}), nil
	}
	ends := 0
	f.executor.EndFunc = func(context.Context, execution.Scope, string) error {
		ends++
		if ends == 1 {
			return execution.ErrStopUnconfirmed
		}
		return nil
	}
	if err := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatal(err)
	}
	if o := f.read(t); o.Status != "succeeded" || o.Released {
		t.Fatal(o)
	}
	if err := f.adapter.Reconcile(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	if o := f.read(t); o.Status != "succeeded" || !o.Released || seals != 1 || ends != 2 {
		t.Fatalf("cleanup=%+v seals=%d ends=%d", o, seals, ends)
	}
}

func TestSealWinsWhileDispatchIsInFlight(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	entered, resume := make(chan struct{}), make(chan struct{})
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		close(entered)
		<-resume
		return execution.Ref{}, execution.ErrAdmissionClosed
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{}, execution.ErrNotFound
	}
	f.executor.SealAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.AdmissionResolution, error) {
		return execution.AdmissionResolution{State: execution.AdmissionNotAdmitted}, nil
	}
	done := make(chan error, 1)
	go func() { done <- f.adapter.Dispatch(ctx, f.scope, f.occ) }()
	<-entered
	err := f.adapter.Reconcile(ctx, f.scope, f.occ)
	close(resume)
	lateErr := <-done
	if err != nil || !errors.Is(lateErr, execution.ErrAdmissionClosed) || errors.Is(lateErr, store.ErrIneligible) {
		t.Fatalf("seal=%v late dispatch=%v", err, lateErr)
	}
	if o := f.read(t); o.Status != "skipped" || o.Reason != "admission_not_accepted" || !o.Released {
		t.Fatal(o)
	}
}

type failResolutionWrite struct {
	RecordStore
	failed bool
}

func (r *failResolutionWrite) ApplyOccurrenceEvidence(ctx context.Context, tenant, user, id string, version int64, proof store.ExecutionEvidence) (store.Occurrence, error) {
	if !r.failed && (proof.Event == "accepted" || proof.Event == "not_accepted") {
		r.failed = true
		return store.Occurrence{}, context.DeadlineExceeded
	}
	return r.RecordStore.ApplyOccurrenceEvidence(ctx, tenant, user, id, version, proof)
}

func TestSealResolutionRetriesAfterLocalWriteFailure(t *testing.T) {
	for _, state := range []execution.AdmissionState{execution.AdmissionAccepted, execution.AdmissionNotAdmitted} {
		t.Run(string(state), func(t *testing.T) {
			f := newModelFixture(t, "fixture-model")
			ctx := context.Background()
			starts, seals := 0, 0
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				starts++
				return execution.Ref{}, execution.ErrAdmissionUnknown
			}
			if err := f.adapter.Dispatch(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrAdmissionUnknown) {
				t.Fatal(err)
			}
			f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
				if f.read(t).ExecutionRef != "" {
					return f.turn(nil), nil
				}
				return execution.Turn{}, execution.ErrNotFound
			}
			f.executor.SealAttemptFunc = func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.AdmissionResolution, error) {
				seals++
				if scope != f.scope || !reflect.DeepEqual(caller, occurrenceCaller(f.occ)) {
					t.Fatal("retry changed identity")
				}
				r := execution.AdmissionResolution{State: state}
				if state == execution.AdmissionAccepted {
					r.Ref = f.ref
				}
				return r, nil
			}
			adapter, err := NewConversationAdapter(&failResolutionWrite{RecordStore: f.records}, f.executor)
			if err != nil {
				t.Fatal(err)
			}
			if err = adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if o := f.read(t); o.Released || o.ExecutionRef != "" || o.Status != "admission_unknown" {
				t.Fatalf("failed write lost occupancy: %+v", o)
			}
			// A replacement coordinator retries the same immutable public resolution.
			coordinator, err := NewCoordinator(f.records, f.adapter, func() time.Time { return f.occ.ScheduledAt })
			if err != nil {
				t.Fatal(err)
			}
			if err = coordinator.Recover(ctx, f.scope); err != nil {
				t.Fatal(err)
			}
			o := f.read(t)
			if seals != 2 || starts != 1 || o.Released != (state == execution.AdmissionNotAdmitted) {
				t.Fatalf("retry=%+v starts=%d seals=%d", o, starts, seals)
			}
		})
	}
}
