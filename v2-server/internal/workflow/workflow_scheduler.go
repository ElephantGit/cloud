// Adapted from cloud/internal/core/workflow_scheduler.go at 0d3299d5.
// Database and execution effects are provided by the v2 adapter.
package workflow

import (
	"time"
)

// This file retains Cloud's graph projection, state codecs and synchronous node
// evaluation. Machine owns serial advancement; Run owns the execution-port handshake.
// Cloud's SQL scanning, transactions, workspace provisioning and recovery are excluded.

// workflowBranchElse is the branch id a Condition node falls back to when no case matched, and the
// meaning of an outgoing edge with no sourceHandle (desktop branch_projection: an unlabeled edge
// defaults to else so graphs that predate source handles still route somewhere safe).
const workflowBranchElse = "else"

// asWorkflowObject accepts a JSON document as either the package's Object or a plain
// map[string]any. Object is a named type, so an Object boxed in `any` does not assert to
// map[string]any — and the scheduler reads documents from two worlds: jsonb-decoded rows produce
// plain maps, while hand-built documents (tests, callers passing literals) carry Object. Asserting
// one shape silently no-matches the other, so every document-level assertion goes through here.
func asWorkflowObject(v any) (Object, bool) {
	switch o := v.(type) {
	case Object:
		return o, true
	case map[string]any:
		return Object(o), true
	default:
		return nil, false
	}
}

// workflowNodeView is one node of the frozen snapshot graph the scheduler executes.
type workflowNodeView struct {
	ID   string
	Kind string
	Data Object
	// Container is the normalized region membership (data.containerId, else parentId). Decode
	// computes it once so every scheduler helper reads the same value.
	Container string
}

// workflowGraphView is the frozen graph reduced to the shapes the scheduler reads.
type workflowGraphView struct {
	Nodes  []workflowNodeView
	Edges  []Object
	ByNode map[string]workflowNodeView
	Start  string // the one start node's id, or "" when the graph has none
}

// readWorkflowGraph reduces a snapshot graph document into its nodes and edges. The document is
// data the scheduler interprets for the first time here, so malformed entries are dropped rather
// than exploding: a graph that reaches a run through a published snapshot was judged at publish,
// and snapshots predating that gate still get an honest run rather than a panic.
func readWorkflowGraph(graph Object) workflowGraphView {
	out := workflowGraphView{ByNode: map[string]workflowNodeView{}}
	// The comma-ok form keeps the promise above: a document whose nodes are not a list at all is an
	// empty graph, not a panic.
	nodes, _ := graph["nodes"].([]any)
	for _, raw := range nodes {
		o, ok := asWorkflowObject(raw)
		if !ok {
			continue
		}
		id := o.S("id")
		if id == "" {
			continue
		}
		if _, seen := out.ByNode[id]; seen {
			continue
		}
		view := workflowNodeView{ID: id, Kind: o.O("data").S("kind"), Data: o.O("data"), Container: workflowContainerID(o)}
		out.Nodes = append(out.Nodes, view)
		out.ByNode[id] = view
		// A loop body carries its own start. The run's entry point is the outer start only; picking a
		// body start here would make the projection root inside the container and never run the loop.
		if view.Kind == "start" && view.Container == "" && out.Start == "" {
			out.Start = id
		}
	}
	edges, _ := graph["edges"].([]any)
	for _, raw := range edges {
		e, ok := asWorkflowObject(raw)
		if !ok {
			continue
		}
		if e.S("source") == "" || e.S("target") == "" {
			continue
		}
		if _, sourceKnown := out.ByNode[e.S("source")]; !sourceKnown {
			continue
		}
		if _, targetKnown := out.ByNode[e.S("target")]; !targetKnown {
			continue
		}
		out.Edges = append(out.Edges, e)
	}
	return out
}

// workflowGraphCycle reports whether the graph contains a cycle, naming the source of the back edge the
// walk found — a node that is on a cycle by construction — or "" when the graph is a DAG.
//
// It is a depth-first walk with three colors: a node still on the current path that is reached again
// closes a cycle. The walk is deterministic — it roots at nodes in document order and follows edges in
// document order — so the node it names is too. Two cheaper-looking tests are wrong here and are worth
// naming so they are not reintroduced: peeling nodes with no incoming or no outgoing edge leaves the
// remainder including nodes that are merely *between* two cycles, and unwinding the stack at a back
// edge misses nodes on a cycle reached by a shortcut edge.
//
// Both the publish gate and the run's own advance read this one walk, so a graph Cloud refuses to
// publish and a graph Cloud refuses to run are the same graph.
func workflowGraphCycle(graph workflowGraphView) string {
	const (
		unvisited = 0
		onPath    = 1
		settled   = 2
	)
	state := map[string]int{}
	cycleSource := ""
	var walk func(id string)
	walk = func(id string) {
		state[id] = onPath
		for _, edge := range graph.Edges {
			if edge.S("source") != id {
				continue
			}
			next := edge.S("target")
			switch state[next] {
			case onPath:
				// A back edge: `next` is an ancestor on the current path, so the path from `next`
				// back to `id` plus this edge is a cycle. The first one found wins.
				if cycleSource == "" {
					cycleSource = id
				}
			case unvisited:
				walk(next)
			}
		}
		state[id] = settled
	}
	for _, node := range graph.Nodes {
		if state[node.ID] == unvisited {
			walk(node.ID)
		}
	}
	return cycleSource
}

// workflowNodeState is one node's committed execution state on the run row. The shape is the one the
// run viewer already renders (the simulator writes the same keys): status is the shared run/node
// vocabulary, output carries what the node produced, and an inactive state marks a node behind a
// branch that was never taken — it never ran and never will.
type workflowNodeState struct {
	Status      string
	DisplayName string
	StartedAt   string
	FinishedAt  string
	// Attempt is the 1-based attempt this state describes (retry decision D4). It is committed on
	// every agent-node state — running, retrying and terminal — because the scheduler reads the
	// budget and the evidence scope back from it after a restart, a replica change or a replay; the
	// attempt counter is not kept anywhere else. Non-agent nodes have no attempts and leave it zero.
	Attempt int64
	// RetryAt is the earliest instant at which the next attempt may start (retry decision D3). Only
	// a `retrying` node carries it, and it is the database clock's instant, so every pass judges the
	// same wake-up. A terminal or running state has none.
	RetryAt string
	// Error is the node's failure reason, and only a failed node has one: a cancelled node was
	// cancelled, not broken, and an idle or succeeded one has nothing to report. The run viewer
	// renders it in its own Error section, which is why it is a field of the state rather than
	// something the viewer has to dig out of Output — a node's Output is what it produced, and a
	// failed node produced nothing. A `retrying` node keeps the failed attempt's reason: the retry
	// replaces a failure, it does not erase it.
	Error  string
	Output Object
	// SessionEndReason is execution evidence, never a declared variable root.
	SessionEndReason string
	// Loop is the round progress of a loop node and nil for every other kind. The active round's
	// member states stay in the run's flat node_states map (so the session, retry and viewer paths see
	// one shape); completed rounds are archived here for audit and recovery.
	Loop *workflowLoopRunState
	// Iteration is the round progress and ledger of an iteration node, nil for every other kind.
	Iteration *workflowIterationRunState
}

// attemptNumber is the state's 1-based attempt, reading an absent attempt as the first one. Only
// agent nodes carry attempts; a state written before attempts existed, or by a non-agent node,
// belongs to attempt 1.
func (s *workflowNodeState) attemptNumber() int64 {
	if s.Attempt < 1 {
		return 1
	}
	return s.Attempt
}

// object renders the state in the shape the run viewer reads (and the simulator writes). The receiver
// is a pointer because the state is large enough that copying it per render is waste, and its one
// caller already holds it in a range variable.
func (s *workflowNodeState) object() Object {
	out := Object{"status": s.Status, "displayName": s.DisplayName, "startedAt": s.StartedAt, "finishedAt": s.FinishedAt}
	if s.Attempt >= 1 {
		out["attempt"] = s.Attempt
	}
	if s.RetryAt != "" {
		out["retryAt"] = s.RetryAt
	}
	if s.Error != "" {
		out["error"] = s.Error
	}
	if s.Output != nil {
		out["output"] = s.Output
	}
	if s.SessionEndReason != "" {
		out["sessionEndReason"] = s.SessionEndReason
	}
	if s.Loop != nil {
		out["loop"] = s.Loop.object()
	}
	if s.Iteration != nil {
		out["iteration"] = s.Iteration.object()
	}
	return out
}

// workflowStatesFromJSON decodes the committed node_states column. A row the column cannot interpret
// contributes nothing rather than failing the pass: the states are re-derived from whatever is
// readable, and a truncated state only costs a re-execution of the affected node, never a wrong
// success — a node without a committed result is never recorded as succeeded.
func workflowStatesFromJSON(raw Object) map[string]workflowNodeState {
	out := map[string]workflowNodeState{}
	for id, value := range raw {
		node, ok := asWorkflowObject(value)
		if !ok {
			continue
		}
		state := workflowNodeState{
			Status:           node.S("status"),
			DisplayName:      node.S("displayName"),
			StartedAt:        node.S("startedAt"),
			FinishedAt:       node.S("finishedAt"),
			Attempt:          node.N("attempt"),
			RetryAt:          node.S("retryAt"),
			Error:            node.S("error"),
			SessionEndReason: node.S("sessionEndReason"),
		}
		if output, ok := asWorkflowObject(node["output"]); ok {
			state.Output = output
		}
		if loop, ok := asWorkflowObject(node["loop"]); ok {
			state.Loop = workflowLoopRunStateFromJSON(loop)
		}
		if iteration, ok := asWorkflowObject(node["iteration"]); ok {
			state.Iteration = workflowIterationRunStateFromJSON(iteration)
		}
		out[id] = state
	}
	return out
}

// workflowProjection is one advance pass's view of the run: the frozen graph, the node states
// committed before the pass, and the Ready/Inactive resolution derived from them. It is the Go
// counterpart of desktop's BranchProjection (branch_projection.rs), restricted to the phase-1
// node kinds — no regions, no iterations, no loops: a run's graph is a DAG.
type workflowProjection struct {
	graph     workflowGraphView
	committed map[string]workflowNodeState
	projected map[string]string
}

// projectWorkflowNodes resolves the projection to fixed point over the committed states:
// Ready/Inactive propagate downstream through each pass until nothing changes, exactly like
// desktop's resolve_projected_nodes.
func projectWorkflowNodes(graph workflowGraphView, committed map[string]workflowNodeState) *workflowProjection {
	p := &workflowProjection{
		graph:     graph,
		committed: committed,
		projected: map[string]string{},
	}
	for id := range committed {
		state := committed[id]
		p.projected[id] = state.Status
	}
	for {
		changed := false
		for _, node := range graph.Nodes {
			if _, known := p.projected[node.ID]; known {
				continue
			}
			computed, resolved := p.nodeState(node.ID)
			if !resolved {
				continue
			}
			p.projected[node.ID] = computed
			changed = true
		}
		if !changed {
			break
		}
	}
	return p
}

// readyNode returns the next ready node in document order, or ok=false when the projection has no
// executable node left. This is the scheduler's ready set: recomputed from committed state only
// (D1 条件④), never from anything the pass remembers across transactions.
func (p *workflowProjection) readyNode() (workflowNodeView, bool) {
	// An exit owns the next serial slot before any sibling can start an external effect.
	for _, node := range p.graph.Nodes {
		if node.Kind == "loopExit" && p.projected[node.ID] == "ready" {
			return node, true
		}
	}
	for _, node := range p.graph.Nodes {
		if p.projected[node.ID] == "ready" {
			return node, true
		}
	}
	return workflowNodeView{}, false
}

// hasPending reports whether the projection left a node unresolved because an upstream edge is still
// pending. A scope whose nodes are all resolved and none ready has drained: for a loop body that is
// the moment the round is judged complete.
func (p *workflowProjection) hasPending() bool {
	for _, node := range p.graph.Nodes {
		if _, ok := p.projected[node.ID]; !ok {
			return true
		}
	}
	return false
}

// hasFailure reports whether any committed node failed.
func (p *workflowProjection) hasFailure() bool {
	// Only nodes of this projection can decide it. A body member's failure belongs to its container:
	// the container engine either absorbs it (iteration `continue`) or fails itself, and only the
	// container's own state may stop the outer wave. Reading the whole committed map here let an
	// absorbed failure escape its region and fail the run.
	for _, node := range p.graph.Nodes {
		if state, ok := p.committed[node.ID]; ok && state.Status == "failed" {
			return true
		}
	}
	return false
}

// nodeState derives one uncommitted node's projected state from its incoming edges:
// no incoming edges is ready only for the start node (a rootless non-start node never runs); any
// pending edge leaves the node unreached (not resolved); any failed edge fails it; any active edge
// readies it; all-inactive marks it inactive (behind a branch that was not taken).
func (p *workflowProjection) nodeState(nodeID string) (string, bool) {
	incoming := p.incomingEdges(nodeID)
	if len(incoming) == 0 {
		// A graph without a declared start begins at every entry node. Iteration regions have no
		// start member: their entries are the members with no predecessor inside the region, and
		// this rule is what lets a round begin there. An outer graph always declares exactly one
		// start (the gate requires it), so its stray roots stay inactive.
		if nodeID == p.graph.Start || p.graph.Start == "" {
			return "ready", true
		}
		return "inactive", true
	}
	anyActive, anyFailed, anyPending := false, false, false
	for _, edge := range incoming {
		switch p.edgeState(edge) {
		case "pending":
			anyPending = true
		case "failed":
			anyFailed = true
		case "active":
			anyActive = true
		}
	}
	if anyPending {
		return "", false // an upstream node is still unresolved: wait
	}
	if anyFailed {
		return "failed", true
	}
	if anyActive {
		return "ready", true
	}
	return "inactive", true
}

// edgeState resolves one edge from its source node's projected state and, for a Condition, the
// branch it committed. A succeeded non-condition node activates all of its outgoing edges; a
// succeeded Condition activates only the edge whose sourceHandle is the committed branch (an edge
// with no handle is the else edge); a failed source fails the edge; an inactive source stays
// inactive.
func (p *workflowProjection) edgeState(edge Object) string {
	switch p.projected[edge.S("source")] {
	case "succeeded":
		source := p.graph.ByNode[edge.S("source")]
		if source.Kind != "condition" {
			return "active"
		}
		selected := p.selectedBranch(source)
		handle := edge.S("sourceHandle")
		if handle == "" {
			handle = workflowBranchElse
		}
		if handle == selected {
			return "active"
		}
		return "inactive"
	case "failed":
		return "failed"
	case "inactive", "cancelled":
		return "inactive"
	default:
		return "pending"
	}
}

// selectedBranch reads the branch a committed Condition selected from its committed state. A
// condition whose state carries no selection is read as else: the safe default routes downstream
// somewhere rather than dead-ending the branch, and a succeeded condition always committed one.
func (p *workflowProjection) selectedBranch(source workflowNodeView) string {
	if state, ok := p.committed[source.ID]; ok {
		if branch := state.Output.S("selectedBranch"); branch != "" {
			return branch
		}
	}
	return workflowBranchElse
}

// incomingEdges returns the edges whose target is nodeID, in document order.
func (p *workflowProjection) incomingEdges(nodeID string) []Object {
	out := []Object{}
	for _, edge := range p.graph.Edges {
		if edge.S("target") == nodeID {
			out = append(out, edge)
		}
	}
	return out
}

// workflowInactiveStates adds a committed inactive state for every node the projection resolved as
// inactive but that carries no committed state. Behind a branch that was not taken, such a node
// never ran and never will; persisting it keeps the run row alone telling the viewer the full story.
func workflowInactiveStates(graph workflowGraphView, states map[string]workflowNodeState) map[string]workflowNodeState {
	p := projectWorkflowNodes(graph, states)
	out := states
	for _, node := range graph.Nodes {
		if p.projected[node.ID] == "inactive" {
			if _, committed := states[node.ID]; !committed {
				out[node.ID] = workflowNodeState{Status: "inactive", DisplayName: workflowNodeDisplayName(node)}
			}
		}
	}
	return out
}

// executeWorkflowNode commits one ready non-agent node's result from committed state. start reports
// the variable roots desktop's Start node exposes; condition evaluates its cases against prior node
// outputs overlaid with the run's ambient variables (sys.* and frozen user globals) and commits the
// selected branch (first match wins, else when nothing matched, and a failure when a rule cannot be
// evaluated); output resolves explicit named bindings into its committed result. No bindings retain
// an empty result; an unassigned binding fails instead of silently emitting a partial object.
func executeWorkflowNode(node workflowNodeView, input Object, states, ambient map[string]workflowNodeState) workflowNodeState {
	now := nowRFC3339()
	state := workflowNodeState{
		Status:      "succeeded",
		DisplayName: workflowNodeDisplayName(node),
		StartedAt:   now,
		FinishedAt:  now,
	}
	switch node.Kind {
	case "start":
		state.Output = workflowStartOutput(node, input)
	case "condition":
		branch, err := evaluateWorkflowCondition(node, workflowConditionStates(states, ambient))
		if err != nil {
			// A condition that cannot evaluate the document it was given fails the node and the run,
			// exactly as desktop's ConditionRuntime does. Falling through to else would hide a
			// malformed or unassigned-variable reference behind a branch the author never chose.
			state.Status = "failed"
			state.Error = err.Error()
			state.Output = Object{"reason": err.Error()}
			return state
		}
		state.Output = Object{"selectedBranch": branch}
	case "output":
		output, failure := evaluateWorkflowOutput(node, workflowConditionStates(states, ambient))
		if failure != nil {
			state.Status = "failed"
			state.Error = failure.Error()
			state.Output = Object{"reason": failure.Reason}
			return state
		}
		state.Output = output
	case "aggregator":
		value, assigned, err := evaluateWorkflowAggregator(node, workflowConditionStates(states, ambient))
		if err != nil {
			state.Status = "failed"
			state.Error = err.Error()
			state.Output = Object{"reason": "aggregator_invalid_config"}
			return state
		}
		if !assigned {
			// Every candidate was declared but none was produced: the stable desktop failure, not an
			// empty output. Publishing nothing would make a branch merge silently disappear.
			state.Status = "failed"
			state.Error = "variable aggregator found no assigned candidate"
			state.Output = Object{"reason": "aggregator_no_match"}
			return state
		}
		state.Output = Object{"output": value}
	case "loopExit":
		// The early-exit marker carries no configuration and produces nothing; the container engine
		// reads its success to skip feedback and publish the round's outputs.
		state.Output = Object{}
	default:
		// Unreachable: the ready pass only yields executable kinds, and the agent case returned
		// earlier. A future kind that reaches here must not be recorded as success.
		state.Status = "failed"
		state.Error = "unsupported_node_kind"
		state.Output = Object{"reason": "unsupported_node_kind"}
	}
	return state
}

// workflowAmbientStates builds the pseudo-node states a condition reads but no graph node commits:
// the runtime-owned sys.* pair and the user globals frozen in the snapshot. The keys are writers (a
// global's first name segment, or "sys"), so a selector addresses them exactly as it addresses a node.
// sys.workflow_id is the run's workflow id and sys.timestamp is the run's creation time in epoch
// milliseconds, both read from the frozen run row so every pass — retry, recovery, re-dispatch —
// agrees. User-authored sys.* declarations are skipped rather than merged: the runtime owns those two
// names, which is desktop's retain-and-repush behavior.
func workflowAmbientStates(run, graph Object) map[string]workflowNodeState {
	ambient := map[string]workflowNodeState{}
	for _, variable := range workflowGlobalVariables(graph) {
		name := variable.S("name")
		if name == "sys.workflow_id" || name == "sys.timestamp" {
			continue
		}
		writer, root, ok := workflowGlobalSelector(name)
		if !ok {
			continue
		}
		value, present := variable["value"]
		if !present || value == nil {
			continue
		}
		setWorkflowAmbientRoot(ambient, writer, root, value)
	}
	ambient["sys"] = workflowNodeState{
		Status: "succeeded",
		Output: Object{
			"workflow_id": run.S("workflowId"),
			"timestamp":   float64(run.N("createdAtMs")),
		},
	}
	return ambient
}

// setWorkflowAmbientRoot records one global's value under its writer's pseudo-node output, creating
// the pseudo-node on first use. Two globals in one namespace share the writer and union their roots.
func setWorkflowAmbientRoot(ambient map[string]workflowNodeState, writer, root string, value any) {
	state, ok := ambient[writer]
	if !ok {
		state = workflowNodeState{Status: "succeeded", Output: Object{}}
	}
	state.Output[root] = value
	ambient[writer] = state
}

// workflowConditionStates returns the states a condition evaluates against: the committed node states
// overlaid with the run's ambient variables. The overlay is per-root rather than per-node because a
// global's writer is a namespace, not a node — a graph may declare a global `start.foo` beside a real
// start node, and desktop's flat pool keeps `start.foo` (global) and `start.input` (node) as distinct
// keys. Committed roots win on a collision, the same way desktop's node pass overwrites a same-named
// global value. The result is a fresh map so the ambient pseudo-nodes are never persisted into
// node_states.
func workflowConditionStates(states, ambient map[string]workflowNodeState) map[string]workflowNodeState {
	if len(ambient) == 0 {
		return states
	}
	out := make(map[string]workflowNodeState, len(states)+len(ambient))
	for id := range ambient {
		out[id] = ambient[id]
	}
	for id := range states {
		state := states[id]
		if existing, ok := out[id]; ok {
			existing.Output = mergeWorkflowOutput(existing.Output, state.Output)
			out[id] = existing
			continue
		}
		out[id] = state
	}
	return out
}

// mergeWorkflowOutput merges an ambient pseudo-node's output with a committed node's, letting the
// committed roots win. The ambient output is nil when no global shares the node's id.
func mergeWorkflowOutput(ambient, committed Object) Object {
	merged := make(Object, len(ambient)+len(committed))
	for key, value := range ambient {
		merged[key] = value
	}
	for key, value := range committed {
		merged[key] = value
	}
	return merged
}

// workflowStartOutput is the Start node's variable pool in the shape desktop declares it: `input` is
// the kickoff prompt (the reserved free-text selector), and every declared input variable is a root
// named by the variable itself. Before circus#210 Cloud nested the whole run input under `input`, so
// `[start, name]` resolved to nothing and `[start, input]` answered for the entire form — the same
// document meant different things on the two sides.
//
// The run form's values are keyed by variable name, so flattening them is exact. The `input` key is
// not flattened with the rest: it is the reserved kickoff selector, and letting a user variable
// shadow it would make the same selector mean a run value on Cloud and a prompt on desktop. Its value
// comes from WorkflowStartKickoff instead.
func workflowStartOutput(node workflowNodeView, input Object) Object {
	output := make(Object, len(input)+1)
	for key, value := range input {
		if key == "input" {
			continue
		}
		output[key] = value
	}
	if kickoff, ok := WorkflowStartKickoff(node.Data, input); ok {
		output["input"] = kickoff
	}
	return output
}

// WorkflowStartKickoff resolves the Start node's kickoff text and reports whether it exists. The
// run's own submitted `input` wins; the authored Start instruction is the fallback.
//
// The two sources exist because runs outlive the code that created them. A run created before Cloud
// persisted the per-run kickoff has no reserved `input` key in its own input object — the prompt it
// executed then lived in the frozen snapshot's Start node, not the run row — so falling back to that
// node's `data.input` keeps historical runs evaluating exactly as they did (circus#287). The fallback
// reads the snapshot's node, never the live workflow document. Cloud's canonical document stores the
// prompt under `data.input` only; the legacy `data.instruction` desktop also reads is migrated to
// `input` by the frontend when a document is read, so the server never has to fall back to it.
//
// The text is used verbatim, whitespace included: desktop's start.input is the kickoff prompt as the
// author or the run submitted it, and trimming here would make the same selector compare differently
// on the two sides. When neither source has a value the root is left out of the pool rather than set
// to "": desktop leaves {start}.input unset when the run supplied no kickoff and the graph has no
// instruction, and exists/not_exists must answer the same way on both sides.
//
// It is exported for the development simulator, which writes the Start output outside this package;
// sharing the resolver keeps the simulated trace from drifting from what the scheduler commits.
func WorkflowStartKickoff(data, input Object) (string, bool) {
	if v, ok := input["input"]; ok && v != nil {
		if text, ok := v.(string); ok {
			return text, true
		}
	}
	if v, ok := data["input"]; ok && v != nil {
		if text, ok := v.(string); ok {
			return text, true
		}
	}
	return "", false
}

// workflowNodeDisplayName is the label the run viewer shows: the author's title, or the node kind
// when the title is empty — the same projection the simulator writes.
func workflowNodeDisplayName(node workflowNodeView) string {
	if title := node.Data.S("title"); title != "" {
		return title
	}
	return node.Kind
}

// workflowRunFailureReason names the first committed failure in node order, so the run's error
// points at the node that actually failed rather than at a downstream casualty. The order has to
// come from the graph: the states are a map, Go randomizes map iteration, and reading the reason
// out of it would let the same trace record a different reason on every replay. Every committed
// state belongs to a graph node — the scheduler writes them from graph nodes, and the graph is
// frozen at creation — so walking the graph reaches them all.
func workflowRunFailureReason(graph workflowGraphView, states map[string]workflowNodeState) string {
	for _, node := range graph.Nodes {
		state := states[node.ID]
		if state.Status != "failed" {
			continue
		}
		if node.Kind == "agent" && state.Error != "" {
			return state.Error
		}
		if reason := state.Output.S("reason"); reason != "" {
			return reason
		}
		return "node failed"
	}
	return "run failed"
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
