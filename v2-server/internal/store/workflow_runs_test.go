package store_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

func admitWorkflow(t *testing.T, f *executionFixture) (workflow.Document, workflow.Admission, workflow.Record) {
	t.Helper()
	d := createWorkflow(t, f, "Runs")
	snapshot, err := f.s.PublishWorkflow(t.Context(), f.scope, d.ID, "publish", workflow.PublishDocument{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.s.CreateProject(t.Context(), f.scope, store.Project{ID: "project", Name: "Project", RepositoryURL: "https://example.test/repo", Commit: "abc", MirrorPath: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	in := workflow.Admission{SnapshotID: snapshot.ID, ProjectID: project.ID, Input: workflow.Object{"input": "hello"}, Executors: map[string]string{"a": "executor"}}
	r, err := f.s.CreateWorkflowRun(t.Context(), f.scope, d.ID, "run", in)
	if err != nil {
		t.Fatal(err)
	}
	return d, in, r
}

func TestWorkflowRunAdmissionIsFrozenAndClaimedOnce(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	d, in, r := admitWorkflow(t, f)
	if r.Checkpoint.State.Status != "pending" || r.Revision != 1 || r.Scope != f.scope || r.Definition.ID != r.ID || r.Definition.ProjectID != in.ProjectID || r.SnapshotID != in.SnapshotID {
		t.Fatalf("admission: %+v", r)
	}
	const count = 8
	var wg sync.WaitGroup
	claims := make([]error, count)
	for i := range count {
		wg.Go(func() {
			again, err := f.s.CreateWorkflowRun(ctx, f.scope, d.ID, "run", in)
			if err != nil || again.ID != r.ID {
				t.Errorf("admission replay: %+v %v", again, err)
			}
			_, claims[i] = f.s.BeginWorkflowRun(ctx, f.scope, d.ID, r.ID)
		})
	}
	wg.Wait()
	winners := 0
	for _, err := range claims {
		if err == nil {
			winners++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("claimed %d times", winners)
	}
	if _, err := f.s.UpdateWorkflow(ctx, f.scope, d.ID, workflow.UpdateDocument{Name: "Changed", Version: &d.Version}); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.GetWorkflowRun(ctx, f.scope, d.ID, r.ID)
	if err != nil || !reflect.DeepEqual(current.Definition.Graph, r.Definition.Graph) || current.Checkpoint.State.Status != "running" {
		t.Fatalf("frozen run changed: %+v %v", current, err)
	}
	in.Input["input"] = "changed"
	if _, err := f.s.CreateWorkflowRun(ctx, f.scope, d.ID, "run", in); !errors.Is(err, store.ErrKeyReused) {
		t.Fatalf("changed admission reused key: %v", err)
	}
}

func TestWorkflowAttemptJournalPreservesReceiptsAndFencesWrites(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	d, _, r := admitWorkflow(t, f)
	r, err := f.s.BeginWorkflowRun(ctx, f.scope, d.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := workflow.Checkpoint{State: workflow.State{Status: "running", Nodes: workflow.Object{"a": workflow.Object{"status": "running"}}}, Active: &workflow.Attempt{Caller: execution.Caller{Kind: execution.CallerWorkflow, ID: r.ID, NodeID: "a", AttemptID: "1"}, ExecutorID: "executor", Prompt: "hello"}}
	// A receipt cannot precede the durable dispatch intent.
	c.Ref = execution.Ref{SessionID: "session", TurnID: "turn"}
	if _, err := f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("unrecorded dispatch: %v", err)
	}
	c.Ref = execution.Ref{}
	r, err = f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Ref = execution.Ref{SessionID: "session", TurnID: "turn"}
	if _, err := f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision-1, c); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale worker: %v", err)
	}
	r, err = f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Ref.TurnID = "replacement"
	if _, err := f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("receipt replaced: %v", err)
	}
	c.Ref = execution.Ref{}
	c.Active = nil
	c.State.Nodes["a"] = workflow.Object{"status": "succeeded", "output": workflow.Object{"output": "answer"}}
	r, err = f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	c.State.Status = "succeeded"
	r, err = f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c)
	if err != nil || r.FinishedAt == nil {
		t.Fatalf("terminal: %+v %v", r, err)
	}
	f.s.Close()
	f.s, err = store.OpenConfig(ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListWorkflowAttempts(ctx, f.scope, d.ID, r.ID, 0, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].Ref.TurnID != "turn" || page.Items[0].SettledAt == nil {
		t.Fatalf("retained receipt: %+v %v", page, err)
	}
	if _, err := f.s.CommitWorkflowRun(ctx, f.scope, d.ID, r.ID, r.Revision, c); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("terminal rewritten: %v", err)
	}
	if _, err := f.s.BeginWorkflowRun(ctx, f.scope, d.ID, r.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("terminal restarted: %v", err)
	}
}

func TestWorkflowRunCancellationRecoveryAndCurrentOwnership(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	d, in, r := admitWorkflow(t, f)
	other := executionMember(t, f.s, "other")
	if _, err := f.admin.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT(tenant_id,user_id) DO UPDATE SET role='member'`, f.scope.TenantID, other.UserID); err != nil {
		t.Fatal(err)
	}
	other.TenantID = f.scope.TenantID
	if _, err := f.s.GetWorkflowRun(ctx, other, d.ID, r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another member read runtime content: %v", err)
	}
	if _, err := f.s.RequestWorkflowRunCancel(ctx, other, d.ID, r.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another member cancelled run: %v", err)
	}
	cancelled, err := f.s.RequestWorkflowRunCancel(ctx, f.scope, d.ID, r.ID)
	if err != nil || cancelled.CancelRequestedAt == nil || cancelled.Revision != r.Revision || cancelled.Checkpoint.State.Status != "pending" {
		t.Fatalf("cancel intent changed execution state: %+v %v", cancelled, err)
	}
	if _, err := f.s.BeginWorkflowRun(ctx, f.scope, d.ID, r.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("cancelled admission dispatched: %v", err)
	}
	second, err := f.s.CreateWorkflowRun(ctx, f.scope, d.ID, "second", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2`, f.scope.TenantID, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateWorkflowRun(ctx, f.scope, d.ID, "run", in); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("revoked owner replayed admission: %v", err)
	}
	// Startup recovery must also settle revoked owners, without dispatching.
	for range 2 {
		if err := f.s.RecoverWorkflowRuns(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.admin.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role) VALUES($1,$2,'owner')`, f.scope.TenantID, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.RequestWorkflowRunCancel(ctx, f.scope, d.ID, r.ID)
	if err != nil || again.Checkpoint.State.Status != "cancelled" || !again.CancelRequestedAt.Equal(*cancelled.CancelRequestedAt) {
		t.Fatalf("cancel replay: %+v %v", again, err)
	}
	interrupted, err := f.s.GetWorkflowRun(ctx, f.scope, d.ID, second.ID)
	if err != nil || interrupted.Checkpoint.State.Status != "failed" || interrupted.Checkpoint.State.Error != "interrupted" || interrupted.FinishedAt == nil {
		t.Fatalf("pending run not interrupted: %+v %v", interrupted, err)
	}
}
