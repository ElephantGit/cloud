package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestLookupCannotOvertakeStart(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		close(entered)
		<-release
		return f.ref, nil
	}
	reads := 0
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		reads++
		return f.turn(nil), nil
	}
	done := make(chan error, 1)
	go func() { done <- f.adapter.Dispatch(ctx, f.scope, f.occ) }()
	<-entered
	err := f.adapter.Reconcile(ctx, f.scope, f.occ)
	cancelErr := f.adapter.Cancel(ctx, f.scope, f.occ)
	close(release)
	startErr := <-done
	if !errors.Is(err, ErrOperationInProgress) || !errors.Is(cancelErr, ErrOperationInProgress) || startErr != nil || reads != 0 {
		t.Fatalf("lookup overtook start: %v / %v / %v, reads=%d", err, cancelErr, startErr, reads)
	}
	if err = f.adapter.Reconcile(ctx, f.scope, f.occ); err != nil || reads != 1 {
		t.Fatalf("subsequent observation: %v reads=%d", err, reads)
	}
}

type failBindingOnce struct {
	RecordStore
	failed bool
}

func (r *failBindingOnce) ApplyOccurrenceEvidence(ctx context.Context, tenant, user, id string, version int64, proof store.ExecutionEvidence) (store.Occurrence, error) {
	if proof.Event == "accepted" && !r.failed {
		r.failed = true
		return store.Occurrence{}, errors.New("local binding write failed")
	}
	return r.RecordStore.ApplyOccurrenceEvidence(ctx, tenant, user, id, version, proof)
}

func TestStartupRecoveryReadsOriginalAttemptWithoutReplay(t *testing.T) {
	for _, mode := range []string{"model", "agent"} {
		for _, accepted := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/missing", true: "/accepted"}[accepted], func(t *testing.T) {
				var f *modelFixture
				if mode == "agent" {
					f = newConversationFixture(t, agentProject, json.RawMessage(agentTarget))
				} else {
					f = newModelFixture(t, "fixture-model")
				}
				ctx := context.Background()
				starts := 0
				start := func() (execution.Ref, error) { starts++; return f.ref, nil }
				f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
					return start()
				}
				f.executor.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) { return start() }
				if accepted {
					adapter, err := NewConversationAdapter(&failBindingOnce{RecordStore: f.records}, f.executor)
					if err != nil {
						t.Fatal(err)
					}
					if err = adapter.Dispatch(ctx, f.scope, f.occ); err == nil {
						t.Fatal("missing injected write failure")
					}
				} else {
					// Simulate process exit after begin, before the synchronous Start call.
					if _, err := f.records.ApplyOccurrenceEvidence(ctx, f.scope.TenantID, f.scope.UserID, f.occ.ID, f.occ.Version, store.ExecutionEvidence{Event: "begin"}); err != nil {
						t.Fatal(err)
					}
				}
				f.records.Close()
				reopened, err := store.OpenConfig(ctx, f.config)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				f.records = reopened
				f.adapter, err = NewConversationAdapter(reopened, f.executor)
				if err != nil {
					t.Fatal(err)
				}
				reads, ends := 0, 0
				f.executor.ReadAttemptFunc = func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
					reads++
					if scope != f.scope || caller.ID != f.occ.ID || caller.AttemptID != f.occ.ID {
						t.Fatal("changed recovery identity")
					}
					if !accepted {
						return execution.Turn{}, execution.ErrNotFound
					}
					// The public startup barrier has already interrupted old active turns.
					return f.turn(&execution.Result{Status: execution.StatusInterrupted, FailureCode: "server_restarted"}), nil
				}
				f.executor.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
					ends++
					if id != f.ref.SessionID {
						t.Fatal("wrong End")
					}
					return nil
				}
				coordinator, err := NewCoordinator(reopened, f.adapter, func() time.Time { return f.occ.ScheduledAt.Add(24 * time.Hour) })
				if err != nil {
					t.Fatal(err)
				}
				if err = coordinator.Recover(ctx, f.scope); err != nil {
					t.Fatal(err)
				}
				if err = coordinator.Tick(ctx, f.scope); err != nil {
					t.Fatal(err)
				}
				o := f.read(t)
				if !o.Released || reads != 1 {
					t.Fatalf("recovery: %+v reads=%d", o, reads)
				}
				if accepted && (o.Status != "interrupted" || starts != 1 || ends != 1) {
					t.Fatalf("accepted: %+v starts=%d ends=%d", o, starts, ends)
				}
				if !accepted && (o.Status != "skipped" || o.Reason != "execution_not_started" || starts != 0 || ends != 0) {
					t.Fatalf("missing: %+v starts=%d ends=%d", o, starts, ends)
				}
				retry, err := coordinator.Manual(ctx, f.scope, o.ScheduleID, o.Key)
				if err != nil || retry.ID != o.ID || retry.Status != o.Status {
					t.Fatalf("same key replayed: %+v %v", retry, err)
				}
			})
		}
	}
}

func TestBoundMissingAttemptKeepsSlot(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		return f.ref, nil
	}
	if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{}, execution.ErrNotFound
	}
	if err := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrNotFound) {
		t.Fatal(err)
	}
	if o := f.read(t); o.Released || o.ExecutionRef == "" || o.Status != "running" {
		t.Fatal(o)
	}
}

func TestSchemaReopenPreservesIdentityAndOccupancy(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, f.config.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `UPDATE v2_schedule_occurrences SET status='reconciling',reason='lookup_required',released=false,version=3`)
	if err != nil {
		t.Fatal(err)
	}
	f.records.Close()
	for i := 0; i < 2; i++ {
		reopened, err := store.OpenConfig(ctx, f.config)
		if err != nil {
			t.Fatal(err)
		}
		f.records = reopened
		o := f.read(t)
		if o.ID != f.occ.ID || o.Key != f.occ.Key || string(o.Snapshot) != string(f.occ.Snapshot) || o.Status != "reconciling" || o.Reason != "lookup_required" || o.Released || o.Version != 3 {
			t.Fatalf("reopen lost evidence: %+v", o)
		}
		if _, err = conn.Exec(ctx, `UPDATE v2_schedule_occurrences SET status='admission_unknown'`); err == nil {
			t.Fatal("obsolete state still accepted")
		}
		reopened.Close()
	}
}
