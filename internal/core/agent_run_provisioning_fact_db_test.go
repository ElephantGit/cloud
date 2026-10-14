package core

import (
	"context"
	"strings"
	"testing"
)

// The D5 fact-takeover entry: after the abandonment the operation's own actions are closed, so a
// Controller with real late evidence submits through `provisioning_fact`. Everything here runs on
// the shipped seams: the dispatcher claim, the claim and defer control actions, the abandonment
// pass, and the fact action itself.

// abandonedCreation is one D5-settled creation: provisioning run, blocked operation, expired
// deadline, abandonment committed. The lease stays seeded for the fact submissions.
type abandonedCreation struct {
	provisioningSeed
	claims *Claims
	effect string
}

// seedAbandonedCreation parks a creation in blocked (freezing the deadline), optionally plans one
// sandbox_ensure effect whose result stays unknown, and abandons the run when the window expires.
func seedAbandonedCreation(t *testing.T, store *Store, withEffect bool) abandonedCreation {
	t.Helper()
	p := seedProvisioningCreation(t, store)
	seedControllerLease(t, store)
	claims := controllerClaims()
	out, err := store.Control(context.Background(), &ControlRequest{Action: "claim", Body: Object{"epoch": 1}, Service: claims})
	if err != nil {
		t.Fatalf("claim the creation: %v", err)
	}
	op := out.O("operation")
	if op.S("id") != p.operation {
		t.Fatalf("claim picked %s, not this scene's creation %s", op.S("id"), p.operation)
	}
	var effect string
	if withEffect {
		// The sandbox step's own plan action: the effect id doubles as the preallocated sandbox id.
		planned, err := store.Control(context.Background(), &ControlRequest{
			Action: "plan", OperationID: p.operation,
			Body:    Object{"epoch": 1, "version": op.N("version"), "kind": "sandbox_ensure", "workspaceId": p.workspace},
			Service: claims,
		})
		if err != nil {
			t.Fatalf("plan the sandbox ensure: %v", err)
		}
		effect = planned.O("effect").S("id")
		op = planned.O("operation")
	}
	if _, err := store.Control(context.Background(), &ControlRequest{
		Action: "defer", OperationID: p.operation,
		Body:    Object{"epoch": 1, "version": op.N("version"), "state": "blocked", "errorCode": "node_unavailable", "retrySeconds": 60},
		Service: claims,
	}); err != nil {
		t.Fatalf("defer to blocked: %v", err)
	}
	expireCreationDeadline(t, store, p.run)
	creationPass(t, store)
	phase, status, reason, _, _, _ := runFields(t, store, p.run)
	if phase.String != "done" || status != "failed" || reason != "workspace_unavailable" {
		t.Fatalf("seed did not abandon the creation: %s/%s/%s", phase.String, status, reason)
	}
	return abandonedCreation{provisioningSeed: p, claims: claims, effect: effect}
}

// submitFact drives the provisioning_fact action; a non-nil error is returned as-is for conflict
// assertions.
func submitFact(t *testing.T, store *Store, scene abandonedCreation, submission string, body Object) (Object, error) {
	t.Helper()
	return store.Control(context.Background(), &ControlRequest{
		Action: "provisioning_fact", OperationID: scene.operation, SubmissionID: submission,
		Body: body, Service: scene.claims,
	})
}

// A late effect fact is stored on the original effect, receipted once on the ledger, replayed
// idempotently, and refused when it contradicts itself. The business terminal never moves.
func TestProvisioningFactReceiptsLateEffectWithoutResurrecting(t *testing.T) {
	store := commandStore(t)
	scene := seedAbandonedCreation(t, store, true)
	factBody := Object{
		"epoch": 1, "kind": "effect", "effectId": scene.effect, "state": "succeeded", "externalId": "sub-123",
		"result": Object{"sandboxInstanceId": scene.effect, "nodeId": "node-late"},
	}

	out, err := submitFact(t, store, scene, "fact-1", factBody)
	if err != nil {
		t.Fatalf("late effect fact refused: %v", err)
	}
	if out.O("effect").S("state") != "succeeded" || out.O("effect").S("externalId") != "sub-123" {
		t.Fatalf("effect fact not stored: %v", out.O("effect"))
	}
	// The same submission replays its recorded response; a different submission of the same fact is
	// a no-op, not a second receipt.
	if _, err := submitFact(t, store, scene, "fact-1", factBody); err != nil {
		t.Fatalf("submission replay failed: %v", err)
	}
	if _, err := submitFact(t, store, scene, "fact-2", factBody); err != nil {
		t.Fatalf("identical fact under a new submission must replay: %v", err)
	}
	var facts int
	var state, external string
	if err := store.Pool.QueryRow(`SELECT (SELECT count(*) FROM jsonb_object_keys(responsibility->'lateFacts')),
	 responsibility->'lateFacts'->('effect:'||$2)->>'state', responsibility->'lateFacts'->('effect:'||$2)->>'externalId'
	 FROM agent_run_residuals WHERE run_id=$1`, scene.run, scene.effect).Scan(&facts, &state, &external); err != nil {
		t.Fatalf("ledger receipt read failed: %v", err)
	}
	if facts != 1 || state != "succeeded" || external != "sub-123" {
		t.Fatalf("ledger receipt wrong: count=%d state=%s external=%s", facts, state, external)
	}
	// A different fact for the same effect identity is refused — by the live path's own evidence
	// rules here (a succeeded effect cannot become failed), which is exactly the guarantee the
	// entry reuses — and the ledger keeps the original receipt.
	conflicting := Object{"epoch": 1, "kind": "effect", "effectId": scene.effect, "state": "failed", "externalId": "sub-123", "result": Object{"error": "substrate said no"}}
	if _, err := submitFact(t, store, scene, "fact-3", conflicting); err == nil {
		t.Fatal("contradicting fact accepted")
	}
	phase, status, reason, _, _, _ := runFields(t, store, scene.run)
	if phase.String != "done" || status != "failed" || reason != "workspace_unavailable" {
		t.Fatalf("late fact resurrected the run: %s/%s/%s", phase.String, status, reason)
	}
	if n := countRunActivities(t, store, scene.issue, "run.failed"); n != 1 {
		t.Fatalf("late fact re-announced failure: %d", n)
	}
}

// Late clone and creation facts use the same receipt path: the clone's real outcome lands on its
// original execution row, the creation's real outcome is evidence only, and neither reopens
// anything.
func TestProvisioningFactReceiptsLateCloneAndCreationOutcomes(t *testing.T) {
	store := commandStore(t)
	scene := seedAbandonedCreation(t, store, false)
	// A clone execution the creation dispatched before blocking, whose result stayed unknown.
	execution := "exec-late-clone"
	if _, err := store.Pool.Exec(`INSERT INTO clone_executions(execution_id,operation_id,workspace_id,node_id,input,dispatched_epoch,node_operation_id)
	 VALUES($1,$2,$3,'node-late','{"kind":"clone"}',1,$1::text)`, execution, scene.operation, scene.workspace); err != nil {
		t.Fatal(err)
	}
	cloneFact := Object{
		"epoch": 1, "kind": "clone", "executionId": execution,
		"result": Object{
			"outcome": "clone_failed", "reason": "CLONE_FAILURE_REASON_BRANCH_NOT_FOUND",
			"node": Object{"nodeId": "node-late", "nodeIncarnationId": "inc-late"},
		},
	}
	if _, err := submitFact(t, store, scene, "clone-1", cloneFact); err != nil {
		t.Fatalf("late clone fact refused: %v", err)
	}
	var outcome string
	if err := store.Pool.QueryRow(`SELECT result->>'outcome' FROM clone_executions WHERE execution_id=$1`, execution).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "clone_failed" {
		t.Fatalf("clone fact not stored on the original execution: %s", outcome)
	}
	if _, err := submitFact(t, store, scene, "clone-2", cloneFact); err != nil {
		t.Fatalf("identical clone fact must replay: %v", err)
	}
	if _, err := submitFact(t, store, scene, "creation-1", Object{"epoch": 1, "kind": "creation", "outcome": "ready"}); err != nil {
		t.Fatalf("late creation fact refused: %v", err)
	}
	if _, err := submitFact(t, store, scene, "creation-1", Object{"epoch": 1, "kind": "creation", "outcome": "ready"}); err != nil {
		t.Fatalf("creation fact submission replay failed: %v", err)
	}
	if _, err := submitFact(t, store, scene, "creation-2", Object{"epoch": 1, "kind": "creation", "outcome": "failed"}); err == nil || !strings.Contains(err.Error(), "fact_conflict") {
		t.Fatalf("contradicting creation outcome accepted: %v", err)
	}
	var creationState string
	if err := store.Pool.QueryRow(`SELECT responsibility->'lateFacts'->'creation'->>'state' FROM agent_run_residuals WHERE run_id=$1`, scene.run).Scan(&creationState); err != nil {
		t.Fatal(err)
	}
	if creationState != "ready" {
		t.Fatalf("creation fact not receipted: %s", creationState)
	}
	// A late success is evidence only: the run keeps its business terminal and the Workspace keeps
	// its admission fence.
	var admissionOpen bool
	if err := store.Pool.QueryRow(`SELECT admission_open FROM workspaces WHERE id=$1`, scene.workspace).Scan(&admissionOpen); err != nil {
		t.Fatal(err)
	}
	phase, status, _, _, _, _ := runFields(t, store, scene.run)
	if admissionOpen || phase.String != "done" || status != "failed" {
		t.Fatalf("late creation success reopened the run: admission=%v %s/%s", admissionOpen, phase.String, status)
	}
}

// The entry is closed to everything that is not a Cloud-abandoned creation, and an invalid fact
// kind or outcome is a client error, not a stored half-fact.
func TestProvisioningFactAcceptsOnlyAbandonedCreations(t *testing.T) {
	store := commandStore(t)
	scene := seedAbandonedCreation(t, store, false)

	// An unknown kind is rejected outright.
	if _, err := submitFact(t, store, scene, "bad-1", Object{"epoch": 1, "kind": "session"}); err == nil || !strings.Contains(err.Error(), "invalid_fact_kind") {
		t.Fatalf("unknown fact kind accepted: %v", err)
	}
	// An invalid creation outcome is rejected.
	if _, err := submitFact(t, store, scene, "bad-2", Object{"epoch": 1, "kind": "creation", "outcome": "maybe"}); err == nil || !strings.Contains(err.Error(), "invalid_outcome") {
		t.Fatalf("invalid outcome accepted: %v", err)
	}

	// An unfinished creation is not a residual: its facts must go through the normal actions. The
	// lease the abandoned scene seeded still covers this store.
	live := seedProvisioningCreation(t, store)
	if _, err := store.Control(context.Background(), &ControlRequest{
		Action: "provisioning_fact", OperationID: live.operation,
		Body: Object{"epoch": 1, "kind": "creation", "outcome": "ready"}, Service: controllerClaims(),
	}); err == nil || !strings.Contains(err.Error(), "operation_not_abandoned") {
		t.Fatalf("fact accepted for an unfinished creation: %v", err)
	}

	// A creation that failed on the Controller's own evidence settled through the ordinary hook;
	// its facts are not residuals either.
	if _, err := store.Pool.Exec(`UPDATE operations SET state='failed',error_code='clone_failed' WHERE id=$1`, live.operation); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Control(context.Background(), &ControlRequest{
		Action: "provisioning_fact", OperationID: live.operation,
		Body: Object{"epoch": 1, "kind": "creation", "outcome": "ready"}, Service: controllerClaims(),
	}); err == nil || !strings.Contains(err.Error(), "operation_not_abandoned") {
		t.Fatalf("fact accepted for an ordinarily failed creation: %v", err)
	}
}

// After the abandonment the creation's own actions stay closed: no claim, no defer, no advance and
// no effect result through the active-operation gates. The fact entry is the only way in.
func TestProvisioningFactDoesNotReopenTheOperationsOwnActions(t *testing.T) {
	store := commandStore(t)
	scene := seedAbandonedCreation(t, store, true)
	closed := []struct {
		name   string
		action string
		body   Object
	}{
		{"defer", "defer", Object{"epoch": 1, "version": 99, "state": "retry_wait", "errorCode": "node_unavailable", "retrySeconds": 30}},
		{"advance", "advance", Object{"epoch": 1, "version": 99}},
		{"plan", "plan", Object{"epoch": 1, "version": 99, "kind": "sandbox_ensure", "workspaceId": scene.workspace}},
		{"effect result", "effect_result", Object{"epoch": 1, "version": 99, "state": "succeeded", "externalId": "sub-x", "result": Object{}}},
	}
	for _, tc := range closed {
		if _, err := store.Control(context.Background(), &ControlRequest{
			Action: tc.action, OperationID: scene.operation, EffectID: scene.effect, Body: tc.body, Service: scene.claims,
		}); err == nil {
			t.Fatalf("%s accepted on an abandoned creation", tc.name)
		}
	}
	// Claim answers nothing: the failed creation is not claimable work anymore.
	claimed, err := store.Control(context.Background(), &ControlRequest{Action: "claim", Body: Object{"epoch": 1}, Service: scene.claims})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed["operation"] != nil {
		t.Fatalf("claim picked up an abandoned creation: %v", claimed["operation"])
	}
}
