package core

import (
	"strings"
	"testing"
)

type operationWaitScene struct {
	operation, workspace, node string
	input                      Object
}

// The fixture stages a real operation at node/clone; every transition under test uses Control.
func seedOperationWait(t *testing.T, s *Store, clone bool) operationWaitScene {
	t.Helper()
	seed := seedDispatchScene(t, s.Pool, false)
	scene := operationWaitScene{operation: newID()}
	if err := s.Pool.QueryRow("SELECT id FROM workspaces WHERE project_id=$1 AND kind='main'", seed.project).Scan(&scene.workspace); err != nil {
		t.Fatal(err)
	}
	_, scene.node = bindConnectedNode(t, s, scene.workspace)
	step := "node"
	request := Object{}
	if clone {
		step = "clone"
		scene.input = Object{"kind": "clone", "repositoryUrl": "https://git.invalid/snapshot.git", "snapshotRef": "refs/ora/upstreams/b/snapshots/" + baseCommit, "baseCommit": baseCommit, "workspaceRef": GitWorkspaceRefPrefix(scene.workspace) + "main"}
		request["clone"] = scene.input
	}
	if _, err := s.Pool.Exec(`INSERT INTO operations(id,tenant_id,actor_user_id,project_id,workspace_id,kind,state,step,request,idempotency_key,request_hash)
 SELECT $1::uuid,tenant_id,owner_user_id,project_id,id,'create_workspace','queued',$3,$4,($1::uuid)::text,'fixture'
 FROM workspaces WHERE id=$2`, scene.operation, scene.workspace, step, jsonText(request)); err != nil {
		t.Fatal(err)
	}
	return scene
}

func operationWaitControl(t *testing.T, s *Store, action, id string, body Object) Object {
	t.Helper()
	out, err := s.Control(t.Context(), &ControlRequest{Action: action, OperationID: id, Body: body, Service: controllerClaims()})
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	return out
}

func operationWaitRow(t *testing.T, s *Store, id string) Object {
	t.Helper()
	out, err := s.transact(t.Context(), func(tx *transaction) Object { return tx.one("SELECT * FROM operations WHERE id=$1", id) })
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func operationWaitClaim(t *testing.T, s *Store) Object {
	return operationWaitControl(t, s, "claim", "", Object{"epoch": 1}).O("operation")
}

func operationWaitDefer(t *testing.T, s *Store, op Object, code string) Object {
	state := "retry_wait"
	if code == "clone_result_unknown" {
		state = "blocked"
	}
	delay := 0
	if code == "clone_result_unknown" {
		delay = 300
	}
	return operationWaitControl(t, s, "defer", op.S("id"), Object{"epoch": 1, "version": op.N("version"), "state": state, "errorCode": code, "retrySeconds": delay})
}

func operationWaitDue(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.Pool.Exec("UPDATE operations SET retry_at=clock_timestamp()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func operationWaitDelay(t *testing.T, s *Store, id string, seconds int) {
	t.Helper()
	var delay float64
	if err := s.Pool.QueryRow("SELECT extract(epoch FROM retry_at-updated_at) FROM operations WHERE id=$1", id).Scan(&delay); err != nil {
		t.Fatal(err)
	}
	if delay < float64(seconds) || delay > float64(seconds)+1 {
		t.Fatalf("server retry delay %.6f, want %d seconds", delay, seconds)
	}
}

func operationWaitDispatch(t *testing.T, s *Store, scene operationWaitScene) string {
	t.Helper()
	execution := newID()
	operationWaitControl(t, s, "clone_dispatch", "", Object{"epoch": 1, "operationId": scene.operation, "executionId": execution, "nodeId": scene.node, "input": scene.input})
	return execution
}

func operationWaitAge(t *testing.T, s *Store, execution string, seconds int) {
	t.Helper()
	if _, err := s.Pool.Exec("UPDATE clone_executions SET created_at=clock_timestamp()-($2*interval '1 second') WHERE execution_id=$1", execution, seconds); err != nil {
		t.Fatal(err)
	}
}

func operationWaitResult(t *testing.T, s *Store, scene operationWaitScene, execution, outcome string) {
	t.Helper()
	result := Object{"outcome": outcome, "node": Object{"nodeId": scene.node, "nodeIncarnationId": "fixture"}}
	if outcome == "clone_ready" {
		result["commit"], result["path"] = baseCommit, "/workspace"
	} else {
		result["reason"] = "CLONE_FAILURE_REASON_OPERATION_FAILED"
	}
	operationWaitControl(t, s, "clone_takeover", "", Object{"epoch": 1, "operationId": scene.operation, "executionId": execution, "result": result, "sequence": 0, "event": "terminal-" + outcome})
}

func TestOperationWaitBackoffPersistsAndResetsOnAdvance(t *testing.T) {
	s := operationWaitDB(t)
	scene := seedOperationWait(t, s, false)
	seedControllerLease(t, s)
	for i, delay := range []int{5, 10, 20, 40, 60, 60} {
		op := operationWaitClaim(t, s)
		if op.N("nodeUnavailableAttempts") != int64(i) {
			t.Fatalf("claim reset count: %v", op)
		}
		out := operationWaitDefer(t, s, op, "node_unavailable")
		if out.N("nodeUnavailableAttempts") != int64(i+1) || out.S("state") != "retry_wait" {
			t.Fatalf("unavailable round: %v", out)
		}
		operationWaitDelay(t, s, scene.operation, delay)
		// Reconstructing Store cannot carry a counter in process memory. Manual retry preserves it.
		s = &Store{Pool: s.Pool}
		BindBusinessHooks(s)
		retried, err := s.transact(t.Context(), func(tx *transaction) Object {
			return retryOperation(tx, &PublicRequest{OperationID: scene.operation, TenantID: out.S("tenantId"), Body: Object{"version": out.N("version")}}, out.S("actorUserId"))
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, exposed := retried.O("operation")["nodeUnavailableAttempts"]; exposed {
			t.Fatal("internal counter leaked into public retry response")
		}
	}
	op := operationWaitClaim(t, s)
	advanced := operationWaitControl(t, s, "advance", scene.operation, Object{"epoch": 1, "version": op.N("version")})
	if advanced.N("nodeUnavailableAttempts") != 0 || advanced.S("step") != "clone" {
		t.Fatalf("real advance did not reset count: %v", advanced)
	}
}

func TestOperationWaitReplayFenceAndRollback(t *testing.T) {
	s := operationWaitDB(t)
	scene := seedOperationWait(t, s, false)
	seedControllerLease(t, s)
	op := operationWaitClaim(t, s)
	req := &ControlRequest{Action: "defer", OperationID: scene.operation, SubmissionID: "one-round", Service: controllerClaims(), Body: Object{"epoch": 1, "version": op.N("version"), "state": "retry_wait", "errorCode": "node_unavailable", "retrySeconds": 1}}
	first, err := s.Control(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Control(t.Context(), req)
	if err != nil || jsonText(first) != jsonText(replay) {
		t.Fatalf("replay changed round: %v / %v", replay, err)
	}
	req.SubmissionID = "stale-version"
	if _, err := s.Control(t.Context(), req); err == nil {
		t.Fatal("stale version accepted")
	}
	operationWaitDue(t, s, scene.operation)
	op = operationWaitClaim(t, s)
	_, err = s.transact(t.Context(), func(tx *transaction) Object {
		deferUnavailableNode(tx, op)
		reject(409, "injected_rollback")
		return nil
	})
	if err == nil {
		t.Fatal("rollback injection did not fail")
	}
	if row := operationWaitRow(t, s, scene.operation); row.N("nodeUnavailableAttempts") != 1 || row.S("state") != "running" || row.S("retryAt") != op.S("retryAt") {
		t.Fatalf("partial defer committed: %v", row)
	}
	if _, err := s.Pool.Exec("UPDATE controller_leases SET epoch=2 WHERE name='global'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Control(t.Context(), req); err == nil || !strings.Contains(err.Error(), "stale_controller") {
		t.Fatalf("old epoch accepted: %v", err)
	}
	recovered := operationWaitControl(t, s, "claim", "", Object{"epoch": 2}).O("operation")
	if recovered.N("nodeUnavailableAttempts") != 1 || recovered.N("controllerEpoch") != 2 {
		t.Fatalf("epoch handover reset count: %v", recovered)
	}
}

func TestOperationWaitCompletionResetsCounter(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, false)
	seedControllerLease(t, s)
	if _, err := s.Pool.Exec("UPDATE operations SET kind='start',node_unavailable_attempts=3 WHERE id=$1", a.operation); err != nil {
		t.Fatal(err)
	}
	op := operationWaitClaim(t, s)
	done := operationWaitControl(t, s, "advance", a.operation, Object{"epoch": 1, "version": op.N("version")})
	if done.S("state") != "succeeded" || done.N("nodeUnavailableAttempts") != 0 {
		t.Fatalf("completion did not reset counter: %v", done)
	}
}

func TestOperationWaitPendingYieldsAndKeepsExecution(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	op := operationWaitClaim(t, s)
	execution := operationWaitDispatch(t, s, a)
	b := seedOperationWait(t, s, false)
	operationWaitDefer(t, s, op, "node_unavailable")
	operationWaitDue(t, s, a.operation)
	op = operationWaitClaim(t, s)
	for i := 0; i < 3; i++ {
		pending := operationWaitDefer(t, s, op, "clone_result_pending")
		if pending.S("errorCode") != "clone_result_pending" || pending.N("nodeUnavailableAttempts") != 1 {
			t.Fatalf("pending changed unavailable count: %v", pending)
		}
		operationWaitDelay(t, s, a.operation, 1)
		if claimed := operationWaitClaim(t, s); claimed.S("id") != b.operation {
			t.Fatalf("pending stole B's opportunity: %v", claimed)
		}
		// B yields independently so A can be recovered once its positive interval has elapsed.
		operationWaitDefer(t, s, operationWaitRow(t, s, b.operation), "node_unavailable")
		operationWaitDue(t, s, a.operation)
		op = operationWaitClaim(t, s)
		operationWaitDue(t, s, b.operation)
	}
	var count int
	var got string
	if err := s.Pool.QueryRow("SELECT count(*),min(execution_id) FROM clone_executions WHERE operation_id=$1", a.operation).Scan(&count, &got); err != nil {
		t.Fatal(err)
	}
	if count != 1 || got != execution {
		t.Fatalf("pending replaced execution: %d %s", count, got)
	}
}

func TestOperationWaitTimeoutBoundaryAndKnownResultRace(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	op := operationWaitClaim(t, s)
	execution := operationWaitDispatch(t, s, a)
	operationWaitAge(t, s, execution, 1799)
	before := operationWaitDefer(t, s, op, "clone_result_timeout")
	if before.S("state") != "retry_wait" || before.S("errorCode") != "clone_result_pending" {
		t.Fatalf("early timeout: %v", before)
	}
	operationWaitDue(t, s, a.operation)
	op = operationWaitClaim(t, s)
	operationWaitAge(t, s, execution, 1800)
	after := operationWaitDefer(t, s, op, "clone_result_pending")
	if after.S("state") != "blocked" || after.S("errorCode") != "clone_result_timeout" {
		t.Fatalf("durable boundary not enforced: %v", after)
	}
	if claim := operationWaitClaim(t, s); len(claim) != 0 {
		t.Fatalf("blocked clone reclaimed: %v", claim)
	}
	// A result commits after Controller observed pending but before its stale timeout submission.
	if _, err := s.Pool.Exec("UPDATE operations SET state='running' WHERE id=$1", a.operation); err != nil {
		t.Fatal(err)
	}
	op = operationWaitRow(t, s, a.operation)
	operationWaitResult(t, s, a, execution, "clone_ready")
	race := operationWaitDefer(t, s, op, "clone_result_timeout")
	if race.S("state") != "queued" || race.S("errorCode") != "" {
		t.Fatalf("known result overwritten by timeout: %v", race)
	}
}

func TestOperationWaitLateSuccessAndUnknownIsolation(t *testing.T) {
	for _, code := range []string{"clone_result_timeout", "clone_result_unknown"} {
		t.Run(code, func(t *testing.T) {
			s := operationWaitDB(t)
			a := seedOperationWait(t, s, true)
			seedControllerLease(t, s)
			op := operationWaitClaim(t, s)
			execution := operationWaitDispatch(t, s, a)
			operationWaitAge(t, s, execution, 1801)
			if code == "clone_result_timeout" {
				operationWaitDefer(t, s, op, "clone_result_timeout")
				// Late known results survive expired readiness, without widening Unknown recovery.
				if _, err := s.Pool.Exec("UPDATE node_instances SET last_seen_at=clock_timestamp()-interval '1 minute',connection_state='disconnected'"); err != nil {
					t.Fatal(err)
				}
				operationWaitResult(t, s, a, execution, "clone_ready")
				if row := operationWaitRow(t, s, a.operation); row.S("state") != "queued" {
					t.Fatalf("late success did not auto-resume: %v", row)
				}
			} else {
				operationWaitDefer(t, s, op, "clone_result_unknown")
				// Age, process reconstruction and connection restoration are not known-result evidence.
				restarted := &Store{Pool: s.Pool}
				if claim := operationWaitClaim(t, restarted); len(claim) != 0 {
					t.Fatalf("unknown reclaimed: %v", claim)
				}
				if row := operationWaitRow(t, s, a.operation); row.S("state") != "blocked" || row.S("errorCode") != code {
					t.Fatalf("Unknown became timeout/recovered: %v", row)
				}
			}
		})
	}
}

func TestOperationWaitLateFailureKeepsThreeAttemptBudget(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	for attempt := 1; attempt <= 3; attempt++ {
		op := operationWaitClaim(t, s)
		execution := operationWaitDispatch(t, s, a)
		operationWaitDefer(t, s, op, "node_unavailable")
		operationWaitDue(t, s, a.operation)
		op = operationWaitClaim(t, s)
		operationWaitDefer(t, s, op, "clone_result_pending")
		operationWaitDue(t, s, a.operation)
		op = operationWaitClaim(t, s)
		operationWaitAge(t, s, execution, 1801)
		operationWaitDefer(t, s, op, "clone_result_timeout")
		operationWaitResult(t, s, a, execution, "clone_failed")
		row := operationWaitRow(t, s, a.operation)
		if attempt < 3 {
			if row.S("state") != "retry_wait" || row.S("errorCode") != "clone_failed" {
				t.Fatalf("attempt %d late failure not resumed: %v", attempt, row)
			}
			operationWaitDelay(t, s, a.operation, attempt*5)
			operationWaitDue(t, s, a.operation)
		} else if row.S("state") != "failed" || row.S("errorCode") != "clone_failed" || row.N("nodeUnavailableAttempts") != 0 {
			t.Fatalf("third failure budget lost: %v", row)
		}
		var count int
		if err := s.Pool.QueryRow("SELECT count(*) FROM clone_executions WHERE operation_id=$1", a.operation).Scan(&count); err != nil || count != attempt {
			t.Fatalf("attempt %d execution count %d: %v", attempt, count, err)
		}
	}
}

func TestOperationWaitCounterClampsBeforeExponent(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, false)
	seedControllerLease(t, s)
	if _, err := s.Pool.Exec("UPDATE operations SET node_unavailable_attempts=9223372036854775807 WHERE id=$1", a.operation); err != nil {
		t.Fatal(err)
	}
	op := operationWaitClaim(t, s)
	deferred := operationWaitDefer(t, s, op, "node_unavailable")
	var count int64
	if err := s.Pool.QueryRow("SELECT node_unavailable_attempts FROM operations WHERE id=$1", a.operation).Scan(&count); err != nil || count != 9223372036854775807 {
		t.Fatalf("count overflow: %d, %v (state %s)", count, err, deferred.S("state"))
	}
	operationWaitDelay(t, s, a.operation, 60)
}
