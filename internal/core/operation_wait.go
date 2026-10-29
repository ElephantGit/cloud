package core

// deferUnavailableNode owns the count and retry time in one write. Bound the exponent before
// computing it, and saturate the durable count too: neither can overflow after long outages.
func deferUnavailableNode(t *transaction, o Object) {
	t.exec(`UPDATE operations SET state='retry_wait',error_code='node_unavailable',
 retry_at=clock_timestamp()+(LEAST(5*power(2,LEAST(node_unavailable_attempts,4)),60)*interval '1 second'),
 node_unavailable_attempts=CASE WHEN node_unavailable_attempts<9223372036854775807
 THEN node_unavailable_attempts+1 ELSE node_unavailable_attempts END,
 version=version+1,updated_at=now() WHERE id=$1`, o.S("id"))
}

// deferCloneWait rechecks result and durable age inside the persistence transaction, after the
// Controller's observation. Result takeover uses this same transaction lock, so a known result
// cannot be overwritten by timeout; a later result can still resume the original execution.
func deferCloneWait(t *transaction, o Object, code string) bool {
	if code != "clone_result_pending" && code != "clone_result_timeout" && code != "node_unavailable" {
		return false
	}
	if o.S("step") != "clone" {
		require(code == "node_unavailable", 409, "invalid_step")
		return false
	}
	e := t.one(`SELECT *,created_at+interval '1800 seconds'<=clock_timestamp() AS result_expired
 FROM clone_executions WHERE operation_id=$1 ORDER BY created_at DESC,execution_id DESC LIMIT 1`, o.S("id"))
	if code == "node_unavailable" && e == nil {
		return false
	}
	require(e != nil, 409, "clone_incomplete")
	if len(e.O("result")) > 0 {
		resumeWorkspaceCloneResult(t, o, cloneRecoveryTimeout)
		return true
	}
	if e.B("resultExpired") {
		parkOperation(t, o, "blocked", "clone_result_timeout", 0)
		return true
	}
	if code == "node_unavailable" {
		return false
	}
	// A Controller clock running ahead cannot shorten the authoritative result window.
	parkOperation(t, o, "retry_wait", "clone_result_pending", 1)
	return true
}
