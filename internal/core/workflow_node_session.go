package core

import (
	"context"
	"encoding/json"
	"strconv"
)

// NodeSessionPageMax is the largest page the node session read returns. It is higher than the
// shared 100-row window because a conversation page is read newest-first and a long exchange should
// not need two pages; the page byte budget bounds the response independently of this count.
const NodeSessionPageMax = 200

const (
	nodeSessionPageDefault = 100
	// nodeSessionPageBytes stops a page at its byte budget before the record count. A single receipt
	// may carry 256 KiB, so a 200-row page could otherwise reach ~50 MiB. The budget is far below
	// SessionHistory's 32 MiB whole-history cap because pages, not whole histories, are what this
	// route returns.
	nodeSessionPageBytes = 4 << 20
)

// The read reports what the selected attempt's evidence supports; an empty list never stands in
// for any of these facts (node session read decision D4).
const (
	// nodeSessionAvailable: the attempt has history receipts (its history may still be incomplete).
	nodeSessionAvailable = "available"
	// nodeSessionNotApplicable: the node has no session at all (a non-agent node).
	nodeSessionNotApplicable = "not_applicable"
	// nodeSessionNotStarted: the attempt exists in the node's state but no session execution was
	// ever dispatched, so there is nothing to read yet.
	nodeSessionNotStarted = "not_started"
	// nodeSessionAwaitingFirstRecord: the session is in flight and has recorded nothing yet.
	nodeSessionAwaitingFirstRecord = "awaiting_first_record"
	// nodeSessionUnavailable: the attempt is terminal but left no replayable history.
	nodeSessionUnavailable = "unavailable"
	// nodeSessionUnknown: the attempt has an execution row and no history, and neither its result
	// nor the node's current state can prove it settled.
	nodeSessionUnknown = "unknown"
)

// workflowNodeSessionRead serves the node session read: one agent node attempt's recorded
// conversation, paged from the newest history receipt backwards.
//
// Authorization is the workflow model read's template taken down to the node: an active tenant
// member, a live workflow, and a live run belonging to that workflow. The node id is a graph id
// resolved against the run's frozen snapshot, and the attempt and execution are resolved from the
// run's committed node state, so no caller-supplied session or execution identifier can widen the
// scope. Reading never opens a session, advances a node, or writes anything.
func (s *Store) workflowNodeSessionRead(ctx context.Context, r *PublicRequest) (Object, int, error) {
	out, err := s.transact(ctx, func(t *transaction) Object {
		uid := sessionReadIdentity(t, r.Identity)
		membership(t, r.TenantID, uid, false)
		workflow(t, r.TenantID, r.WorkflowID)
		require(validID(r.RunID), 404, "not_found")
		run := t.one("SELECT id, snapshot_id, node_states FROM workflow_runs WHERE id=$1 AND tenant_id=$2 AND workflow_id=$3 AND deleted_at IS NULL", r.RunID, r.TenantID, r.WorkflowID)
		require(run != nil, 404, "not_found")
		graph := Object{}
		if snapshot := t.one("SELECT graph FROM workflow_snapshots WHERE id=$1", run.S("snapshotId")); snapshot != nil {
			graph = snapshot.O("graph")
		}
		view, known := readWorkflowGraph(graph).ByNode[r.NodeID]
		require(known, 404, "not_found")
		states := workflowStatesFromJSON(run.O("nodeStates"))
		// The node is known because the frozen graph contains it, and that is what D2 gates on. A run
		// that has not reached this node yet committed no state for it; that is "no session was
		// established" (D4), not an unknown resource, so the zero state reads as not_started instead
		// of turning a real empty state into 404.
		current := states[r.NodeID]
		node := nodeSessionNodeState(&current)
		if view.Kind != "agent" {
			return Object{"execution": nodeSessionEmptyExecution(0, nodeSessionNotApplicable), "node": node}
		}
		attempt, ok := nodeSessionAttempt(r.Attempt, &current)
		require(ok, 404, "not_found")
		before, hasBefore := nodeSessionBefore(r)
		execution := t.one(`SELECT execution_id, result, last_event_sequence FROM node_executions
			WHERE operation_id=$1 AND workflow_node_id=$2 AND workflow_node_attempt=$3 AND kind='agent_session'`,
			r.RunID, r.NodeID, attempt)
		if execution == nil {
			return Object{"execution": nodeSessionEmptyExecution(attempt, nodeSessionNotStarted), "node": node}
		}
		result, terminal := nodeSessionResult(execution)
		return Object{"execution": nodeSessionPage(t, execution, attempt, result, terminal, before, hasBefore, r.Limit, &current), "node": node}
	})
	return out, 200, err
}

// nodeSessionNodeState projects the node's current committed state. These fields describe the
// current attempt only: after a retry the node's output and end reason belong to the new attempt,
// so a reader of an older attempt must not present them as that attempt's answer (D1).
func nodeSessionNodeState(state *workflowNodeState) Object {
	// A committed state always carries a status, so an empty one means the run never reached this
	// node: the key stays present and null rather than reporting an empty status a client would have
	// to special-case.
	var status any
	if state.Status != "" {
		status = state.Status
	}
	out := Object{"status": status, "attempt": state.attemptNumber(), "error": nil, "output": nil, "sessionEndReason": nil}
	if state.Error != "" {
		out["error"] = state.Error
	}
	if state.Output != nil {
		out["output"] = state.Output
	}
	if state.SessionEndReason != "" {
		out["sessionEndReason"] = state.SessionEndReason
	}
	return out
}

// nodeSessionAttempt resolves the requested attempt against the node's committed attempt count.
// An absent attempt means the node's current one. A malformed attempt is a pagination fault; one
// beyond the node's committed attempts addresses nothing and is 404, the same answer a foreign
// run or node gets.
func nodeSessionAttempt(raw string, state *workflowNodeState) (int64, bool) {
	current := state.attemptNumber()
	if raw == "" {
		return current, true
	}
	attempt, e := strconv.ParseInt(raw, 10, 64)
	require(e == nil && attempt >= 1, 400, "invalid_pagination")
	return attempt, attempt <= current
}

// nodeSessionBefore reads the page cursor. A cursor without an attempt would be resolved against
// "the current attempt", which a retry can change between pages and thereby splice another
// attempt's receipts onto this page; the decision therefore refuses that combination.
func nodeSessionBefore(r *PublicRequest) (int64, bool) {
	require(r.Before == "" || r.Attempt != "", 400, "invalid_pagination")
	if r.Before == "" {
		return 0, false
	}
	before, e := strconv.ParseInt(r.Before, 10, 64)
	require(e == nil && before >= 1, 400, "invalid_pagination")
	return before, true
}

// nodeSessionResult separates the execution's committed terminal evidence from the absence of it.
// A non-null result is Node-reported (or a settled retryable failure) and always means the attempt
// settled. A null result proves nothing by itself: Cloud settles several terminal paths (unreachable
// Node, turn/start/end deadlines, cancellation) without writing a result.
func nodeSessionResult(execution Object) (Object, bool) {
	result, ok := execution["result"].(map[string]any)
	if !ok || len(result) == 0 {
		return nil, false
	}
	return Object(result), true
}

// nodeSessionEmptyExecution is the page envelope for an attempt with no readable session.
func nodeSessionEmptyExecution(attempt int64, state string) Object {
	return Object{
		"attempt": attempt, "executionId": nil, "result": nil, "records": []any{},
		"state": state, "incomplete": false, "hasMore": false, "nextBefore": nil,
	}
}

// nodeSessionPage reads one page of an attempt's history receipts and decides the read state from
// the whole attempt's evidence, not from the page.
func nodeSessionPage(t *transaction, execution Object, attempt int64, result Object, terminal bool, before int64, hasBefore bool, limit int, state *workflowNodeState) Object {
	if limit == 0 {
		limit = nodeSessionPageDefault
	}
	// The terminal receipt is the row at last_event_sequence, and nodeExecutionResult writes it in
	// the same transaction as result. Only that committed evidence identifies it: a terminal event is
	// an arbitrary string, so it may itself look like a history record and its shape proves nothing.
	executionID := execution.S("executionId")
	var terminalSequence any
	if terminal {
		terminalSequence = execution.N("lastEventSequence")
	}
	var beforeArg any
	if hasBefore {
		beforeArg = before
	}
	counts := t.one(`SELECT COUNT(*) AS total, COALESCE(MIN(sequence),0) AS oldest, COALESCE(MAX(sequence),0) AS newest
		FROM node_event_receipts WHERE execution_id=$1 AND ($2::bigint IS NULL OR sequence <> $2::bigint)`, executionID, terminalSequence)
	history := counts.N("total")
	// A history receipt carries the canonEvent object takeOverThreadEvents stored: the record string,
	// the receipt's own sequence, its turn, and the optional bounded projection. The window keeps the
	// newest rows until the byte budget is reached; the row that starts below the budget is always
	// included so a page can never come back empty while receipts remain. The page is taken newest
	// first and returned oldest first, which is the ordering the neighboring conversation reads use
	// (thread_read.go, task_conversation_messages.go): a client appends pages without a second case.
	rows := t.list(`WITH history AS MATERIALIZED (
			SELECT sequence, octet_length(event) AS bytes,
				CASE WHEN event IS JSON OBJECT THEN event::jsonb END AS canon
			FROM node_event_receipts
			WHERE execution_id=$1 AND ($2::bigint IS NULL OR sequence <> $2::bigint)
			  AND ($3::bigint IS NULL OR sequence < $3::bigint)
		), budgeted AS (
			SELECT sequence, bytes, canon,
				SUM(bytes) OVER (ORDER BY sequence DESC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS running
			FROM history
		), page AS (
			SELECT sequence, canon FROM budgeted
			WHERE running - bytes < $4::bigint
			ORDER BY sequence DESC LIMIT $5
		)
		SELECT sequence, canon::text AS canon FROM page ORDER BY sequence`, executionID, terminalSequence, beforeArg, nodeSessionPageBytes, limit)
	records := make([]any, 0, len(rows))
	incomplete := false
	for _, row := range rows { // ascending, as returned to the caller
		record, shaped := nodeSessionRecord(row)
		if !shaped {
			// The row survived the terminal-sequence exclusion but is not a history receipt Cloud
			// wrote. That is missing history, not a record to render: the page says so.
			incomplete = true
			continue
		}
		records = append(records, record)
	}
	// The cursor is the earliest sequence this page covered, which for an intact page is exactly
	// the earliest record it returned — the first row, since the page arrives ascending. A page
	// whose rows were all unshaped still has to advance: pointing at an unrendered row re-reads only
	// that row (and reports it again) instead of stranding every earlier receipt behind a page that
	// returned nothing.
	nextBefore := int64(0)
	if len(rows) > 0 {
		nextBefore = rows[0].N("sequence")
	}
	if nodeSessionGap(rows, before, hasBefore, counts.N("newest")) {
		incomplete = true
	}
	more := false
	if nextBefore > 0 {
		more = t.one(`SELECT EXISTS(SELECT 1 FROM node_event_receipts WHERE execution_id=$1
			AND ($2::bigint IS NULL OR sequence <> $2::bigint) AND sequence < $3) AS more`, executionID, terminalSequence, nextBefore).B("more")
	}
	read := nodeSessionState(history, terminal, state, attempt)
	page := Object{
		"attempt": attempt, "executionId": executionID, "result": nil, "records": records,
		"state": read, "incomplete": incomplete, "hasMore": more, "nextBefore": nil,
	}
	if result != nil {
		page["result"] = result
	}
	if more {
		page["nextBefore"] = nextBefore
	}
	return page
}

// nodeSessionRecord projects one stored receipt into a record, and reports whether it is shaped like
// the history receipt it claims to be. The inner record string is parsed here so the response carries
// the object the frontend's HistoryRecord decoder expects, never a re-escaped string.
func nodeSessionRecord(row Object) (Object, bool) {
	canon := Object{}
	if raw := row.S("canon"); raw != "" {
		if e := json.Unmarshal([]byte(raw), &canon); e != nil {
			return nil, false
		}
	}
	text := canon.S("record")
	var record Object
	if text == "" || json.Unmarshal([]byte(text), &record) != nil || record == nil {
		return nil, false
	}
	out := Object{"sequence": row.N("sequence"), "record": record}
	for _, key := range []string{"turnId", "visibleText"} {
		if value := canon.S(key); value != "" {
			out[key] = value
		}
	}
	if value, present := canon["historySequence"]; present {
		out["historySequence"] = value
	}
	if canon.B("truncated") {
		out["truncated"] = true
	}
	if canon.B("visibleTextTruncated") {
		out["visibleTextTruncated"] = true
	}
	return out, true
}

// nodeSessionGap reports whether the page observed missing receipts, which is a per-page
// observation: an earlier page's loss cannot be ruled out from here, so the reader keeps the
// attempt's integrity unknown until every page is read and none reported a gap. `newest` is the
// newest history receipt overall, so it is already the sequence the first page must start at; the
// terminal receipt is excluded from that count, never subtracted a second time.
func nodeSessionGap(rows []Object, before int64, hasBefore bool, newest int64) bool {
	expected := newest
	if hasBefore && before <= expected {
		expected = before - 1
	}
	if expected < 1 || len(rows) == 0 {
		return false
	}
	// Rows arrive ascending, so the walk starts at the newest expected sequence and moves down.
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].N("sequence") != expected {
			return true
		}
		expected--
	}
	return false
}

// nodeSessionState names the read state from the selected attempt's evidence (D4). Terminal evidence
// is the union of the execution's own result and — while the node is still on this attempt — the
// node's committed status: Cloud settles several terminal paths (unreachable Node, the session
// deadlines, cancellation) without writing a result, so a null result is never in-flight evidence on
// its own.
func nodeSessionState(history int64, terminal bool, state *workflowNodeState, attempt int64) string {
	if history > 0 {
		return nodeSessionAvailable
	}
	if terminal {
		return nodeSessionUnavailable
	}
	if state.attemptNumber() != attempt {
		// A retry happens only after its attempt settled with a result, so an earlier attempt without
		// one is contradictory data. Report it as unknown rather than inventing a settled or live read.
		return nodeSessionUnknown
	}
	switch state.Status {
	case "succeeded", "failed", "cancelled":
		return nodeSessionUnavailable
	case "running":
		return nodeSessionAwaitingFirstRecord
	}
	// `retrying` without a result and a node the run never reached are both states this attempt's
	// evidence cannot resolve into a session that is live or one that ended.
	return nodeSessionUnknown
}
