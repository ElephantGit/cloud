package scheduler

import (
	"context"
	"errors"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"testing"
	"time"
)

// Real scheduling persistence and the actual adapter share a fake execution service.
// This verifies cross-module effects rather than declaring Stub responses to be execution.
func TestCoordinatorConversationAdapterLookupThroughCleanup(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	now := f.occ.ScheduledAt
	starts, ends := 0, 0
	var caller execution.Caller
	f.executor.StartModelFunc = func(_ context.Context, _ execution.Scope, r execution.ModelStartRequest) (execution.Ref, error) {
		starts++
		caller = r.Caller
		return execution.Ref{}, execution.ErrConflict
	}
	f.executor.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{}, execution.ErrUnavailable
	}
	c, e := NewCoordinator(f.records, f.adapter, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Recover(ctx, f.scope); e != nil {
		t.Fatal(e)
	}
	if e = c.Tick(ctx, f.scope); e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Hour)
	if e = c.Tick(ctx, f.scope); !errors.Is(e, execution.ErrConflict) {
		t.Fatalf("start outcome %v", e)
	}
	if starts != 1 {
		t.Fatalf("starts=%d", starts)
	}
	o, e := f.records.GetOccurrence(ctx, f.scope.TenantID, f.scope.UserID, caller.ID)
	if e != nil || o.Status != "reconciling" || o.Released {
		t.Fatalf("unknown=%+v %v", o, e)
	}
	now = now.Add(time.Hour)
	if e = c.Tick(ctx, f.scope); !errors.Is(e, execution.ErrUnavailable) {
		t.Fatalf("missing outcome %v", e)
	}
	items, e := f.records.Occurrences(ctx, f.scope.TenantID, f.scope.UserID, f.occ.ScheduleID, store.OccurrenceCursor{}, 100)
	if e != nil {
		t.Fatal(e)
	}
	overlaps := 0
	for _, v := range items {
		if v.Reason == "previous_unfinished" {
			overlaps++
		}
	}
	if starts != 1 || overlaps != 1 {
		t.Fatalf("readmission=%d overlaps=%d", starts, overlaps)
	}
	// A new coordinator recovers durable unknown state without another StartModel.
	c, e = NewCoordinator(f.records, f.adapter, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Recover(ctx, f.scope); !errors.Is(e, execution.ErrUnavailable) {
		t.Fatalf("recovery=%v", e)
	}
	f.executor.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, got execution.Caller) (execution.Turn, error) {
		if got.ID != caller.ID || got.AttemptID != caller.AttemptID {
			t.Fatal("identity changed")
		}
		return execution.Turn{Caller: got, Ref: f.ref, Result: &execution.Result{Status: execution.StatusCompleted, Answer: "complete", AnswerComplete: true}}, nil
	}
	f.executor.EndFunc = func(context.Context, execution.Scope, string) error {
		ends++
		if ends == 1 {
			return execution.ErrStopUnconfirmed
		}
		return nil
	}
	if e = c.Tick(ctx, f.scope); !errors.Is(e, execution.ErrStopUnconfirmed) {
		t.Fatalf("end outcome %v", e)
	}
	o, e = f.records.GetOccurrence(ctx, f.scope.TenantID, f.scope.UserID, caller.ID)
	if e != nil || o.Status != "succeeded" || o.Released {
		t.Fatalf("pending cleanup=%+v %v", o, e)
	}
	if e = c.Tick(ctx, f.scope); e != nil {
		t.Fatal(e)
	}
	o, e = f.records.GetOccurrence(ctx, f.scope.TenantID, f.scope.UserID, caller.ID)
	if e != nil || !o.Released || o.Status != "succeeded" || starts != 1 || ends != 2 {
		t.Fatalf("final=%+v err=%v starts=%d ends=%d", o, e, starts, ends)
	}
}

type beforeBeginFailure struct {
	Runner
	fail bool
}

func (r *beforeBeginFailure) Dispatch(ctx context.Context, scope execution.Scope, o store.Occurrence) error {
	if r.fail {
		r.fail = false
		return context.Canceled
	}
	return r.Runner.Dispatch(ctx, scope, o)
}

func TestCoordinatorManualRetryUsesOriginalRequest(t *testing.T) {
	f := newModelFixture(t, "fixture-model")
	ctx := context.Background()
	now := f.occ.ScheduledAt
	starts := 0
	f.executor.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		starts++
		return f.ref, nil
	}
	f.executor.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
		return execution.Turn{Caller: c, Ref: f.ref}, nil
	}
	c, e := NewCoordinator(f.records, &beforeBeginFailure{Runner: f.adapter, fail: true}, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Recover(ctx, f.scope); e != nil {
		t.Fatal(e)
	}
	first, e := c.Manual(ctx, f.scope, f.occ.ScheduleID, "request-key")
	if !errors.Is(e, context.Canceled) || first.Status != "recorded" || starts != 0 {
		t.Fatalf("pre-begin error %v %+v starts=%d", e, first, starts)
	}
	now = now.Add(time.Minute)
	retry, e := c.Manual(ctx, f.scope, f.occ.ScheduleID, "request-key")
	if e != nil {
		t.Fatal(e)
	}
	if first.ID != retry.ID || !first.ScheduledAt.Equal(retry.ScheduledAt) || starts != 1 {
		t.Fatalf("manual changed %v %v %d", first, retry, starts)
	}
	if _, e = c.Manual(ctx, f.scope, f.occ.ScheduleID, "request-key"); e != nil || starts != 1 {
		t.Fatalf("repeated retry %v starts=%d", e, starts)
	}
	if _, e = f.records.GetOccurrence(ctx, "00000000-0000-0000-0000-000000000001", f.scope.UserID, retry.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("foreign query %v", e)
	}
}
