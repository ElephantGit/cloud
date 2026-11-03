package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestChatWorkspaceFollowsTheConversationSession(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	c, err := f.s.CreateChatConversation(ctx, store.NewChatConversation{
		TenantID: f.scope.TenantID, UserID: f.scope.UserID, CreateKey: "k", ProjectID: "demo", ExecutorID: "fixture", Title: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChatWorkspace(ctx, f.scope.TenantID, f.scope.UserID, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("before the first turn: %v, want ErrNotFound", err)
	}
	turn := executionTurn("s1", "t1")
	if err := f.s.CreateExecutionSession(ctx, f.scope, execution.Session{ID: "s1", WorkspaceID: "w1", Model: execution.Model{ID: "fixture"}}, turn); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordChatTurn(ctx, c.ID, "s1", "k", "t1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChatWorkspace(ctx, f.scope.TenantID, f.scope.UserID, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("before the checkout is ready: %v, want ErrNotFound", err)
	}
	for _, at := range []store.WorkspaceLocation{{Path: "/old", BaseCommit: "a"}, {Path: "/data/w1", BaseCommit: "b"}} {
		if err := f.s.RecordWorkspace(ctx, "w1", at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.s.ChatWorkspace(ctx, f.scope.TenantID, f.scope.UserID, c.ID)
	if err != nil || got != (store.WorkspaceLocation{Path: "/data/w1", BaseCommit: "b"}) {
		t.Fatalf("workspace = %+v, %v; want the latest record", got, err)
	}
	other := executionMember(t, f.s, "other")
	if _, err := f.s.ChatWorkspace(ctx, f.scope.TenantID, other.UserID, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another person's conversation: %v, want ErrNotFound", err)
	}
}

func TestWorkflowRunWorkspaceUsesItsAgentNodes(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	run := pendingRun(t, f.s, f.scope.UserID, f.scope.TenantID, "flow")
	if _, err := f.s.WorkflowRunWorkspace(ctx, f.scope.TenantID, run.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("run without Agent nodes: %v, want ErrNotFound", err)
	}
	if err := f.s.CreateExecutionSession(ctx, f.scope, execution.Session{ID: "node-session", WorkspaceID: "run-w", Model: execution.Model{ID: "fixture"}}, executionTurn("node-session", "t1")); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordWorkspace(ctx, "run-w", store.WorkspaceLocation{Path: "/data/run", BaseCommit: "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `
		INSERT INTO v2_workflow_node_runs (tenant_id, run_id, scope_id, node_id, node_type, session_id, status)
		VALUES ($1, $2, 'root', 'agent', 'agent', 'node-session', 'succeeded')`, f.scope.TenantID, run.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.WorkflowRunWorkspace(ctx, f.scope.TenantID, run.ID)
	if err != nil || got != (store.WorkspaceLocation{Path: "/data/run", BaseCommit: "c"}) {
		t.Fatalf("run workspace = %+v, %v", got, err)
	}
}
