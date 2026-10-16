package core

// DB tests for the Cloud-side workflow scheduler's advance pass (workflow_scheduler.go): a pending
// run is claimed and settled from the frozen snapshot's graph, agent nodes fail the run honestly
// until the execution wiring lands, and the pass is idempotent against already-advanced runs. Each
// test is a real isolated schema (dispatcherDB), and the graph reaches the pass the way production
// delivers it — jsonb round-tripped, so every document is plain map[string]any.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// seedWorkflowRunScene loads one tenant with a workflow, a frozen snapshot and a pending run whose
// graph is the given document, returning the run id the assertions read back. The scene is minimal
// on purpose: the advance pass reads workflow_runs joined to workflow_snapshots and nothing else.
func seedWorkflowRunScene(t *testing.T, store *Store, graph Object) string {
	t.Helper()
	wid, sid, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	tid, uid := uuid.NewString(), uuid.NewString()
	g, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	tx, err := store.Pool.Begin()
	if err != nil {
		t.Fatalf("seed begin: %v", err)
	}
	exec := func(name, q string, args ...any) {
		if _, err := tx.Exec(q, args...); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	exec("user", `INSERT INTO users(id,display_name,status) VALUES($1,'wf','active')`, uid)
	exec("tenant", `INSERT INTO tenants(id,name,status) VALUES($1,'wf','active')`, tid)
	exec("membership", `INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES($1,$2,'admin','active')`, tid, uid)
	exec("workflow", `INSERT INTO workflows(id,tenant_id,name,graph) VALUES($1,$2,'wf',$3::jsonb)`, wid, tid, g)
	exec("snapshot", `INSERT INTO workflow_snapshots(id,tenant_id,workflow_id,version,name,graph) VALUES($1,$2,$3,1,'v1',$4::jsonb)`, sid, tid, wid, g)
	exec("run", `INSERT INTO workflow_runs(id,tenant_id,workflow_id,snapshot_id,name,status,input) VALUES($1,$2,$3,$4,'disp','pending','{}'::jsonb)`, runID, tid, wid, sid)
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	return runID
}

// readWorkflowRun reads the run row back at the storage boundary: the settled status and error, the
// committed node states and the version the CAS bumped.
func readWorkflowRun(t *testing.T, store *Store, runID string) (status, runErr string, states Object, version int64) {
	t.Helper()
	var raw []byte
	if err := store.Pool.QueryRow(`SELECT status, error, node_states, version FROM workflow_runs WHERE id=$1`, runID).
		Scan(&status, &runErr, &raw, &version); err != nil {
		t.Fatalf("read run: %v", err)
	}
	if err := json.Unmarshal(raw, &states); err != nil {
		t.Fatalf("decode node_states: %v", err)
	}
	return status, runErr, states, version
}

// One pass claims the pending run, executes start then output from the frozen graph, and settles the
// run succeeded with every node's committed state on the row.
func TestWorkflowAdvanceSucceedsLinearRun(t *testing.T) {
	store := dispatcherDB(t)
	runID := seedWorkflowRunScene(t, store, wfGraph(
		[]Object{wfNode("s", "start", ""), wfNode("o", "output", "done")},
		[][3]string{{"s", "o", ""}},
	))

	if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
		t.Fatalf("advance: %v", err)
	}
	status, runErr, states, version := readWorkflowRun(t, store, runID)
	if status != "succeeded" || runErr != "" {
		t.Fatalf("run settled %q error %q; want succeeded", status, runErr)
	}
	if version != 3 { // claim +1, settle +1
		t.Fatalf("version = %d; want 3", version)
	}
	for id, want := range map[string]string{"s": "succeeded", "o": "succeeded"} {
		state, ok := asWorkflowObject(states[id])
		if !ok {
			t.Fatalf("node %s has no committed state", id)
		}
		if state.S("status") != want {
			t.Fatalf("node %s status = %q; want %q", id, state.S("status"), want)
		}
	}
	if start := states["s"].(map[string]any)["output"].(map[string]any); start["input"] == nil {
		t.Fatal("start node output does not report the run input")
	}
}

// A ready agent node settles the run failed with an explicit reason: the node never ran, so it is
// never recorded as succeeded (invariant 5) and nothing downstream of it committed a state.
func TestWorkflowAdvanceFailsRunOnUnwiredAgentNode(t *testing.T) {
	store := dispatcherDB(t)
	runID := seedWorkflowRunScene(t, store, wfGraph(
		[]Object{wfNode("s", "start", ""), wfNode("a", "agent", "worker"), wfNode("o", "output", "")},
		[][3]string{{"s", "a", ""}, {"a", "o", ""}},
	))

	if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
		t.Fatalf("advance: %v", err)
	}
	status, runErr, states, _ := readWorkflowRun(t, store, runID)
	if status != "failed" {
		t.Fatalf("run settled %q; want failed", status)
	}
	if !strings.Contains(runErr, "a") || !strings.Contains(runErr, "not wired") {
		t.Fatalf("run error %q does not name the unwired agent node", runErr)
	}
	agent, ok := asWorkflowObject(states["a"])
	if !ok {
		t.Fatal("agent node has no committed state")
	}
	if agent.S("status") != "failed" {
		t.Fatalf("agent node status = %q; want failed", agent.S("status"))
	}
	if agent.O("output").S("reason") != "agent_execution_not_wired" {
		t.Fatalf("agent node reason = %q; want agent_execution_not_wired", agent.O("output").S("reason"))
	}
	if _, ran := states["o"]; ran {
		t.Fatal("output node downstream of the unwired agent committed a state")
	}
	if _, ran := states["s"]; !ran {
		t.Fatal("start node before the agent did not commit its state")
	}
}

// The pass is idempotent: a second tick over a settled run rescans nothing and leaves the committed
// row untouched — no second claim, no second settle, no version drift.
func TestWorkflowAdvanceIsIdempotent(t *testing.T) {
	store := dispatcherDB(t)
	runID := seedWorkflowRunScene(t, store, wfGraph(
		[]Object{wfNode("s", "start", ""), wfNode("o", "output", "")},
		[][3]string{{"s", "o", ""}},
	))

	for i := 0; i < 3; i++ {
		if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
	}
	status, runErr, _, version := readWorkflowRun(t, store, runID)
	if status != "succeeded" || runErr != "" {
		t.Fatalf("run settled %q error %q; want succeeded", status, runErr)
	}
	if version != 3 {
		t.Fatalf("version = %d after three passes; want 3 (claim + settle only)", version)
	}
}
