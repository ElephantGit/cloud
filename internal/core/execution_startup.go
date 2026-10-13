package core

import "time"

// agentStartupEvidence is control-plane evidence read under the dispatch transaction lock.
// Registration is a conservative boundary: even a missing acknowledgement can have reached Node.
type agentStartupEvidence struct {
	registered       bool
	modelUnavailable bool
	expired          bool
	endReason        string
}

// inspectAgentStartup keeps execution, command and Node table reads with their control-plane owner.
func inspectAgentStartup(t *transaction, run Object, timeout time.Duration) agentStartupEvidence {
	rid, wid := run.S("id"), run.S("workspaceId")
	work := t.one("SELECT * FROM execution_work WHERE run_id=$1 AND kind='agent_session' ORDER BY created_at,id LIMIT 1", rid)
	execution := t.one("SELECT * FROM node_executions WHERE operation_id=$1 AND kind='agent_session' ORDER BY created_at,execution_id LIMIT 1", rid)
	evidence := agentStartupEvidence{registered: execution != nil || work != nil && work.S("executionId") != ""}
	command := t.one("SELECT body FROM thread_commands WHERE run_id=$1 AND kind='end_session' ORDER BY queue_sequence LIMIT 1", rid)
	if command != nil {
		evidence.endReason = command.O("body").S("reason")
	}
	input := run.O("input")
	if work != nil {
		input = work.O("input")
	}
	if input.S("modelBindingId") != "" {
		// Only an initialized, reachable current incarnation supplies definitive capability evidence.
		// The connected flag can outlive its heartbeat, so use dispatch's database freshness bound.
		node := t.one(`SELECT n.model_proxy FROM node_instances n
			JOIN sandbox_instances si ON si.id=n.sandbox_instance_id
			JOIN workspaces w ON w.id=n.workspace_id AND w.runtime_generation=si.generation
			WHERE n.workspace_id=$1 AND n.connection_state='connected' AND n.initialized
			  AND n.ended_at IS NULL AND si.terminated_at IS NULL
			  AND n.last_seen_at>clock_timestamp()-interval '30 seconds'`, wid)
		evidence.modelUnavailable = node != nil && !node.B("modelProxy")
	}
	if timeout > 0 {
		// Accepted user turns update the run timestamp. They cannot renew the startup budget.
		evidence.expired = t.one(`SELECT ir.id FROM issue_runs ir
			WHERE ir.id=$1 AND COALESCE(
			 (SELECT min(created_at) FROM node_executions WHERE operation_id=ir.id AND kind='agent_session'),
			 (SELECT min(created_at) FROM execution_work WHERE run_id=ir.id AND kind='agent_session'),
			 (SELECT max(updated_at) FROM operations WHERE workspace_id=ir.workspace_id AND kind='create_workspace' AND state='succeeded'),
			 ir.updated_at) < now()-make_interval(secs => $2)`, rid, timeout.Seconds()) != nil
	}
	return evidence
}

// withdrawUndispatchedAgentSession retires only intent that no registered execution could have used.
// Dispatch registration shares this transaction lock. A late registration therefore finds no work.
func withdrawUndispatchedAgentSession(t *transaction, runID string) bool {
	if t.one("SELECT execution_id FROM node_executions WHERE operation_id=$1 AND kind='agent_session'", runID) != nil ||
		t.one("SELECT id FROM execution_work WHERE run_id=$1 AND kind='agent_session' AND execution_id IS NOT NULL", runID) != nil ||
		t.one("SELECT id FROM thread_commands WHERE run_id=$1 AND delivered_execution_id IS NOT NULL", runID) != nil {
		return false
	}
	t.exec("DELETE FROM thread_commands WHERE run_id=$1 AND delivered_execution_id IS NULL", runID)
	t.exec("DELETE FROM execution_work WHERE run_id=$1 AND kind='agent_session' AND execution_id IS NULL", runID)
	return true
}
