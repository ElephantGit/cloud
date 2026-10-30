package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/schedule"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type coordFixture struct {
	mu                                     sync.Mutex
	configs                                map[execution.Scope][]store.ScheduleConfig
	items                                  map[string]store.Occurrence
	owners                                 map[string]execution.Scope
	dispatched                             []string
	reconciled                             []string
	dispatchErr, reconcileErr, recoveryErr error
}

func newCoordFixture() *coordFixture {
	return &coordFixture{configs: map[execution.Scope][]store.ScheduleConfig{}, items: map[string]store.Occurrence{}, owners: map[string]execution.Scope{}}
}
func (f *coordFixture) ScheduleConfigs(_ context.Context, tenant, user string) ([]store.ScheduleConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.ScheduleConfig(nil), f.configs[execution.Scope{TenantID: tenant, UserID: user}]...), nil
}
func (f *coordFixture) GetOccurrence(_ context.Context, tenant, user, id string) (store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.get(tenant, user, id)
}
func (f *coordFixture) get(tenant, user, id string) (store.Occurrence, error) {
	o, ok := f.items[id]
	if !ok || f.owners[id] != (execution.Scope{TenantID: tenant, UserID: user}) {
		return o, store.ErrNotFound
	}
	return o, nil
}
func (f *coordFixture) FindManualOccurrence(_ context.Context, tenant, user, scheduleID, key string) (store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, o := range f.items {
		if o.ScheduleID == scheduleID && o.Kind == "manual" && o.Key == key {
			if v, e := f.get(tenant, user, id); e == nil {
				return v, nil
			}
		}
	}
	return store.Occurrence{}, store.ErrNotFound
}
func (f *coordFixture) RecordOccurrence(_ context.Context, tenant, user, id string, tr store.Trigger, now time.Time) (store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	scope := execution.Scope{TenantID: tenant, UserID: user}
	key := tr.Key
	if tr.Kind == "scheduled" {
		key = tr.At.Format(time.RFC3339Nano)
	}
	for oid, o := range f.items {
		if f.owners[oid] == scope && o.ScheduleID == id && o.Kind == tr.Kind && o.Key == key {
			return o, nil
		}
	}
	status, reason := "recorded", ""
	for oid, o := range f.items {
		if f.owners[oid] == scope && o.ScheduleID == id && !o.Released {
			status, reason = "skipped", "previous_unfinished"
		}
	}
	o := store.Occurrence{ID: fmt.Sprintf("occ-%d", len(f.items)+1), ScheduleID: id, Kind: tr.Kind, Key: key, ScheduledAt: tr.At, ConfigVersion: tr.Version, RecordedAt: now, Status: status, Reason: reason, Released: true, Version: 1}
	f.items[o.ID] = o
	f.owners[o.ID] = scope
	return o, nil
}
func (f *coordFixture) selectItems(tenant, user string, recoverable bool) []store.Occurrence {
	out := []store.Occurrence{}
	for id, o := range f.items {
		if f.owners[id] == (execution.Scope{TenantID: tenant, UserID: user}) && (!o.Released || (recoverable && o.Status == "recorded")) {
			out = append(out, o)
		}
	}
	return out
}
func (f *coordFixture) ActiveOccurrences(_ context.Context, tenant, user string) ([]store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.selectItems(tenant, user, false), nil
}
func (f *coordFixture) RecoverableOccurrences(_ context.Context, tenant, user string) ([]store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.selectItems(tenant, user, true), nil
}
func (f *coordFixture) ApplyOccurrenceEvidence(_ context.Context, tenant, user, id string, version int64, e store.ExecutionEvidence) (store.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recoveryErr != nil {
		return store.Occurrence{}, f.recoveryErr
	}
	o, err := f.get(tenant, user, id)
	if err != nil {
		return o, err
	}
	if o.Version != version {
		return o, store.ErrConflict
	}
	if e.Event != "recover" {
		return o, errors.New("unexpected evidence")
	}
	if o.Status == "recorded" {
		o.Status, o.Reason = "skipped", "server_restarted"
	} else if !o.Released {
		o.Status = "reconciling"
	}
	o.Version++
	f.items[id] = o
	return o, nil
}
func (f *coordFixture) Dispatch(_ context.Context, _ execution.Scope, o store.Occurrence) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatched = append(f.dispatched, o.ID)
	if f.dispatchErr == nil {
		o.Status, o.Released = "running", false
		o.Version++
		f.items[o.ID] = o
	}
	return f.dispatchErr
}
func (f *coordFixture) Reconcile(_ context.Context, _ execution.Scope, o store.Occurrence) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconciled = append(f.reconciled, o.ID)
	return f.reconcileErr
}
func (f *coordFixture) Cancel(context.Context, execution.Scope, store.Occurrence) error { return nil }

var coordScope = execution.Scope{TenantID: "tenant-a", UserID: "user-a"}

func coordTime(s string) time.Time {
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return v
}
func coordConfig(id string) store.ScheduleConfig {
	return store.ScheduleConfig{ID: id, Name: id, Version: 1, Enabled: true, CreatedAt: coordTime("2026-10-29T00:00:00Z"), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)}
}
func coordService(t *testing.T, f *coordFixture, clock *time.Time) *Coordinator {
	t.Helper()
	c, e := NewCoordinator(f, f, func() time.Time { return *clock })
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestCoordinatorRecoveryGateAndConcurrentTicks(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	f.configs[coordScope] = []store.ScheduleConfig{coordConfig("one")}
	clock := coordTime("2026-10-29T08:30:00Z")
	c := coordService(t, f, &clock)
	if e := c.Tick(ctx, coordScope); !errors.Is(e, ErrRecoveryRequired) {
		t.Fatalf("tick before recovery: %v", e)
	}
	if _, e := c.Manual(ctx, coordScope, "one", "key"); !errors.Is(e, ErrRecoveryRequired) {
		t.Fatalf("manual before recovery: %v", e)
	}
	if e := c.Recover(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if len(f.items) != 0 {
		t.Fatal("first discovery backfilled")
	}
	clock = coordTime("2026-10-29T09:00:00Z")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- c.Tick(ctx, coordScope) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(f.items) != 1 || len(f.dispatched) != 1 {
		t.Fatalf("concurrent tick duplicate: %d records %d dispatches", len(f.items), len(f.dispatched))
	}
	clock = coordTime("2026-10-29T12:05:00Z")
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if len(f.items) != 2 {
		t.Fatal("late tick replayed a backlog")
	}
	latest := f.items["occ-2"]
	if !latest.ScheduledAt.Equal(coordTime("2026-10-29T12:00:00Z")) || latest.Status != "skipped" {
		t.Fatalf("late tick %+v", latest)
	}
}

func TestCoordinatorPauseResumeAndScopeIsolation(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	f.configs[coordScope] = []store.ScheduleConfig{coordConfig("one")}
	clock := coordTime("2026-10-29T08:30:00Z")
	c := coordService(t, f, &clock)
	_ = c.Recover(ctx, coordScope)
	_ = c.Tick(ctx, coordScope)
	config := f.configs[coordScope][0]
	config.Enabled = false
	config.Version = 2
	f.configs[coordScope] = []store.ScheduleConfig{config}
	clock = coordTime("2026-10-29T09:00:00Z")
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	config.Enabled = true
	config.Version = 3
	f.configs[coordScope] = []store.ScheduleConfig{config}
	clock = coordTime("2026-10-29T10:15:00Z")
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if len(f.items) != 0 {
		t.Fatal("resume caught up paused work")
	}
	clock = coordTime("2026-10-29T11:00:00Z")
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if len(f.items) != 1 || f.items["occ-1"].ConfigVersion != 3 {
		t.Fatal("resumed anchor/version wrong")
	}
	other := execution.Scope{TenantID: "tenant-a", UserID: "user-b"}
	f.configs[other] = []store.ScheduleConfig{coordConfig("one")}
	if e := c.Tick(ctx, other); !errors.Is(e, ErrRecoveryRequired) {
		t.Fatal("scope inherited readiness")
	}
	_ = c.Recover(ctx, other)
	_ = c.Tick(ctx, other)
	if len(f.items) != 1 {
		t.Fatal("scope inherited old cursor")
	}
}

func TestCoordinatorUnknownManualRetryKeepsTimeAndNeverRedispatches(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	config := coordConfig("one")
	config.Enabled = false
	f.configs[coordScope] = []store.ScheduleConfig{config}
	clock := coordTime("2026-10-29T08:30:00Z")
	c := coordService(t, f, &clock)
	_ = c.Recover(ctx, coordScope)
	f.dispatchErr = errors.New("lookup failed")
	first, e := c.Manual(ctx, coordScope, "one", "stable")
	if e == nil {
		t.Fatal("dispatch error hidden")
	}
	// ConversationAdapter persists reconciliation state before returning that error. This fixture
	// only models dispatch decisions, so install the durable state explicitly here.
	unknown := f.items[first.ID]
	unknown.Status, unknown.Released = "reconciling", false
	f.items[first.ID] = unknown
	clock = clock.Add(time.Hour)
	config.Version = 2
	f.configs[coordScope] = []store.ScheduleConfig{config}
	second, e := c.Manual(ctx, coordScope, "one", "stable")
	if e != nil {
		t.Fatal(e)
	}
	if first.ID != second.ID || !first.ScheduledAt.Equal(second.ScheduledAt) || second.ConfigVersion != 1 || len(f.dispatched) != 1 {
		t.Fatal("manual retry changed identity/time or redispatched")
	}
	if _, e = c.Manual(ctx, coordScope, "one", ""); !errors.Is(e, store.ErrIneligible) {
		t.Fatal("empty key accepted")
	}
}

func TestCoordinatorRecordedManualRetryContinuesFirstAdmission(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	f.configs[coordScope] = []store.ScheduleConfig{coordConfig("one")}
	clock := coordTime("2026-10-29T08:30:00Z")
	c := coordService(t, f, &clock)
	if err := c.Recover(ctx, coordScope); err != nil {
		t.Fatal(err)
	}
	f.dispatchErr = errors.New("database unavailable before begin")
	first, err := c.Manual(ctx, coordScope, "one", "stable")
	if err == nil || first.Status != "recorded" {
		t.Fatalf("first admission = %+v, %v", first, err)
	}
	clock = clock.Add(time.Hour)
	f.dispatchErr = nil
	second, err := c.Manual(ctx, coordScope, "one", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !first.ScheduledAt.Equal(second.ScheduledAt) || first.ConfigVersion != second.ConfigVersion || len(f.items) != 1 || second.Status != "running" {
		t.Fatalf("retry failed to continue original occurrence: first=%+v second=%+v", first, second)
	}
	if len(f.dispatched) != 2 || f.dispatched[0] != f.dispatched[1] {
		t.Fatal("retry changed dispatch identity")
	}
	if _, err = c.Manual(ctx, coordScope, "one", "stable"); err != nil {
		t.Fatal(err)
	}
	if len(f.dispatched) != 2 || len(f.reconciled) != 1 {
		t.Fatal("started occurrence was dispatched again")
	}
}

func TestCoordinatorDispatchFailureDoesNotRepeatSameScheduledRun(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	f.configs[coordScope] = []store.ScheduleConfig{coordConfig("one")}
	clock := coordTime("2026-10-29T08:30:00Z")
	c := coordService(t, f, &clock)
	_ = c.Recover(ctx, coordScope)
	_ = c.Tick(ctx, coordScope)
	clock = coordTime("2026-10-29T09:00:00Z")
	f.dispatchErr = errors.New("unknown")
	if e := c.Tick(ctx, coordScope); e == nil {
		t.Fatal("dispatch error hidden")
	}
	if e := c.Tick(ctx, coordScope); e != nil {
		t.Fatal(e)
	}
	if len(f.dispatched) != 1 || len(f.items) != 1 {
		t.Fatal("same failed trigger redispatched")
	}
}

func TestCoordinatorRestartRecoversWithoutReplayingAndDoesNotBlockOtherTasks(t *testing.T) {
	ctx := context.Background()
	f := newCoordFixture()
	f.configs[coordScope] = []store.ScheduleConfig{coordConfig("old"), coordConfig("new")}
	clock := coordTime("2026-10-29T08:30:00Z")
	unsent, _ := f.RecordOccurrence(ctx, coordScope.TenantID, coordScope.UserID, "old", store.Trigger{Kind: "manual", Key: "unsent", At: clock, Version: 1}, clock)
	active, _ := f.RecordOccurrence(ctx, coordScope.TenantID, coordScope.UserID, "old", store.Trigger{Kind: "manual", Key: "active", At: clock, Version: 1}, clock)
	_ = f.Dispatch(ctx, coordScope, active)
	f.dispatched = nil
	f.recoveryErr = errors.New("database unavailable")
	c := coordService(t, f, &clock)
	if e := c.Recover(ctx, coordScope); e == nil {
		t.Fatal("recovery write error hidden")
	}
	if e := c.Tick(ctx, coordScope); !errors.Is(e, ErrRecoveryRequired) {
		t.Fatal("failed recovery admitted work")
	}
	f.recoveryErr = nil
	f.reconcileErr = errors.New("execution unavailable")
	if e := c.Recover(ctx, coordScope); e == nil {
		t.Fatal("reconcile error hidden")
	}
	if f.items[unsent.ID].Status != "skipped" || f.items[active.ID].Status != "reconciling" {
		t.Fatal("wrong recovery states")
	}
	_ = c.Tick(ctx, coordScope)
	clock = coordTime("2026-10-29T09:00:00Z")
	if e := c.Tick(ctx, coordScope); e == nil {
		t.Fatal("active lookup failure not reported")
	}
	if len(f.dispatched) != 1 || f.items[f.dispatched[0]].ScheduleID != "new" {
		t.Fatal("unresolved old task blocked unrelated task or replayed")
	}
}

func TestCalendarLateTickUsesLatestDue(t *testing.T) {
	rule := schedule.Rule{Kind: "daily", Time: "09:00", Timezone: "Asia/Shanghai"}
	created := coordTime("2026-01-01T00:00:00Z")
	first := coordTime("2026-10-01T01:00:00Z")
	now := coordTime("2026-10-29T12:05:00Z")
	got, e := mostRecentDue(rule, created, first, now)
	if e != nil || !got.Equal(coordTime("2026-10-29T01:00:00Z")) {
		t.Fatalf("latest due %v %v", got, e)
	}
}
