package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// DB tests for the workflow node session read (specs decisions/cloud/workflow
// 20261026-workflow-node-session-read.md D1–D4, specs#236).
//
// The read is proved on the shipped seams: history arrives through takeOverThreadEvents, an attempt
// settles through executionResult, a retry starts through AdvanceWorkflowRunsOnce, and every read
// goes through Store.Public exactly as the router drives it. Two kinds of state are staged directly
// because no seam produces them: the settled run Workspace (the control plane's asynchronous create),
// and — only in the cases that need it — a committed row no writer would ever produce, such as a
// receipt with a hole in its sequence or a node state that contradicts its executions.

// nodeSessionScene is one WorkflowRun staged through the seams together with the ids the reads
// address.
type nodeSessionScene struct {
	store  *Store
	runID  string
	tid    string
	wid    string
	wsID   string
	node   string // the Node service id the session dispatches through
	caller *Claims
}

// nodeSessionQuery is one read's query as the router resolves it: PublicRequest already carries the
// parsed attempt, cursor and limit, so the core never re-reads a query string.
type nodeSessionQuery struct {
	attempt string
	before  string
	limit   int
}

// newNodeSessionScene stages graph one pass away from its first agent node, then settles the run's
// Workspace so a connected Node can start the session.
func newNodeSessionScene(t *testing.T, store *Store, graph Object) *nodeSessionScene {
	t.Helper()
	runID, tid, _, _, _ := seedWorkflowSessionSceneGraph(t, store, graph)
	if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
		t.Fatalf("advance to provision the run Workspace: %v", err)
	}
	return &nodeSessionScene{
		store: store, runID: runID, tid: tid, wid: workflowRunWorkflowID(t, store, runID),
		wsID: settleSessionWorkspace(t, store, runID), caller: cancelCaller(t, store, tid),
	}
}

// startAttempt starts nodeID's next attempt through the scheduler and dispatches the session work it
// released, returning that work's spec: its initial turn id is the one the history records of this
// attempt must carry for the output aggregation to read them.
func (s *nodeSessionScene) startAttempt(t *testing.T, nodeID, execution string) Object {
	t.Helper()
	var spec Object
	if s.node == "" {
		node, started := startSessionNode(t, s.store, s.runID, s.wsID, nodeID)
		s.node, spec = node, started
	} else {
		if err := s.store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
			t.Fatalf("advance to start the next attempt of %s: %v", nodeID, err)
		}
		spec = sessionWorkSpec(t, s.store, s.runID, nodeID)
	}
	dispatchSessionAttempt(t, s.store, s.runID, nodeID, s.node, execution, spec)
	return spec
}

// history writes n contiguous history receipts from `from` in the answer shape the output
// aggregation reads, so the same records can become a published `output` once the attempt succeeds.
func (s *nodeSessionScene) history(t *testing.T, execution, turnID string, from int64, n int) {
	t.Helper()
	events := make([]Object, 0, n)
	for i := 0; i < n; i++ {
		events = append(events, workflowAnswerEvent(from+int64(i), turnID, nodeSessionChunk(from+int64(i))))
	}
	takeOverWorkflowEvents(t, s.store, s.runID, execution, events)
}

// nodeSessionChunk is the text of the receipt at sequence, short enough to keep a page small and
// distinct enough to prove a record was not swapped for its neighbor.
func nodeSessionChunk(sequence int64) string { return fmt.Sprintf("chunk %d", sequence) }

// readNodeSession drives the public read exactly as the router does.
func (s *nodeSessionScene) readNodeSession(t *testing.T, nodeID string, q nodeSessionQuery) (Object, int, error) {
	t.Helper()
	return s.store.Public(context.Background(), &PublicRequest{
		Method:   "GET",
		Path:     "/api/v1/tenants/" + s.tid + "/workflows/" + s.wid + "/runs/" + s.runID + "/nodes/" + nodeID + "/session",
		TenantID: s.tid, WorkflowID: s.wid, RunID: s.runID, NodeID: nodeID,
		Attempt: q.attempt, Before: q.before, Limit: q.limit, Identity: s.caller,
	})
}

// mustRead fails unless the read answers 200.
func (s *nodeSessionScene) mustRead(t *testing.T, nodeID string, q nodeSessionQuery) Object {
	t.Helper()
	out, status, err := s.readNodeSession(t, nodeID, q)
	if err != nil || status != 200 {
		t.Fatalf("GET node %q session %+v: status=%d err=%v", nodeID, q, status, err)
	}
	return out
}

// requireFault fails unless the read is refused with want.
func (s *nodeSessionScene) requireFault(t *testing.T, nodeID string, q nodeSessionQuery, want string) {
	t.Helper()
	out, status, err := s.readNodeSession(t, nodeID, q)
	if err == nil || ErrorCode(err).Code != want {
		t.Fatalf("GET node %q session %+v: status=%d out=%v err=%v; want %s", nodeID, q, status, out, err, want)
	}
}

// nodeSessionRecords returns a page's projected records.
func nodeSessionRecords(page Object) []Object {
	return objectsOf(page.O("execution")["records"])
}

// nodeSessionSequences renders a page's outer receipt sequences in response order.
func nodeSessionSequences(page Object) string {
	seqs := make([]string, 0, 4)
	for _, record := range nodeSessionRecords(page) {
		seqs = append(seqs, strconv.FormatInt(record.N("sequence"), 10))
	}
	return strings.Join(seqs, ",")
}

// insertRawReceipt writes one receipt row directly. It is the only way to stage the corrupted
// history the takeover path refuses: a gap, or a body it would never accept.
func insertRawReceipt(t *testing.T, store *Store, execution string, sequence int64, event string) {
	t.Helper()
	if _, err := store.Pool.Exec(`INSERT INTO node_event_receipts(execution_id,sequence,event) VALUES($1,$2,$3)`, execution, sequence, event); err != nil {
		t.Fatalf("insert raw receipt %d: %v", sequence, err)
	}
}

// rawHistoryReceipt is the canonEvent body takeOverThreadEvents stores, so a hand-inserted row is
// indistinguishable from a stored one except by what surrounds it.
func rawHistoryReceipt(sequence int64) string {
	return jsonText(Object{
		"record":   workflowAnswerEvent(sequence, "turn-1", nodeSessionChunk(sequence)).S("record"),
		"sequence": sequence, "truncated": false, "turnId": "turn-1",
	})
}

// The read is addressed by (run, node, attempt). A retry must not let one attempt inherit the
// other's records, result or state, and a page must stay pinned to the attempt it started on.
func TestWorkflowNodeSessionReadScopesToTheRequestedAttempt(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowRetryGraph(true, 2, 300))
	first := "exec-a-1-" + scene.runID[:8]
	scene.startAttempt(t, "a", first)
	scene.history(t, first, "turn-1", 1, 3)
	reportSessionEndAt(t, store, scene.runID, first, scene.node, "agent_failed", 4)

	makeWorkflowRetryDue(t, store, scene.runID, "a")
	second := "exec-a-2-" + scene.runID[:8]
	scene.startAttempt(t, "a", second)
	scene.history(t, second, "turn-1", 1, 2)

	old := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1"})
	exec := old.O("execution")
	if exec.N("attempt") != 1 || exec.S("executionId") != first {
		t.Fatalf("attempt=1 read resolved attempt %d execution %q; want 1/%q", exec.N("attempt"), exec.S("executionId"), first)
	}
	if got := nodeSessionSequences(old); got != "1,2,3" {
		t.Fatalf("attempt=1 records = %s; want 1,2,3", got)
	}
	if got := exec.O("result").S("reason"); got != "agent_failed" {
		t.Fatalf("attempt=1 result = %v; want the first attempt's own reason", exec["result"])
	}
	if node := old.O("node"); node.N("attempt") != 2 || node.S("status") != "running" {
		t.Fatalf("node block = %v; want the current attempt 2 running", node)
	}

	// An absent attempt reads the node's current one, and only that one's execution.
	current := scene.mustRead(t, "a", nodeSessionQuery{})
	if exec := current.O("execution"); exec.N("attempt") != 2 || exec.S("executionId") != second {
		t.Fatalf("default read resolved attempt %d execution %q; want the current 2/%q", exec.N("attempt"), exec.S("executionId"), second)
	}
	if got := nodeSessionSequences(current); got != "1,2" {
		t.Fatalf("default read records = %s; want 1,2", got)
	}

	// A page cursor is resolved against the attempt the caller sends back, so a retry between two
	// pages cannot splice the new attempt's receipts onto the old one.
	paged := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1", before: "3"})
	if got := nodeSessionSequences(paged); got != "1,2" {
		t.Fatalf("attempt=1 before=3 records = %s; want 1,2", got)
	}
	if exec := paged.O("execution"); exec.B("hasMore") || exec["nextBefore"] != nil {
		t.Fatalf("attempt=1 before=3 page = %v; want the end of that attempt's history", exec)
	}

	scene.requireFault(t, "a", nodeSessionQuery{attempt: "3"}, "not_found")
	scene.requireFault(t, "a", nodeSessionQuery{attempt: "0"}, "invalid_pagination")
	// before cannot name an attempt by itself: resolving it against "the current attempt" would let
	// a retry change which attempt the next page reads.
	scene.requireFault(t, "a", nodeSessionQuery{before: "2"}, "invalid_pagination")
}

// The two blocks answer different questions. `execution` is the selected attempt's evidence; `node`
// is the node's current state, whose output and end reason belong to the current attempt alone.
func TestWorkflowNodeSessionReadNeverPresentsTheCurrentAttemptAsAnOldOne(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowRetryGraph(true, 2, 300))
	first := "exec-a-1-" + scene.runID[:8]
	scene.startAttempt(t, "a", first)
	scene.history(t, first, "turn-1", 1, 2)
	reportSessionEndAt(t, store, scene.runID, first, scene.node, "agent_failed", 3)

	makeWorkflowRetryDue(t, store, scene.runID, "a")
	second := "exec-a-2-" + scene.runID[:8]
	turn := scene.startAttempt(t, "a", second).O("initialTurn").S("turnId")
	takeOverWorkflowEvents(t, store, scene.runID, second, []Object{
		workflowAnswerEvent(1, turn, "Inspected the files"),
		threadEvent(2, turnEndedRecord("end_turn"), turn),
	})
	reportSessionEndAt(t, store, scene.runID, second, scene.node, "idle_timeout", 3)

	old := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1"})
	node := old.O("node")
	if node.N("attempt") != 2 || node.S("status") != "succeeded" {
		t.Fatalf("node block = %v; want the current attempt 2 succeeded", node)
	}
	if got := node.O("output").S("output"); got != "Inspected the files" {
		t.Fatalf("node.output = %v; want the current attempt's answer, never the old attempt's", node["output"])
	}
	if got := node.S("sessionEndReason"); got != "idle_timeout" {
		t.Fatalf("node.sessionEndReason = %q; want the current attempt's idle_timeout", got)
	}
	if got := old.O("execution").O("result").S("reason"); got != "agent_failed" {
		t.Fatalf("attempt=1 execution.result reason = %q; want the failed attempt's own", got)
	}

	// Only when the read selects the current attempt do the two blocks describe one attempt.
	current := scene.mustRead(t, "a", nodeSessionQuery{})
	if exec, node := current.O("execution"), current.O("node"); exec.N("attempt") != node.N("attempt") {
		t.Fatalf("default read execution attempt %d, node attempt %d; want the same", exec.N("attempt"), node.N("attempt"))
	}
}

// Every record is reachable: nextBefore is the earliest sequence the page covered, the next page
// takes sequence < nextBefore, and hasMore is an independent probe rather than "the page filled up".
func TestWorkflowNodeSessionReadPagesEveryRecordWithoutSkipping(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	scene.history(t, execution, "turn-1", 1, 5)

	var pages []string
	q := nodeSessionQuery{limit: 2}
	for {
		page := scene.mustRead(t, "a", q)
		exec := page.O("execution")
		pages = append(pages, nodeSessionSequences(page))
		records := nodeSessionRecords(page)
		if len(records) == 0 {
			t.Fatalf("page %d returned no record; the loop only stops at the end of the history", len(pages))
		}
		if exec.B("hasMore") && exec.N("nextBefore") != records[0].N("sequence") {
			t.Fatalf("page nextBefore = %d, earliest returned record = %d; the cursor must name a returned record",
				exec.N("nextBefore"), records[0].N("sequence"))
		}
		if !exec.B("hasMore") {
			if exec["nextBefore"] != nil {
				t.Fatalf("last page carries nextBefore %v without hasMore", exec["nextBefore"])
			}
			break
		}
		q = nodeSessionQuery{
			attempt: strconv.FormatInt(exec.N("attempt"), 10),
			before:  strconv.FormatInt(exec.N("nextBefore"), 10),
			limit:   2,
		}
	}
	if got := strings.Join(pages, " "); got != "4,5 2,3 1" {
		t.Fatalf("pages = %q; want 4,5 / 2,3 / 1 with no record skipped", got)
	}
}

// A page is bounded by its record count and by its byte budget, whichever comes first; the byte
// budget is what keeps a page of protocol-maximum receipts from becoming a 50 MiB response.
func TestWorkflowNodeSessionReadStopsAPageAtItsByteBudget(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	// Twenty receipts of 250 KiB are about 5 MiB, past the 4 MiB budget and far under the 200-record
	// limit, so only the byte budget can stop this page. They arrive in two contiguous batches
	// because one takeover carries at most 64 events.
	const total = 20
	text := strings.Repeat("x", 250*1024)
	for from := int64(1); from <= total; from += 10 {
		events := make([]Object, 0, 10)
		for seq := from; seq < from+10; seq++ {
			events = append(events, workflowAnswerEvent(seq, "turn-1", text))
		}
		takeOverWorkflowEvents(t, store, scene.runID, execution, events)
	}

	page := scene.mustRead(t, "a", nodeSessionQuery{limit: NodeSessionPageMax})
	exec := page.O("execution")
	records := nodeSessionRecords(page)
	if len(records) == 0 || len(records) >= total {
		t.Fatalf("page returned %d of %d records; want the byte budget to stop it early", len(records), total)
	}
	if !exec.B("hasMore") || exec.N("nextBefore") != records[0].N("sequence") {
		t.Fatalf("byte-bounded page = hasMore %v nextBefore %d; want more history behind %d",
			exec.B("hasMore"), exec.N("nextBefore"), records[0].N("sequence"))
	}
	if exec.B("incomplete") {
		t.Fatal("a byte-bounded page reported a history gap; the cursor stopped it, nothing was lost")
	}
	// Paging never rewrites a record body: the first record still carries the whole answer.
	if got := len(records[0].O("record").O("update").O("content").S("text")); got != len(text) {
		t.Fatalf("first record body is %d bytes; want the stored %d", got, len(text))
	}

	next := scene.mustRead(t, "a", nodeSessionQuery{
		attempt: "1", before: strconv.FormatInt(exec.N("nextBefore"), 10), limit: NodeSessionPageMax,
	})
	if got, want := len(nodeSessionRecords(next)), total-len(records); got != want {
		t.Fatalf("page after the byte-bounded page returned %d records; want the remaining %d", got, want)
	}
	if next.O("execution").B("incomplete") {
		t.Fatal("the second page reported a gap; the budget stopped the first page, nothing was lost")
	}
}

// The terminal receipt is excluded by its committed sequence, never by the shape of its content: a
// terminal event is an arbitrary non-empty string, so it may itself look like a history receipt.
func TestWorkflowNodeSessionReadExcludesTheTerminalReceiptBySequenceNotShape(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	scene.history(t, execution, "turn-1", 1, 2)
	shaped := jsonText(Object{
		"record":   jsonText(turnEndedRecord("end_turn")),
		"sequence": int64(3), "truncated": false, "turnId": "turn-1",
	})
	if _, err := store.transact(context.Background(), func(tx *transaction) Object {
		executionResult(tx, &ControlRequest{Body: Object{
			"operationId": scene.runID, "executionId": execution,
			"result":   Object{"outcome": "agent_session_ended", "reason": "idle_timeout", "node": Object{"nodeId": scene.node}},
			"sequence": int64(3), "event": shaped,
		}}, true)
		return Object{}
	}); err != nil {
		t.Fatalf("settle with a history-shaped terminal receipt: %v", err)
	}

	page := scene.mustRead(t, "a", nodeSessionQuery{})
	exec := page.O("execution")
	if got := nodeSessionSequences(page); got != "1,2" {
		t.Fatalf("records = %s; want 1,2 — the terminal receipt must not be projected", got)
	}
	if exec.B("incomplete") {
		t.Fatal("the terminal receipt was reported as a history gap")
	}
	if exec.B("hasMore") {
		t.Fatal("the terminal receipt produced another page")
	}
	if got := exec.S("state"); got != nodeSessionAvailable {
		t.Fatalf("state = %q; want available", got)
	}
}

// A gap is a per-page observation. The page that looks across the hole reports it; a page entirely
// above or entirely below the hole cannot see it and does not.
func TestWorkflowNodeSessionReadReportsAGapOnlyOnThePageThatObservesIt(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	// A lost receipt is not something the takeover path can produce (it refuses a gap), so the hole
	// is written directly: 1, 2 and then 4, 5 with 3 missing.
	for _, sequence := range []int64{1, 2, 4, 5} {
		insertRawReceipt(t, store, execution, sequence, rawHistoryReceipt(sequence))
	}

	for _, tc := range []struct {
		name       string
		query      nodeSessionQuery
		want       string
		incomplete bool
	}{
		{"above the hole", nodeSessionQuery{limit: 1}, "5", false},
		{"down to the hole's edge", nodeSessionQuery{attempt: "1", before: "5", limit: 1}, "4", false},
		{"across the hole", nodeSessionQuery{attempt: "1", before: "4", limit: 1}, "2", true},
		{"below the hole", nodeSessionQuery{attempt: "1", before: "2", limit: 1}, "1", false},
	} {
		page := scene.mustRead(t, "a", tc.query)
		if got := nodeSessionSequences(page); got != tc.want {
			t.Fatalf("%s: records = %s; want %s", tc.name, got, tc.want)
		}
		if got := page.O("execution").B("incomplete"); got != tc.incomplete {
			t.Fatalf("%s: incomplete = %v; want %v", tc.name, got, tc.incomplete)
		}
	}
}

// A receipt that is not the history receipt Cloud wrote is reported, not silently dropped — and a
// page made only of such rows still advances the cursor, so earlier history cannot be stranded
// behind a page that returned nothing.
func TestWorkflowNodeSessionReadReportsUnshapedHistoryWithoutStrandingTheRest(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	insertRawReceipt(t, store, execution, 1, rawHistoryReceipt(1))
	insertRawReceipt(t, store, execution, 2, "session-terminal-event")
	insertRawReceipt(t, store, execution, 3, rawHistoryReceipt(3))

	first := scene.mustRead(t, "a", nodeSessionQuery{limit: 1})
	if got := nodeSessionSequences(first); got != "3" {
		t.Fatalf("first page records = %s; want 3", got)
	}
	if first.O("execution").B("incomplete") {
		t.Fatal("a page above the unshaped row reported a gap")
	}

	second := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1", before: "3", limit: 1})
	exec := second.O("execution")
	if got := nodeSessionSequences(second); got != "" {
		t.Fatalf("page over the unshaped row returned %s; want no record", got)
	}
	if !exec.B("incomplete") {
		t.Fatal("the unshaped row was dropped without reporting a gap")
	}
	if !exec.B("hasMore") || exec.N("nextBefore") != 2 {
		t.Fatalf("page = hasMore %v nextBefore %d; want the cursor at 2 so receipt 1 stays reachable",
			exec.B("hasMore"), exec.N("nextBefore"))
	}

	third := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1", before: "2", limit: 1})
	if got := nodeSessionSequences(third); got != "1" {
		t.Fatalf("page below the unshaped row = %s; want 1", got)
	}
}

// An empty history never stands in for a fact, and a node the run has not reached is not an unknown
// resource: the frozen graph knows it, so its empty state reads as not_started.
func TestWorkflowNodeSessionReadDistinguishesNotApplicableAndNotStarted(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowTwoAgentGraph())

	pending := scene.mustRead(t, "a1", nodeSessionQuery{})
	if exec := pending.O("execution"); exec.S("state") != nodeSessionNotStarted || exec.N("attempt") != 1 {
		t.Fatalf("undispatched node = %v; want not_started on attempt 1", exec)
	}
	node := pending.O("node")
	if status, present := node["status"]; !present || status != nil {
		t.Fatalf("node block = %v; want a present null status for a node the run never reached", node)
	}

	// A node without a session at all is not an empty conversation.
	start := scene.mustRead(t, "s", nodeSessionQuery{})
	if got := start.O("execution").S("state"); got != nodeSessionNotApplicable {
		t.Fatalf("start node state = %q; want not_applicable", got)
	}
	if got := nodeSessionSequences(start); got != "" {
		t.Fatalf("start node records = %s; want none", got)
	}
}

// A dispatched session that has recorded nothing is in flight, and says so instead of claiming its
// history was lost.
func TestWorkflowNodeSessionReadReportsAnInFlightAttemptAwaitingItsFirstRecord(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	scene.startAttempt(t, "a", "exec-a-"+scene.runID[:8])

	page := scene.mustRead(t, "a", nodeSessionQuery{})
	exec := page.O("execution")
	if got := exec.S("state"); got != nodeSessionAwaitingFirstRecord {
		t.Fatalf("state = %q; want awaiting_first_record", got)
	}
	if got := nodeSessionSequences(page); got != "" {
		t.Fatalf("records = %s; want none", got)
	}
	if exec.B("incomplete") || exec.B("hasMore") {
		t.Fatalf("empty in-flight page = incomplete %v hasMore %v; want neither", exec.B("incomplete"), exec.B("hasMore"))
	}
}

// Cloud settles several terminal paths without writing a result. A null result is therefore not
// in-flight evidence: the node's own terminal state decides.
func TestWorkflowNodeSessionReadReportsACloudSettlementAsUnavailableNotInFlight(t *testing.T) {
	store := dispatcherDB(t)
	store.DeliveryUnreachableAfter = 30 * time.Minute
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	scene.startAttempt(t, "a", "exec-a-"+scene.runID[:8])
	// The Node stops reporting: its heartbeat goes stale past the window, and Cloud settles the
	// session itself instead of holding the run running forever.
	if _, err := store.Pool.Exec(`UPDATE node_instances SET last_seen_at=clock_timestamp()-interval '2 hours' WHERE workspace_id=$1`, scene.wsID); err != nil {
		t.Fatalf("age the Node heartbeat: %v", err)
	}
	if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
		t.Fatalf("advance after the Node went silent: %v", err)
	}
	var result any
	if err := store.Pool.QueryRow(`SELECT result FROM node_executions WHERE operation_id=$1 AND workflow_node_id='a' AND kind='agent_session'`, scene.runID).Scan(&result); err != nil {
		t.Fatalf("read the execution result: %v", err)
	}
	if result != nil {
		t.Fatalf("execution result = %v; this settlement must not fabricate one", result)
	}

	page := scene.mustRead(t, "a", nodeSessionQuery{})
	exec := page.O("execution")
	if got := exec.S("state"); got != nodeSessionUnavailable {
		t.Fatalf("state = %q; want unavailable — a null result is not in flight", got)
	}
	if exec["result"] != nil {
		t.Fatalf("result = %v; Cloud's own settlement has none", exec["result"])
	}
	node := page.O("node")
	if node.S("status") != "failed" || node.S("sessionEndReason") != workflowSessionNodeUnavailable || node.S("error") != workflowSessionNodeUnavailable {
		t.Fatalf("node block = %v; want failed under %s", node, workflowSessionNodeUnavailable)
	}
	if node["output"] != nil {
		t.Fatalf("node.output = %v; a failed node published no answer", node["output"])
	}
}

// Evidence that contradicts itself is reported as unknown, not resolved into a live or settled read:
// a retry happens only after its attempt settled with a result, so an earlier attempt without one is
// data no writer should have produced.
func TestWorkflowNodeSessionReadReportsContradictoryEvidenceAsUnknown(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	scene.startAttempt(t, "a", "exec-a-"+scene.runID[:8])
	if _, err := store.Pool.Exec(`UPDATE workflow_runs
		SET node_states = jsonb_set(node_states, ARRAY['a','attempt'], '2'::jsonb) WHERE id=$1`, scene.runID); err != nil {
		t.Fatalf("stage the contradictory attempt: %v", err)
	}

	// attempt 1 has an execution row and no result while the node has moved on to attempt 2.
	if got := scene.mustRead(t, "a", nodeSessionQuery{attempt: "1"}).O("execution").S("state"); got != nodeSessionUnknown {
		t.Fatalf("contradictory attempt state = %q; want unknown", got)
	}
	// attempt 2 is the node's committed attempt but nothing was ever dispatched for it.
	if got := scene.mustRead(t, "a", nodeSessionQuery{}).O("execution").S("state"); got != nodeSessionNotStarted {
		t.Fatalf("undispatched current attempt state = %q; want not_started", got)
	}
}

// Reading changes nothing: no state, execution or receipt moves, and no command or work item is
// released — not even by a page that faults.
func TestWorkflowNodeSessionReadIsReadOnly(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowSessionGraph())
	execution := "exec-a-" + scene.runID[:8]
	scene.startAttempt(t, "a", execution)
	scene.history(t, execution, "turn-1", 1, 4)

	snapshot := func() string {
		t.Helper()
		var states, executions, receipts string
		var commands, work int
		if err := store.Pool.QueryRow(`SELECT node_states::text FROM workflow_runs WHERE id=$1`, scene.runID).Scan(&states); err != nil {
			t.Fatalf("snapshot node_states: %v", err)
		}
		if err := store.Pool.QueryRow(`SELECT COALESCE(json_agg(e ORDER BY e.execution_id),'[]'::json)::text FROM node_executions e WHERE e.operation_id=$1`, scene.runID).Scan(&executions); err != nil {
			t.Fatalf("snapshot node_executions: %v", err)
		}
		if err := store.Pool.QueryRow(`SELECT COALESCE(json_agg(r ORDER BY r.sequence),'[]'::json)::text FROM node_event_receipts r WHERE r.execution_id=$1`, execution).Scan(&receipts); err != nil {
			t.Fatalf("snapshot node_event_receipts: %v", err)
		}
		if err := store.Pool.QueryRow(`SELECT (SELECT count(*) FROM thread_commands WHERE workflow_run_id=$1),
			(SELECT count(*) FROM execution_work WHERE workflow_run_id=$1)`, scene.runID).Scan(&commands, &work); err != nil {
			t.Fatalf("snapshot commands and work: %v", err)
		}
		return fmt.Sprintf("%s|%s|%s|%d|%d", states, executions, receipts, commands, work)
	}

	before := snapshot()
	// Two successful reads, two accepted-but-equal repeats and two faults.
	for _, q := range []nodeSessionQuery{
		{},
		{attempt: "1"},
		{limit: 2},
		{attempt: "1", before: "3", limit: 1},
		{attempt: "9"},
		{before: "3"},
	} {
		if _, status, err := scene.readNodeSession(t, "a", q); err == nil && status != 200 {
			t.Fatalf("GET %+v answered %d without an error", q, status)
		}
	}
	if after := snapshot(); after != before {
		t.Fatalf("reading changed durable state:\nbefore %s\nafter  %s", before, after)
	}
}

// Authorization stops at (tenant member, workflow, run, node). Neither a foreign node, a foreign
// tenant, a foreign caller, nor an execution id used as an address reaches another run's session.
func TestWorkflowNodeSessionReadStaysOnTheWorkflowRunNode(t *testing.T) {
	store := dispatcherDB(t)
	scene := newNodeSessionScene(t, store, workflowTwoAgentGraph())
	execution := "exec-a1-" + scene.runID[:8]
	scene.startAttempt(t, "a1", execution)
	scene.history(t, execution, "turn-1", 1, 2)

	// An active non-admin member reads; the read is not an admin surface.
	member := nodeSessionMember(t, store, scene.tid, "wf-session-reader")
	if out, status, err := store.Public(context.Background(), &PublicRequest{
		Method: "GET", Path: "/api/v1/tenants/" + scene.tid + "/workflows/" + scene.wid + "/runs/" + scene.runID + "/nodes/a1/session",
		TenantID: scene.tid, WorkflowID: scene.wid, RunID: scene.runID, NodeID: "a1", Identity: member,
	}); err != nil || status != 200 || nodeSessionSequences(out) != "1,2" {
		t.Fatalf("member read: status=%d out=%v err=%v; want the session", status, out, err)
	}

	// A caller outside the tenant is refused before any query runs.
	outsider, _ := modelUser(t, store, "wf-session-outsider")
	if _, _, err := store.Public(context.Background(), &PublicRequest{
		Method: "GET", Path: "/api/v1/tenants/" + scene.tid + "/workflows/" + scene.wid + "/runs/" + scene.runID + "/nodes/a1/session",
		TenantID: scene.tid, WorkflowID: scene.wid, RunID: scene.runID, NodeID: "a1", Identity: outsider,
	}); err == nil || ErrorCode(err).Code != "membership_required" {
		t.Fatalf("outsider read: err=%v; want membership_required", err)
	}

	// A node the frozen graph does not contain, and an execution id used as an address, both 404:
	// no route reads a session by anything but (run, node, attempt).
	scene.requireFault(t, "missing", nodeSessionQuery{}, "not_found")
	scene.requireFault(t, execution, nodeSessionQuery{}, "not_found")

	// The same node id under another tenant is a different resource, not this one.
	otherTenant := nodeSessionForeignTenant(t, store, scene.caller)
	if _, _, err := store.Public(context.Background(), &PublicRequest{
		Method: "GET", Path: "/api/v1/tenants/" + otherTenant + "/workflows/" + scene.wid + "/runs/" + scene.runID + "/nodes/a1/session",
		TenantID: otherTenant, WorkflowID: scene.wid, RunID: scene.runID, NodeID: "a1", Identity: scene.caller,
	}); err == nil || ErrorCode(err).Code != "not_found" {
		t.Fatalf("foreign-tenant read: err=%v; want not_found", err)
	}

	// A deleted workflow takes its runs' history with it.
	if _, err := store.Pool.Exec(`UPDATE workflows SET deleted_at=clock_timestamp() WHERE id=$1`, scene.wid); err != nil {
		t.Fatalf("delete the workflow: %v", err)
	}
	scene.requireFault(t, "a1", nodeSessionQuery{}, "not_found")
}

// nodeSessionMember binds one more active non-admin member of the tenant.
func nodeSessionMember(t *testing.T, store *Store, tid, subject string) *Claims {
	t.Helper()
	claims, uid := modelUser(t, store, subject)
	if _, err := store.Pool.Exec(`INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES($1,$2,'member','active')`, tid, uid); err != nil {
		t.Fatalf("add the member: %v", err)
	}
	return claims
}

// nodeSessionForeignTenant gives caller an active membership of a second tenant, so a read can name
// that tenant while asking for the first tenant's workflow.
func nodeSessionForeignTenant(t *testing.T, store *Store, caller *Claims) string {
	t.Helper()
	var uid string
	if err := store.Pool.QueryRow(`SELECT user_id FROM user_identities WHERE source=$1 AND subject=$2`, caller.Source, caller.Subject).Scan(&uid); err != nil {
		t.Fatalf("read the caller: %v", err)
	}
	tenantID, workflowID := uuid.NewString(), uuid.NewString()
	if _, err := store.transact(context.Background(), func(tx *transaction) Object {
		tx.exec(`INSERT INTO tenants(id,name,status) VALUES($1,'foreign','active')`, tenantID)
		tx.exec(`INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES($1,$2,'admin','active')`, tenantID, uid)
		tx.exec(`INSERT INTO workflows(id,tenant_id,name,graph) VALUES($1,$2,'foreign','{"nodes":[],"edges":[]}'::jsonb)`, workflowID, tenantID)
		return Object{}
	}); err != nil {
		t.Fatalf("stage the foreign tenant: %v", err)
	}
	return tenantID
}
