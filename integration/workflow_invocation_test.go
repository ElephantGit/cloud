package integration

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// TestWorkflowConfirmCreatesLinkedWorkflowRun checks the persisted execution identity across the
// public confirm boundary, rather than accepting a legacy IssueRun simulation as execution evidence.
func TestWorkflowConfirmCreatesLinkedWorkflowRun(t *testing.T) {
	f := setup(t)
	wid := createWorkflow(t, f, "Linked workflow", "inv-link-wf", nil)
	snapshot := publishWorkflow(t, f, wid, "inv-link-pub")
	iid := f.call("POST", f.path("/issues"), core.Object{"title": "Linked execution", "projectRef": runProject(t, f, "inv-link-project")}, "inv-link-issue", 200).O("resource").S("id")
	interaction := workflowComment(t, f, wid, iid, "inv-link-comment", "run")
	run := f.call("POST", confirmPath(f, iid, interaction.S("id")), core.Object{"values": core.Object{
		"repository": "ora-space/cloud", "branch": "main", "snapshotId": snapshot.S("id"),
	}}, "inv-link-confirm", 200).O("resource")
	if run.S("workflowInvocationRef") == "" {
		t.Fatalf("confirmed workflow IssueRun has no linked WorkflowRun: %v", run)
	}
	workflowRun := f.call("GET", f.path("/workflows/"+wid+"/runs/"+run.S("workflowInvocationRef")), nil, "", 200)
	if workflowRun.S("snapshotId") != snapshot.S("id") || run.O("input").S("workflowSnapshotId") != snapshot.S("id") {
		t.Fatalf("linked runs disagree about the selected snapshot: issue=%v workflow=%v", run, workflowRun)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_runs WHERE workflow_id=$1", wid); n != 1 {
		t.Fatalf("confirm created %d WorkflowRuns, want exactly one", n)
	}
}

// assertUnclaimedWorkflowInvocation checks the entire refused-start transaction, including activity
// and the interaction claim; a status assertion alone would miss a partially committed start.
func assertUnclaimedWorkflowInvocation(t *testing.T, f *fixture, iid, interactionID string, activities int) {
	t.Helper()
	if n := f.scalar("SELECT count(*) FROM issue_runs WHERE issue_id=$1", iid); n != 0 {
		t.Fatalf("refused confirm left %d IssueRuns", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_runs"); n != 0 {
		t.Fatalf("refused confirm left %d WorkflowRuns", n)
	}
	if n := f.scalar("SELECT count(*) FROM issue_activities WHERE issue_id=$1", iid); n != activities {
		t.Fatalf("refused confirm changed activity count from %d to %d", activities, n)
	}
	if n := f.scalar("SELECT count(*) FROM issue_interactions WHERE id=$1 AND run_id IS NULL AND input='{}'::jsonb", interactionID); n != 1 {
		t.Fatal("refused confirm claimed the interaction or changed its input")
	}
}

func invocationVariable(name string) core.Object {
	return core.Object{"name": name, "displayName": name, "fieldType": "text-input", "valueType": "string", "required": true}
}

func TestWorkflowConfirmAdmissionRollsBackBothRuns(t *testing.T) {
	for _, scenario := range []string{"unpublished", "wrong-workflow-snapshot", "foreign-snapshot", "missing-input", "invalid-input-type"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			wid := createWorkflow(t, f, "Admission workflow", "inv-gate-wf", []any{invocationVariable("topic")})
			values := core.Object{"repository": "ora-space/cloud", "branch": "main", "topic": "auth"}
			status, code := 400, ""
			if scenario != "unpublished" {
				values["snapshotId"] = publishWorkflow(t, f, wid, "inv-gate-pub").S("id")
			} else {
				code = "workflow_no_published_snapshot"
			}
			switch scenario {
			case "wrong-workflow-snapshot":
				other := createWorkflow(t, f, "Other workflow", "inv-gate-other", nil)
				values["snapshotId"] = publishWorkflow(t, f, other, "inv-gate-other-pub").S("id")
				status = 404
			case "foreign-snapshot":
				other, err := f.store.Bootstrap(context.Background(), "Other tenant", "corp", "inv-other", "Other")
				must(t, err)
				tenant, subject := f.tid, f.user.Subject
				f.tid, f.user.Subject = other.S("tenantId"), "inv-other"
				foreign := createWorkflow(t, f, "Foreign workflow", "inv-gate-foreign", nil)
				values["snapshotId"] = publishWorkflow(t, f, foreign, "inv-gate-foreign-pub").S("id")
				f.tid, f.user.Subject = tenant, subject
				status = 404
			case "missing-input":
				delete(values, "topic")
				code = "required_field_missing"
			case "invalid-input-type":
				values["topic"] = 42
				code = "invalid_field_value"
			}
			iid := f.call("POST", f.path("/issues"), core.Object{"title": "Rejected start", "projectRef": runProject(t, f, "inv-gate-project")}, "inv-gate-issue", 200).O("resource").S("id")
			it := workflowComment(t, f, wid, iid, "inv-gate-comment", "run")
			activities := f.scalar("SELECT count(*) FROM issue_activities WHERE issue_id=$1", iid)
			refusal := f.call("POST", confirmPath(f, iid, it.S("id")), core.Object{"values": values}, "inv-gate-confirm", status)
			if code != "" && refusal.S("code") != code {
				t.Fatalf("refused start code = %q, want %q: %v", refusal.S("code"), code, refusal)
			}
			assertUnclaimedWorkflowInvocation(t, f, iid, it.S("id"), activities)
		})
	}
}

func TestWorkflowConfirmFreezesHistoricalSchemaAndInput(t *testing.T) {
	f := setup(t)
	graph := func(variables []any) core.Object {
		out := startInputGraph(variables)
		out["globalVariables"] = []any{core.Object{"name": "review.mode", "valueType": "string", "value": "default"}}
		return out
	}
	wid := f.call("POST", f.path("/workflows"), core.Object{
		"name": "Historical workflow", "graph": graph([]any{invocationVariable("topic")}),
	}, "inv-old-wf", 200).O("resource").S("id")
	older := publishWorkflow(t, f, wid, "inv-old-pub-1")
	edit := func(variables []any, zoom float64) {
		t.Helper()
		live := f.call("GET", f.path("/workflows/"+wid), nil, "", 200)
		updated := graph(variables)
		updated["viewport"] = core.Object{"x": 0, "y": 0, "zoom": zoom}
		f.call("PUT", f.path("/workflows/"+wid), core.Object{
			"graph": updated, "version": live.N("version"),
		}, "", 200)
	}
	// A newer graph with the same fields keeps the live-form compatibility gate satisfied.
	edit([]any{invocationVariable("topic")}, 2)
	newer := publishWorkflow(t, f, wid, "inv-old-pub-2")
	if reflect.DeepEqual(older.O("graph"), newer.O("graph")) {
		t.Fatal("historical snapshot test needs distinct published definitions")
	}
	iid := f.call("POST", f.path("/issues"), core.Object{"title": "Historical start", "projectRef": runProject(t, f, "inv-old-project")}, "inv-old-issue", 200).O("resource").S("id")
	it := workflowComment(t, f, wid, iid, "inv-old-comment", "run old version")
	created := f.call("POST", confirmPath(f, iid, it.S("id")), core.Object{"values": core.Object{
		"repository": "ora-space/cloud", "branch": "main", "snapshotId": older.S("id"), "topic": "auth", "review.mode": "strict",
	}}, "inv-old-confirm", 200).O("resource")
	ref := created.S("workflowInvocationRef")
	if ref == "" {
		t.Fatal("historical confirm did not link a WorkflowRun")
	}
	path := f.path("/workflows/" + wid + "/runs/" + ref)
	before := f.call("GET", path, nil, "", 200)
	if before.S("snapshotId") != older.S("id") || created.O("input").S("workflowSnapshotId") != older.S("id") {
		t.Fatalf("historical snapshot was not selected: issue=%v workflow=%v", created, before)
	}
	if !reflect.DeepEqual(before.O("input"), core.Object{"topic": "auth", "review.mode": "strict"}) {
		t.Fatalf("execution input must contain selected-schema variables only: %v", before.O("input"))
	}
	if !reflect.DeepEqual(before.O("definitionSnapshot"), older.O("graph")) {
		t.Fatalf("definition does not match selected frozen document: %v", before)
	}
	edit([]any{invocationVariable("replacement")}, 3)
	publishWorkflow(t, f, wid, "inv-old-pub-3")
	after := f.call("GET", path, nil, "", 200)
	for _, field := range []string{"snapshotId", "input", "definitionSnapshot"} {
		if !reflect.DeepEqual(after[field], before[field]) {
			t.Fatalf("editing and republishing changed frozen %s: before=%v after=%v", field, before[field], after[field])
		}
	}
}

func TestWorkflowConfirmInvocationIdempotency(t *testing.T) {
	f := setup(t)
	wid := createWorkflow(t, f, "Replay workflow", "inv-idem-wf", nil)
	publishWorkflow(t, f, wid, "inv-idem-pub")
	iid := f.call("POST", f.path("/issues"), core.Object{"title": "Replay start", "projectRef": runProject(t, f, "inv-idem-project")}, "inv-idem-issue", 200).O("resource").S("id")
	it := workflowComment(t, f, wid, iid, "inv-idem-comment", "run")
	body := core.Object{"values": core.Object{"repository": "ora-space/cloud", "branch": "main"}}
	path := confirmPath(f, iid, it.S("id"))
	first := f.call("POST", path, body, "inv-idem-confirm", 200)
	replay := f.call("POST", path, body, "inv-idem-confirm", 200)
	if !reflect.DeepEqual(first, replay) {
		t.Fatalf("same-key confirm did not replay its original response: first=%v replay=%v", first, replay)
	}
	if first.O("resource").S("workflowInvocationRef") == "" {
		t.Fatal("replayed response has no WorkflowRun identity")
	}
	refusal := f.call("POST", path, body, "inv-idem-second", 409)
	if refusal.S("code") != "interaction_already_confirmed" {
		t.Fatalf("second confirm fault = %v", refusal)
	}
	if n := f.scalar("SELECT count(*) FROM issue_runs WHERE issue_id=$1", iid); n != 1 {
		t.Fatalf("replay created %d IssueRuns", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_runs WHERE workflow_id=$1", wid); n != 1 {
		t.Fatalf("replay created %d WorkflowRuns", n)
	}
}

func TestWorkflowConfirmConcurrentInvocationCreatesExactlyOnePair(t *testing.T) {
	f := setup(t)
	wid := createWorkflow(t, f, "Concurrent workflow", "inv-race-wf", nil)
	publishWorkflow(t, f, wid, "inv-race-pub")
	iid := f.call("POST", f.path("/issues"), core.Object{"title": "Concurrent start", "projectRef": runProject(t, f, "inv-race-project")}, "inv-race-issue", 200).O("resource").S("id")
	it := workflowComment(t, f, wid, iid, "inv-race-comment", "run")
	path := confirmPath(f, iid, it.S("id"))
	start := make(chan struct{})
	var wg sync.WaitGroup
	bodies, statuses, errs := make([]core.Object, 2), make([]int, 2), make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			bodies[i], statuses[i], errs[i] = callRaw(f, "POST", path, core.Object{"values": core.Object{
				"repository": "ora-space/cloud", "branch": "main",
			}}, "inv-race-"+uuid.NewString())
		}(i)
	}
	close(start)
	wg.Wait()
	var accepted core.Object
	for i := range 2 {
		must(t, errs[i])
		if statuses[i] == 200 {
			accepted = bodies[i].O("resource")
		} else if bodies[i].S("code") != "interaction_already_confirmed" {
			t.Fatalf("concurrent refusal = %v", bodies[i])
		}
	}
	sort.Ints(statuses)
	if !reflect.DeepEqual(statuses, []int{200, 409}) {
		t.Fatalf("concurrent statuses = %v, want [200 409]", statuses)
	}
	if n := f.scalar("SELECT count(*) FROM issue_runs WHERE issue_id=$1", iid); n != 1 {
		t.Fatalf("concurrent confirm created %d IssueRuns", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_runs WHERE workflow_id=$1", wid); n != 1 {
		t.Fatalf("concurrent confirm created %d WorkflowRuns", n)
	}
	if n := f.scalar("SELECT count(*) FROM issue_runs ir JOIN workflow_runs wr ON wr.id=ir.workflow_invocation_ref AND wr.tenant_id=ir.tenant_id JOIN issue_interactions ix ON ix.run_id=ir.id WHERE ix.id=$1 AND ir.id=$2 AND wr.id=$3", it.S("id"), accepted.S("id"), accepted.S("workflowInvocationRef")); n != 1 {
		t.Fatal("winning confirm did not atomically persist the interaction and same-tenant run pair")
	}
}

func TestManualWorkflowInvocationCreatesLinkedRun(t *testing.T) {
	f := setup(t)
	wid := createWorkflow(t, f, "Manual workflow", "inv-manual-wf", []any{invocationVariable("topic")})
	snapshot := publishWorkflow(t, f, wid, "inv-manual-pub")
	iid := f.call("POST", f.path("/issues"), core.Object{"title": "Manual start", "projectRef": runProject(t, f, "inv-manual-project")}, "inv-manual-issue", 200).O("resource").S("id")
	status, body := manualWorkflowRun(t, f, iid, wid, "inv-manual-start", core.Object{"topic": "auth"})
	if status != 200 {
		t.Fatalf("manual start status = %d: %v", status, body)
	}
	run := body.O("resource")
	if run.S("workflowInvocationRef") == "" {
		t.Fatal("manual workflow IssueRun has no WorkflowRun identity")
	}
	linked := f.call("GET", f.path("/workflows/"+wid+"/runs/"+run.S("workflowInvocationRef")), nil, "", 200)
	if linked.S("snapshotId") != snapshot.S("id") || run.O("input").S("workflowSnapshotId") != snapshot.S("id") {
		t.Fatalf("manual run pair froze different snapshots: issue=%v workflow=%v", run, linked)
	}
	if linked.O("input").S("topic") != "auth" {
		t.Fatalf("manual WorkflowRun lost its execution input: %v", linked)
	}
	if n := f.scalar("SELECT count(*) FROM issue_runs ir JOIN workflow_runs wr ON wr.id=ir.workflow_invocation_ref AND wr.tenant_id=ir.tenant_id WHERE ir.issue_id=$1", iid); n != 1 {
		t.Fatalf("manual start created %d valid linked pairs, want 1", n)
	}
}

// confirmInvocationScene crosses the public comment/form boundary. Unlike the fixture's direct
// workflow-run POST, Confirm leaves execution pending for the real scheduler used by these tests.
func confirmInvocationScene(t *testing.T, f *fixture, graph core.Object, key string) (wid, iid string, confirmed core.Object) {
	t.Helper()
	wid = f.call("POST", f.path("/workflows"), core.Object{
		"name": "Scheduled invocation", "graph": graph,
	}, key+"-workflow", 200).O("resource").S("id")
	publishWorkflow(t, f, wid, key+"-publish")
	iid = launchIssue(t, f, runProject(t, f, key+"-project"), "Scheduled invocation", key+"-issue").S("id")
	interaction := workflowComment(t, f, wid, iid, key+"-comment", "run this workflow")
	confirmed = f.call("POST", confirmPath(f, iid, interaction.S("id")), core.Object{"values": core.Object{
		"repository": "ora-space/cloud", "branch": "main",
	}}, key+"-confirm", 200).O("resource")
	if confirmed.S("status") != "queued" || confirmed.S("workflowInvocationRef") == "" {
		t.Fatalf("Confirm must enqueue a linked invocation before scheduling: %v", confirmed)
	}
	return wid, iid, confirmed
}

// assertInvocationProjection reads both public run resources and the issue timeline, so a scheduler
// state change without its IssueRun projection or lifecycle activity cannot satisfy the test.
func assertInvocationProjection(t *testing.T, f *fixture, wid, iid string, confirmed core.Object, workflowStatus, issueStatus string, actions []string) (core.Object, core.Object) {
	t.Helper()
	workflow := f.call("GET", f.path("/workflows/"+wid+"/runs/"+confirmed.S("workflowInvocationRef")), nil, "", 200)
	var issueRun core.Object
	for _, candidate := range issueItems(f.call("GET", f.path("/issues/"+iid+"/runs"), nil, "", 200)) {
		if candidate.S("id") == confirmed.S("id") {
			issueRun = candidate
			break
		}
	}
	if workflow.S("status") != workflowStatus || issueRun.S("status") != issueStatus {
		t.Fatalf("scheduler projection disagrees: workflow=%v issue=%v; want %s/%s", workflow, issueRun, workflowStatus, issueStatus)
	}
	if issueRun.S("workflowInvocationRef") != workflow.S("id") || issueRun.S("executorId") != wid || issueRun.S("executorType") != "workflow" {
		t.Fatalf("projected IssueRun lost its execution identity: %v", issueRun)
	}
	if workflow.S("startedAt") == "" || issueRun.S("startedAt") != workflow.S("startedAt") || issueRun.S("completedAt") != workflow.S("finishedAt") {
		t.Fatalf("projected lifecycle timestamps disagree: workflow=%v issue=%v", workflow, issueRun)
	}
	if issueRun.S("failureReason") != workflow.S("error") || issueRun.S("error") != workflow.S("error") {
		t.Fatalf("projected failure reason disagrees: workflow=%v issue=%v", workflow, issueRun)
	}
	var got []string
	for _, item := range issueItems(f.call("GET", f.path("/issues/"+iid+"/timeline"), nil, "", 200)) {
		details := item.O("details")
		if item.S("kind") != "activity" || details.S("runId") != confirmed.S("id") {
			continue
		}
		got = append(got, item.S("action"))
		if details.S("executorType") != "workflow" || details.S("executorId") != wid {
			t.Fatalf("activity lost its executor identity: %v", item)
		}
		if item.S("action") != "run.enqueued" && details.S("workflowInvocationRef") != workflow.S("id") {
			t.Fatalf("scheduler activity lost its workflow invocation identity: %v", item)
		}
		if item.S("action") == "run.failed" && details.S("failureReason") != workflow.S("error") {
			t.Fatalf("failure activity lost the authoritative reason: %v", item)
		}
	}
	if !reflect.DeepEqual(got, actions) {
		t.Fatalf("invocation lifecycle activities = %v, want %v", got, actions)
	}
	return workflow, issueRun
}

func TestWorkflowConfirmSchedulerProjectsRunningOnce(t *testing.T) {
	f := setup(t)
	seedPersonalModel(t, f)
	wid, iid, confirmed := confirmInvocationScene(t, f, runGraph(), "inv-scheduler-running")
	// The Project is still provisioning: session startup waits without Node or cluster execution.
	// A second advance exercises the running-state retry, which must not emit another run.started.
	for range 2 {
		must(t, f.store.AdvanceWorkflowRunsOnce(t.Context()))
	}
	workflow, _ := assertInvocationProjection(t, f, wid, iid, confirmed, "running", "running", []string{"run.enqueued", "run.started"})
	if len(workflow.O("nodeStates")) != 0 || workflow.S("finishedAt") != "" {
		t.Fatalf("waiting invocation fabricated committed nodes or terminal evidence: %v", workflow)
	}
}

func TestWorkflowConfirmSchedulerCompletesAndAllowsNextInvocation(t *testing.T) {
	f := setup(t)
	graph := startInputGraph(nil)
	graph["nodes"] = append(graph["nodes"].([]any), core.Object{
		"id": "output-1", "type": "workflow", "position": core.Object{"x": 300, "y": 0},
		"data": core.Object{"kind": "output", "title": "Done"},
	})
	graph["edges"] = []any{core.Object{"id": "output-edge", "source": "start-1", "target": "output-1"}}
	wid, iid, confirmed := confirmInvocationScene(t, f, graph, "inv-scheduler-completed")
	for range 2 {
		must(t, f.store.AdvanceWorkflowRunsOnce(t.Context()))
	}
	workflow, _ := assertInvocationProjection(t, f, wid, iid, confirmed, "succeeded", "completed", []string{"run.enqueued", "run.started", "run.completed"})
	for _, node := range []string{"start-1", "output-1"} {
		if workflow.O("nodeStates").O(node).S("status") != "succeeded" {
			t.Fatalf("completed invocation did not execute node %s: %v", node, workflow)
		}
	}
	if workflow.S("finishedAt") == "" {
		t.Fatal("completed invocation has no terminal timestamp")
	}

	// workflowComment returns the first form, so find the fresh unclaimed form explicitly on this
	// same Issue. Terminal projection must release the pending-executor dedup for this Confirm.
	f.call("POST", f.path("/issues/"+iid+"/comments"), core.Object{
		"body": "run this workflow again", "targets": []any{core.Object{"type": "workflow", "id": wid}},
	}, "inv-scheduler-completed-next-comment", 200)
	var nextInteraction core.Object
	for _, interaction := range issueItems(f.call("GET", f.path("/issues/"+iid+"/interactions"), nil, "", 200)) {
		if interaction.S("mode") == "form" && interaction.S("runId") == "" {
			nextInteraction = interaction
			break
		}
	}
	if nextInteraction == nil {
		t.Fatal("second workflow comment did not create an unclaimed form")
	}
	next := f.call("POST", confirmPath(f, iid, nextInteraction.S("id")), core.Object{"values": core.Object{
		"repository": "ora-space/cloud", "branch": "main",
	}}, "inv-scheduler-completed-next-confirm", 200).O("resource")
	if next.S("status") != "queued" || next.S("id") == confirmed.S("id") || next.S("workflowInvocationRef") == "" || next.S("workflowInvocationRef") == confirmed.S("workflowInvocationRef") {
		t.Fatalf("second Confirm did not create a new linked queued invocation: first=%v next=%v", confirmed, next)
	}
}

func TestWorkflowConfirmSchedulerProjectsConditionFailure(t *testing.T) {
	f := setup(t)
	graph := startInputGraph(nil)
	// input is a declared Start root and is always a string. Publishing admits the selector, but
	// traversing a nested object path on that string deterministically fails during real execution.
	graph["nodes"] = append(graph["nodes"].([]any), core.Object{
		"id": "condition-1", "type": "workflow", "position": core.Object{"x": 300, "y": 0},
		"data": core.Object{"kind": "condition", "title": "Invalid nested input", "cases": []any{core.Object{
			"id": "matched", "logic": "and", "conditions": []any{core.Object{
				"variableSelector": []any{"start-1", "input", "missing"}, "operator": "equals", "value": "value",
			}},
		}}},
	})
	graph["edges"] = []any{core.Object{"id": "condition-edge", "source": "start-1", "target": "condition-1"}}
	wid, iid, confirmed := confirmInvocationScene(t, f, graph, "inv-scheduler-failed")
	for range 2 {
		must(t, f.store.AdvanceWorkflowRunsOnce(t.Context()))
	}
	workflow, _ := assertInvocationProjection(t, f, wid, iid, confirmed, "failed", "failed", []string{"run.enqueued", "run.started", "run.message", "run.failed"})
	failed := workflow.O("nodeStates").O("condition-1")
	if failed.S("status") != "failed" || !strings.Contains(workflow.S("error"), "start-1.input.missing") || failed.S("error") != workflow.S("error") || workflow.S("finishedAt") == "" {
		t.Fatalf("condition failure did not preserve its node reason and terminal evidence: %v", workflow)
	}
}
