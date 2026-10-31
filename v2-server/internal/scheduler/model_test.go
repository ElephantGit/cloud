package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type modelFixture struct {
	config   *pgxpool.Config
	records  *store.Store
	adapter  *ConversationAdapter
	executor *executiontest.Stub
	scope    execution.Scope
	occ      store.Occurrence
	ref      execution.Ref
}

func newModelFixture(t *testing.T, model string) *modelFixture {
	t.Helper()
	target, _ := json.Marshal(map[string]string{"kind": "chat", "prompt": "frozen prompt", "agent": "", "model": model})
	return newConversationFixture(t, "", target)
}

func newConversationFixture(t *testing.T, project string, target json.RawMessage) *modelFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	var b [8]byte
	_, _ = rand.Read(b[:])
	schema := "model_" + hex.EncodeToString(b[:])
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	records, err := store.OpenConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(records.Close)
	token, err := records.Login(ctx, store.Identity{Source: "test", Subject: "owner", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := records.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := records.Tenants(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
	saved, err := records.CreateScheduleConfig(ctx, scope.TenantID, scope.UserID, store.ScheduleInput{Name: "task", ProjectID: project, Target: target, Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	at := saved.CreatedAt.Add(time.Hour)
	occ, err := records.RecordOccurrence(ctx, scope.TenantID, scope.UserID, saved.ID, store.Trigger{Kind: "manual", Key: "first", At: at, Version: 1}, at)
	if err != nil {
		t.Fatal(err)
	}
	executor := &executiontest.Stub{}
	adapter, err := NewConversationAdapter(records, executor)
	if err != nil {
		t.Fatal(err)
	}
	return &modelFixture{config: config, records: records, adapter: adapter, executor: executor, scope: scope, occ: occ, ref: execution.Ref{SessionID: "session-1", TurnID: "turn-1"}}
}

func (f *modelFixture) read(t *testing.T) store.Occurrence {
	t.Helper()
	o, e := f.records.GetOccurrence(context.Background(), f.scope.TenantID, f.scope.UserID, f.occ.ID)
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func (f *modelFixture) turn(result *execution.Result) execution.Turn {
	return execution.Turn{Caller: occurrenceCaller(f.occ), Ref: f.ref, Result: result}
}

func TestModelDispatchOnceAndCleanupRetry(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	starts, reads, ends := 0, 0, 0
	f.executor.StartModelFunc = func(_ context.Context, scope execution.Scope, in execution.ModelStartRequest) (execution.Ref, error) {
		starts++
		text, e := in.Prompt.Text()
		if e != nil || text != "frozen prompt" || in.ModelID != "fixture-model" || in.Caller.Kind != execution.CallerSchedule || in.Caller.ID != f.occ.ID || in.Caller.AttemptID != f.occ.ID || in.Caller.NodeID != "" || len(in.Caller.Rounds) != 0 || scope != f.scope {
			t.Fatal("incorrect admission identity or snapshot")
		}
		return f.ref, nil
	}
	f.executor.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
		reads++
		return f.turn(&execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "complete answer"}), nil
	}
	f.executor.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
		ends++
		if id != f.ref.SessionID {
			t.Fatal("wrong session")
		}
		if ends == 1 {
			return execution.ErrStopUnconfirmed
		}
		return nil
	}
	if e := f.adapter.Dispatch(ctx, f.scope, f.occ); e != nil {
		t.Fatal(e)
	}
	if e := f.adapter.Dispatch(ctx, f.scope, f.occ); e != nil {
		t.Fatal(e)
	}
	if starts != 1 {
		t.Fatalf("admitted %d times", starts)
	}
	if e := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(e, execution.ErrStopUnconfirmed) {
		t.Fatalf("cleanup = %v", e)
	}
	o := f.read(t)
	if o.Status != "succeeded" || o.Released {
		t.Fatalf("unconfirmed cleanup = %+v", o)
	}
	if e := f.adapter.Reconcile(ctx, f.scope, f.occ); e != nil {
		t.Fatal(e)
	}
	if o = f.read(t); !o.Released || o.Status != "succeeded" || starts != 1 || reads != 1 || ends != 2 {
		t.Fatalf("retry changed result or admission: %+v %d/%d/%d", o, starts, reads, ends)
	}
}

func TestModelAdmissionFailures(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           error
		state, reason string
		released      bool
	}{
		{"busy", execution.ErrWorkspaceBusy, "skipped", "execution_busy", true},
		{"unavailable", execution.ErrUnavailable, "skipped", "execution_unavailable", true},
		{"conflict", execution.ErrConflict, "reconciling", "lookup_required", false},
		{"timeout", context.DeadlineExceeded, "reconciling", "lookup_required", false},
		{"cancelled", context.Canceled, "reconciling", "lookup_required", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newModelFixture(t, "fixture-model")
			calls := 0
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				calls++
				return execution.Ref{}, tc.err
			}
			if e := f.adapter.Dispatch(context.Background(), f.scope, f.occ); !errors.Is(e, tc.err) {
				t.Fatal(e)
			}
			o := f.read(t)
			if o.Status != tc.state || o.Reason != tc.reason || o.Released != tc.released {
				t.Fatalf("unexpected state %+v", o)
			}
			_ = f.adapter.Dispatch(context.Background(), f.scope, f.occ)
			if calls != 1 {
				t.Fatal("admission retried")
			}
		})
	}
}

func TestModelLookupFailureReconcilesWithoutReadmission(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	calls := 0
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		calls++
		return execution.Ref{}, execution.ErrConflict
	}
	_ = f.adapter.Dispatch(ctx, f.scope, f.occ)
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{}, execution.ErrUnavailable
	}
	if e := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(e, execution.ErrUnavailable) {
		t.Fatal(e)
	}
	o := f.read(t)
	if o.Released || o.Status != "reconciling" {
		t.Fatal("missing lookup released unknown")
	}
	o, e := f.records.ApplyOccurrenceEvidence(ctx, f.scope.TenantID, f.scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: "recover"})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.adapter.Reconcile(ctx, f.scope, o); !errors.Is(e, execution.ErrUnavailable) {
		t.Fatal(e)
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return f.turn(&execution.Result{Status: execution.StatusInterrupted, FailureCode: "server_restarted"}), nil
	}
	f.executor.EndFunc = func(context.Context, execution.Scope, string) error { return nil }
	if e = f.adapter.Reconcile(ctx, f.scope, o); e != nil {
		t.Fatal(e)
	}
	if o = f.read(t); o.Status != "interrupted" || !o.Released || calls != 1 {
		t.Fatalf("recovery %+v admissions %d", o, calls)
	}
}

func TestModelIncompleteAndCancel(t *testing.T) {
	for _, mode := range []string{"incomplete", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newModelFixture(t, "fixture-model")
			ctx := context.Background()
			f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
				return f.ref, nil
			}
			if e := f.adapter.Dispatch(ctx, f.scope, f.occ); e != nil {
				t.Fatal(e)
			}
			f.executor.EndFunc = func(context.Context, execution.Scope, string) error { return nil }
			if mode == "incomplete" {
				f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
					return f.turn(&execution.Result{Status: execution.StatusCompleted, Answer: "truncated"}), nil
				}
				if e := f.adapter.Reconcile(ctx, f.scope, f.occ); e != nil {
					t.Fatal(e)
				}
				o := f.read(t)
				if o.Status != "failed" || o.Reason != "result_incomplete" || !o.Released {
					t.Fatal(o)
				}
			} else {
				f.executor.CancelFunc = func(context.Context, execution.Scope, execution.Ref) (execution.Result, error) {
					return execution.Result{}, execution.ErrStopUnconfirmed
				}
				if e := f.adapter.Cancel(ctx, f.scope, f.occ); !errors.Is(e, execution.ErrStopUnconfirmed) {
					t.Fatal(e)
				}
				o := f.read(t)
				if o.Status != "cancelling" || o.Released {
					t.Fatal(o)
				}
				f.executor.CancelFunc = func(context.Context, execution.Scope, execution.Ref) (execution.Result, error) {
					return execution.Result{Status: execution.StatusCompleted, AnswerComplete: true}, nil
				}
				if e := f.adapter.Cancel(ctx, f.scope, f.occ); e != nil {
					t.Fatal(e)
				}
				o = f.read(t)
				if o.Status != "succeeded" || !o.Released {
					t.Fatal(o)
				}
			}
		})
	}
}

func TestModelMissingSelectionDoesNotAdmit(t *testing.T) {
	f := newModelFixture(t, "")
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		t.Fatal("unselected model admitted")
		return execution.Ref{}, nil
	}
	if e := f.adapter.Dispatch(context.Background(), f.scope, f.occ); e != nil {
		t.Fatal(e)
	}
	o := f.read(t)
	if o.Status != "skipped" || o.Reason != "model_required" || !o.Released {
		t.Fatal(o)
	}
}

func TestConcurrentModelDispatchAdmitsOnce(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	var calls atomic.Int32
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		calls.Add(1)
		return f.ref, nil
	}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); errs <- f.adapter.Dispatch(context.Background(), f.scope, f.occ) }()
	}
	group.Wait()
	close(errs)
	for e := range errs {
		if e != nil && !errors.Is(e, store.ErrConflict) && !errors.Is(e, ErrOperationInProgress) {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("admissions %d", calls.Load())
	}
}

func TestModelMismatchedAttemptNeverFinishesOrReleases(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		return f.ref, nil
	}
	if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	f.executor.EndFunc = func(context.Context, execution.Scope, string) error {
		t.Fatal("mismatched attempt ended session")
		return nil
	}
	for _, wrong := range []string{"caller", "ref"} {
		f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
			turn := f.turn(&execution.Result{Status: execution.StatusCompleted, AnswerComplete: true})
			if wrong == "caller" {
				turn.Caller.AttemptID = "another"
			} else {
				turn.Ref.TurnID = "another"
			}
			return turn, nil
		}
		if err := f.adapter.Reconcile(ctx, f.scope, f.occ); !errors.Is(err, execution.ErrConflict) {
			t.Fatalf("%s: %v", wrong, err)
		}
	}
	o := f.read(t)
	if o.Status != "running" || o.Released {
		t.Fatal(o)
	}
}

func TestModelAdmissionSurvivesObserverCancellation(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		cancel()
		return f.ref, nil
	}
	if err := f.adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	o := f.read(t)
	if o.Status != "running" || o.Released || o.ExecutionRef == "" {
		t.Fatal(o)
	}
}

// cancelAfterBeginRecords injects cancellation after the actual PostgreSQL reservation commits.
type cancelAfterBeginRecords struct {
	RecordStore
	cancel context.CancelFunc
}

func (r *cancelAfterBeginRecords) ApplyOccurrenceEvidence(ctx context.Context, tenant, user, id string, version int64, proof store.ExecutionEvidence) (store.Occurrence, error) {
	o, err := r.RecordStore.ApplyOccurrenceEvidence(ctx, tenant, user, id, version, proof)
	if err == nil && proof.Event == "begin" {
		r.cancel()
	}
	return o, err
}

func TestModelLocalRejectionSurvivesCancellationAfterReservation(t *testing.T) {
	f := newModelFixture(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	records := &cancelAfterBeginRecords{RecordStore: f.records, cancel: cancel}
	adapter, err := NewConversationAdapter(records, f.executor)
	if err != nil {
		t.Fatal(err)
	}
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		t.Fatal("invalid model selection reached public execution")
		return execution.Ref{}, nil
	}
	if err = adapter.Dispatch(ctx, f.scope, f.occ); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("test did not cancel the original context")
	}
	o := f.read(t)
	if o.Status != "skipped" || o.Reason != "model_required" || !o.Released {
		t.Fatalf("known local rejection retained reservation: %+v", o)
	}
}
