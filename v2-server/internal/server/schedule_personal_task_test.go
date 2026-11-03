package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/taskrun"
)

// One due instant must create one run on the assigned task. Later ticks, manual
// retries and recovery must all keep its identity and the original turn's result.
func TestSchedulePersonalTask(t *testing.T) {
	t.Run("accepted", func(t *testing.T) { testPersonalTask(t, false) })
	t.Run("start reply and first lookup lost", func(t *testing.T) { testPersonalTask(t, true) })
}
func testPersonalTask(t *testing.T, lost bool) {
	f := newWorkflowTargetFixture(t)
	ctx := t.Context()
	scope := f.scope
	src, e := f.db.CreateTask(ctx, scope.TenantID, scope.UserID, store.TaskInput{Title: "核对发布清单", ProjectID: f.project.ID, Assignee: &store.TaskAssigneeRef{Type: store.AssigneeHuman, ID: scope.UserID}})
	if e != nil {
		t.Fatal(e)
	}
	ref := execution.Ref{SessionID: "session", TurnID: "turn"}
	var called execution.Caller
	starts := 0
	f.catalog.ListExecutorsFunc = func(context.Context, execution.Scope) ([]execution.Executor, error) {
		return []execution.Executor{{ID: "fixture", Name: "fixture"}}, nil
	}
	f.catalog.StartFunc = func(_ context.Context, s execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		if s != scope || in.Target.ProjectID != src.ProjectID || in.ExecutorID != "fixture" {
			t.Fatalf("start scope/target: %+v %+v", s, in)
		}
		starts++
		called = in.Caller
		if lost {
			return execution.Ref{}, context.DeadlineExceeded
		}
		return ref, nil
	}
	turn := func() execution.Turn {
		return execution.Turn{Ref: ref, Result: &execution.Result{Status: execution.StatusCompleted}}
	}
	firstLookup := true
	f.catalog.ReadAttemptFunc = func(_ context.Context, s execution.Scope, c execution.Caller) (execution.Turn, error) {
		if c.Kind != execution.CallerTask || c.ID != called.ID || c.AttemptID != called.AttemptID || s != scope {
			t.Fatalf("unrelated attempt: %+v", c)
		}
		if firstLookup && lost {
			firstLookup = false
			return execution.Turn{}, execution.ErrUnavailable
		}
		return turn(), nil
	}
	f.catalog.ReadTurnsFunc = func(context.Context, execution.Scope, string, string, int) (execution.TurnPage, error) {
		return execution.TurnPage{Items: []execution.Turn{turn()}}, nil
	}
	f.catalog.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
		return endedStream{}, nil
	}
	host := taskrun.New(f.db, f.catalog, f.catalog, nil)
	defer host.Close()
	opts := f.options(false)
	opts.TaskRuns = host
	http := f.client(t, opts)
	cfg := f.config(src.ProjectID, map[string]any{"kind": "task", "taskId": src.ID, "executorId": "fixture", "note": "核对附件"})
	due := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	cfg["rule"] = map[string]any{"kind": "once", "time": due.Format("15:04"), "date": due.Format("2006-01-02"), "timezone": "UTC", "days": []int{}, "hours": 1}
	saved := http("POST", "/schedule-configs", cfg)
	if saved.Code != 201 {
		t.Fatalf("save: %d %s", saved.Code, saved.Body)
	}
	var plan store.ScheduleConfig
	if e := json.Unmarshal(saved.Body.Bytes(), &plan); e != nil {
		t.Fatal(e)
	}
	now := due.Add(-time.Minute)
	coord, e := server.NewScheduleCoordinator(f.db, opts, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	if e = coord.Recover(ctx, scope); e != nil {
		t.Fatal(e)
	}
	if e = coord.Tick(ctx, scope); e != nil {
		t.Fatal(e)
	}
	now = due
	if e = coord.Tick(ctx, scope); e != nil && !(lost && errors.Is(e, context.DeadlineExceeded)) {
		t.Fatal(e)
	}
	if starts != 1 {
		t.Fatalf("starts=%d", starts)
	}
	oid := strings.TrimPrefix(called.AttemptID, store.ScheduledTaskStartPrefix)
	// Recovery follows the original run without replaying the first turn.
	restarted, e := server.NewScheduleCoordinator(f.db, opts, func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	if e = restarted.Recover(ctx, scope); e != nil {
		t.Fatal(e)
	}
	o, e := f.db.GetOccurrence(ctx, scope.TenantID, scope.UserID, oid)
	if e != nil || o.Status != "succeeded" || !o.Released {
		t.Fatalf("occurrence: %+v %v", o, e)
	}
	var link struct {
		TaskID    string
		TaskRunID string
	}
	if e = json.Unmarshal([]byte(o.ExecutionRef), &link); e != nil || link.TaskID != src.ID || link.TaskRunID != called.ID {
		t.Fatalf("link: %s %v", o.ExecutionRef, e)
	}
	r, e := f.db.TaskRun(ctx, scope.TenantID, src.ID, link.TaskRunID)
	if e != nil || !r.Scheduled || r.StartedBy != scope.UserID {
		t.Fatalf("task run: %+v %v", r, e)
	}
	now = due.Add(24 * time.Hour)
	if e = coord.Tick(ctx, scope); e != nil {
		t.Fatal(e)
	}
	if starts != 1 {
		t.Fatalf("single shot started %d times", starts)
	}
	// Reassignment after save rejects the next manual trigger, and a retry has no run link.
	current, e := f.db.Task(ctx, scope.TenantID, scope.UserID, src.ID)
	if e != nil {
		t.Fatal(e)
	}
	// Wait for the asynchronous task watcher to deliver the completed turn.
	for i := 0; i < 100 && current.Status != store.TaskReview; i++ {
		time.Sleep(10 * time.Millisecond)
		current, e = f.db.Task(ctx, scope.TenantID, scope.UserID, src.ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	if current.Status != store.TaskReview {
		t.Fatalf("task status: %s", current.Status)
	}
	review, e := coord.Manual(ctx, scope, plan.ID, "in review")
	if e != nil || review.Reason != "task_no_longer_actionable" || review.ExecutionRef != "" {
		t.Fatalf("review: %+v %v", review, e)
	}
	_, e = f.db.SetTaskAssignee(ctx, scope.TenantID, scope.UserID, src.ID, nil, current.Version)
	if e != nil {
		t.Fatal(e)
	}
	skipped, e := coord.Manual(ctx, scope, plan.ID, "reassigned")
	if e != nil || skipped.Status != "skipped" || skipped.Reason != "task_assignment_changed" || skipped.ExecutionRef != "" {
		t.Fatalf("reassignment: %+v %v", skipped, e)
	}
	replay, e := coord.Manual(ctx, scope, plan.ID, "reassigned")
	if e != nil || replay.ID != skipped.ID || starts != 1 {
		t.Fatalf("retry: %+v %v", replay, e)
	}
	refused := http("POST", "/schedule-configs", cfg)
	if refused.Code != 400 {
		t.Fatalf("save reassigned task: %d %s", refused.Code, refused.Body)
	}
}

func TestSchedulePersonalTaskBusyAndCancel(t *testing.T) {
	f := newWorkflowTargetFixture(t)
	ctx := t.Context()
	scope := f.scope
	original, err := f.db.CreateTask(ctx, scope.TenantID, scope.UserID, store.TaskInput{Title: "test", ProjectID: f.project.ID, Assignee: &store.TaskAssigneeRef{Type: store.AssigneeHuman, ID: scope.UserID}})
	if err != nil {
		t.Fatal(err)
	}
	rt := f.catalog
	rt.ListExecutorsFunc = func(context.Context, execution.Scope) ([]execution.Executor, error) {
		return []execution.Executor{{ID: "fixture", Name: "Fixture"}}, nil
	}
	ref := execution.Ref{SessionID: "task-session", TurnID: "initial"}
	started, cancelled := 0, 0
	rt.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
		started++
		return ref, nil
	}
	rt.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		turn := execution.Turn{Ref: ref}
		if cancelled > 0 {
			turn.Result = &execution.Result{Status: execution.StatusCancelled}
		}
		return turn, nil
	}
	rt.CancelFunc = func(_ context.Context, s execution.Scope, turn execution.Ref) (execution.Result, error) {
		if s != scope || turn != ref {
			t.Fatalf("cancel touched another turn: %+v %+v", s, turn)
		}
		cancelled++
		return execution.Result{Status: execution.StatusCancelled}, nil
	}
	rt.EndFunc = func(context.Context, execution.Scope, string) error { return nil }
	host := taskrun.New(f.db, rt, rt, nil)
	defer host.Close()
	opts := f.options(false)
	opts.TaskRuns = host
	c, err := server.NewScheduleCoordinator(f.db, opts, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Recover(ctx, scope); err != nil {
		t.Fatal(err)
	}
	cfg := f.config(f.project.ID, map[string]any{"kind": "task", "taskId": original.ID, "executorId": "fixture", "note": ""})
	create := func() string {
		t.Helper()
		rec := f.client(t, opts)("POST", "/schedule-configs", cfg)
		if rec.Code != 201 {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
		var p store.ScheduleConfig
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	firstID := create()
	first, err := c.Manual(ctx, scope, firstID, "start")
	if err != nil || first.Status != "running" {
		t.Fatalf("first: %+v %v", first, err)
	}
	again, err := c.Manual(ctx, scope, firstID, "start")
	if err != nil || again.ID != first.ID || again.ExecutionRef != first.ExecutionRef {
		t.Fatalf("replay: %+v %v", again, err)
	}
	skipped, err := c.Manual(ctx, scope, create(), "competing")
	if err != nil || skipped.Status != "skipped" || skipped.Reason != "execution_busy" || skipped.ExecutionRef != "" {
		t.Fatalf("busy: %+v %v", skipped, err)
	}
	if err = c.Cancel(ctx, scope, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = c.Tick(ctx, scope); err != nil {
		t.Fatal(err)
	}
	done, err := f.db.GetOccurrence(ctx, scope.TenantID, scope.UserID, first.ID)
	if err != nil || done.Status != "cancelled" || !done.Released || done.ExecutionRef != first.ExecutionRef || started != 1 || cancelled != 1 {
		t.Fatalf("cancelled: %+v %v starts=%d cancels=%d", done, err, started, cancelled)
	}
}
