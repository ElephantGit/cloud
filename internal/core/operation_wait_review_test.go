package core

import (
	"errors"
	"testing"
)

// A known result whose original Node ended must not be replaced by a scheduling timeout.
func TestOperationWaitRefusedKnownResultTakeoverPreservesRound(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	if _, err := s.Pool.Exec("UPDATE operations SET node_unavailable_attempts=3 WHERE id=$1", a.operation); err != nil {
		t.Fatal(err)
	}
	op := operationWaitClaim(t, s)
	execution := operationWaitDispatch(t, s, a)
	operationWaitAge(t, s, execution, 1801)
	operationWaitResult(t, s, a, execution, "clone_ready")
	if _, err := s.Pool.Exec("UPDATE node_instances SET ended_at=clock_timestamp() WHERE workspace_id=$1", a.workspace); err != nil {
		t.Fatal(err)
	}
	read := func() Object {
		out, err := s.transact(t.Context(), func(tx *transaction) Object {
			return Object{
				"operation": tx.one("SELECT * FROM operations WHERE id=$1", a.operation),
				"execution": tx.one("SELECT * FROM clone_executions WHERE execution_id=$1", execution),
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := read()
	// If deferCloneWait returned false, Control would persist the requested timeout instead.
	operationWaitDefer(t, s, op, "clone_result_timeout")
	if after := read(); jsonText(after) != jsonText(before) {
		t.Fatalf("refused takeover changed the round or known execution: before=%v after=%v", before, after)
	}
}

func TestOperationWaitUnavailableExpiredExecutionPreservesCounter(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	if _, err := s.Pool.Exec("UPDATE operations SET node_unavailable_attempts=3 WHERE id=$1", a.operation); err != nil {
		t.Fatal(err)
	}
	op := operationWaitClaim(t, s)
	execution := operationWaitDispatch(t, s, a)
	operationWaitAge(t, s, execution, 1801)
	out := operationWaitDefer(t, s, op, "node_unavailable")
	if out.S("state") != "blocked" || out.S("errorCode") != "clone_result_timeout" || out.N("nodeUnavailableAttempts") != 3 {
		t.Fatalf("unavailable expired execution did not retain its count at timeout: %v", out)
	}
}

func TestOperationWaitPendingRejectsNonCloneStep(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, false)
	seedControllerLease(t, s)
	op := operationWaitClaim(t, s)
	_, err := s.Control(t.Context(), &ControlRequest{
		Action: "defer", OperationID: a.operation, Service: controllerClaims(),
		Body: Object{"epoch": 1, "version": op.N("version"), "state": "retry_wait", "errorCode": "clone_result_pending", "retrySeconds": 0},
	})
	var fault *Fault
	if !errors.As(err, &fault) || fault.Status != 409 || fault.Code != "invalid_step" {
		t.Fatalf("pending on non-clone step: %v, want 409 invalid_step", err)
	}
}

func TestOperationWaitPendingRequiresExecution(t *testing.T) {
	s := operationWaitDB(t)
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	op := operationWaitClaim(t, s)
	_, err := s.Control(t.Context(), &ControlRequest{
		Action: "defer", OperationID: a.operation, Service: controllerClaims(),
		Body: Object{"epoch": 1, "version": op.N("version"), "state": "retry_wait", "errorCode": "clone_result_pending", "retrySeconds": 0},
	})
	var fault *Fault
	if !errors.As(err, &fault) || fault.Status != 409 || fault.Code != "clone_incomplete" {
		t.Fatalf("pending without an execution: %v, want 409 clone_incomplete", err)
	}
}
