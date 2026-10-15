package core

import (
	"regexp"
	"strings"
)

// Canonical workflow graph validation: the server-executable subset of the graph schema specs owns.
//
// The editor owns the document and specs owns the canonical schema; Cloud owns exactly the subset it
// can be asked to execute, and this file is that subset written down once. The publish gate, the run
// admission gate and the @ form's schema projection all read it, so a graph Cloud publishes is a
// graph Cloud has agreed to run, and the reason a graph is refused names the node or slot at fault
// instead of silently ignoring the part Cloud does not understand.
//
// Only publishing validates the graph. Saving is deliberately unchecked: an unfinished draft is a
// legitimate document — the author is mid-edit and the workflow is not runnable yet — so create and
// update accept any JSON object and only publish asks whether the document is complete. That split is
// the whole point of the draft/published distinction, and moving an install or completeness check
// into the save path is the mistake this file exists to prevent.

// workflowExecutableNodeKinds is the subset of the editor's node kinds Cloud executes in its first
// phase (#67 结论 4): start, agent, condition and output. Everything else stays editable and
// saveable, but publishing refuses it rather than freezing a graph whose nodes would be dropped.
var workflowExecutableNodeKinds = map[string]bool{
	"start":     true,
	"agent":     true,
	"condition": true,
	"output":    true,
}

// workflowDefaultSlot is the executor slot every graph may reference without declaring it, and the
// slot an agent node uses when it names none (specs#98 D2). It is reserved: a graph may not declare
// it, because declaring it would let an author redefine the one name every runner already has.
const workflowDefaultSlot = "default"

// workflowSlotNamePattern is the grammar of an executor slot name (specs#98 D1). The name is the key
// a runner's personal mapping is looked up by, so it stays a stable, easily compared identifier: a
// name with case or whitespace variants would look configured and still miss.
var workflowSlotNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Bounds on the author-written slot description, which is the only thing a runner sees when deciding
// what to map the slot to (specs#98 D1).
const (
	minSlotDescription = 1
	maxSlotDescription = 500
)

// workflowGraphProblem is one canonical-schema violation: a stable code plus the safe parameters that
// say where it is. It carries no prose because the wire contract is codes and parameters — a client
// localizes them, and a message written here would be a second, untranslated source of truth.
type workflowGraphProblem struct {
	Code   string
	NodeID string
	Slot   string
	Kind   string
}

// Object renders one problem for the fault's `problems` list, omitting the fields that do not apply.
func (p workflowGraphProblem) Object() Object {
	out := Object{"code": p.Code}
	if p.NodeID != "" {
		out["nodeId"] = p.NodeID
	}
	if p.Slot != "" {
		out["slot"] = p.Slot
	}
	if p.Kind != "" {
		out["kind"] = p.Kind
	}
	return out
}

// workflowNodes reads the graph's node list. The graph is a document the API never interprets, so a
// document whose nodes are not a list contributes no nodes rather than failing the read; publishing is
// what turns that into a problem.
func workflowNodes(graph Object) []any {
	nodes, _ := graph["nodes"].([]any)
	return nodes
}

// validateWorkflowGraph reports every way a graph violates the canonical schema, in document order so
// a client can render them next to the nodes they name. It is pure and total: a graph it cannot
// interpret produces problems rather than an error, because a malformed document is exactly what it
// is here to describe.
func validateWorkflowGraph(graph Object) []workflowGraphProblem {
	problems := []workflowGraphProblem{}
	nodes, ok := graph["nodes"].([]any)
	if !ok {
		problems = append(problems, workflowGraphProblem{Code: "nodes_not_a_list"})
		nodes = nil
	}
	if _, ok := graph["edges"].([]any); !ok {
		problems = append(problems, workflowGraphProblem{Code: "edges_not_a_list"})
	}
	problems = append(problems, validateWorkflowNodes(nodes)...)
	problems = append(problems, validateWorkflowEdges(graph, nodes)...)
	problems = append(problems, validateWorkflowSlots(graph, nodes)...)
	return problems
}

// validateWorkflowNodes checks node identity, kind and the start-node count. The editor allows at
// most one Start node, and a run needs exactly one entry point, so zero and two are both refused: a
// graph with none is unfinished, and one with two has no defined place to begin.
func validateWorkflowNodes(nodes []any) []workflowGraphProblem {
	problems := []workflowGraphProblem{}
	seen := map[string]bool{}
	starts := 0
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			problems = append(problems, workflowGraphProblem{Code: "node_not_an_object"})
			continue
		}
		o := Object(node)
		id := o.S("id")
		if id == "" {
			problems = append(problems, workflowGraphProblem{Code: "node_id_missing"})
			continue
		}
		if seen[id] {
			problems = append(problems, workflowGraphProblem{Code: "node_id_duplicate", NodeID: id})
			continue
		}
		seen[id] = true
		kind := o.O("data").S("kind")
		switch {
		case kind == "":
			problems = append(problems, workflowGraphProblem{Code: "node_kind_missing", NodeID: id})
		case !workflowExecutableNodeKinds[kind]:
			problems = append(problems, workflowGraphProblem{Code: "node_kind_unsupported", NodeID: id, Kind: kind})
		case kind == "start":
			starts++
		}
	}
	if starts != 1 {
		problems = append(problems, workflowGraphProblem{Code: "start_node_count"})
	}
	return problems
}

// validateWorkflowEdges checks that every edge joins two nodes that exist. An edge to a node the
// document does not contain is a dangling reference the run would have to guess at, and the editor
// only produces one when the document was edited outside it.
func validateWorkflowEdges(graph Object, nodes []any) []workflowGraphProblem {
	problems := []workflowGraphProblem{}
	edges, _ := graph["edges"].([]any)
	known := map[string]bool{}
	for _, raw := range nodes {
		if node, ok := raw.(map[string]any); ok {
			if id := Object(node).S("id"); id != "" {
				known[id] = true
			}
		}
	}
	for _, raw := range edges {
		edge, ok := raw.(map[string]any)
		if !ok {
			problems = append(problems, workflowGraphProblem{Code: "edge_not_an_object"})
			continue
		}
		o := Object(edge)
		for _, endpoint := range []string{o.S("source"), o.S("target")} {
			if endpoint == "" || !known[endpoint] {
				problems = append(problems, workflowGraphProblem{Code: "edge_endpoint_unknown", NodeID: endpoint})
			}
		}
	}
	return problems
}

// validateWorkflowSlots checks the graph's executor-slot declarations and the agent nodes' references
// to them (specs#98 D1/D2). A reference to an undeclared slot is refused here rather than at run
// time: the author is the only one who can say what the slot means, and a run created against an
// undeclared slot could never resolve an executor for it.
//
// A declared slot no node references is *not* a problem: an author may be mid-edit or keeping one for
// later, and refusing it would block publishing for a mistake with no runtime consequence.
func validateWorkflowSlots(graph Object, nodes []any) []workflowGraphProblem {
	problems := []workflowGraphProblem{}
	declared := map[string]bool{}
	for _, slot := range workflowSlotDeclarations(graph) {
		name := slot.S("name")
		switch {
		case name == "":
			problems = append(problems, workflowGraphProblem{Code: "slot_name_missing"})
		case !workflowSlotNamePattern.MatchString(name):
			problems = append(problems, workflowGraphProblem{Code: "slot_name_invalid", Slot: name})
		case name == workflowDefaultSlot:
			problems = append(problems, workflowGraphProblem{Code: "slot_name_reserved", Slot: name})
		case declared[name]:
			problems = append(problems, workflowGraphProblem{Code: "slot_name_duplicate", Slot: name})
		}
		if name != "" {
			declared[name] = true
		}
		description := strings.TrimSpace(slot.S("description"))
		if len(description) < minSlotDescription || len(description) > maxSlotDescription {
			problems = append(problems, workflowGraphProblem{Code: "slot_description_invalid", Slot: name})
		}
		if hint, ok := slot["hint"]; ok && hint != nil && !validSlotHint(hint) {
			problems = append(problems, workflowGraphProblem{Code: "slot_hint_invalid", Slot: name})
		}
	}
	for _, reference := range workflowReferencedSlots(nodes) {
		if reference.Name == workflowDefaultSlot || declared[reference.Name] {
			continue
		}
		for _, nodeID := range reference.Nodes {
			problems = append(problems, workflowGraphProblem{Code: "slot_not_declared", NodeID: nodeID, Slot: reference.Name})
		}
	}
	return problems
}

// validSlotHint reports whether a hint is the shape specs#98 D3 declares: an object whose `agentCli`
// and `modelId`, when present, are non-empty strings. A hint never resolves an executor, so it is
// checked only far enough to keep the document well formed for the runner that reads it.
func validSlotHint(hint any) bool {
	o, ok := hint.(map[string]any)
	if !ok {
		return false
	}
	h := Object(o)
	for _, key := range []string{"agentCli", "modelId"} {
		value, present := h[key]
		if !present || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok || text == "" || len(text) > 200 {
			return false
		}
	}
	return true
}

// workflowSlotDeclarations reads the graph's executor-slot declarations. The graph is a document the
// API never interprets, so an entry that is not an object contributes nothing rather than failing the
// request; publishing is what turns the rest into problems.
func workflowSlotDeclarations(graph Object) []Object {
	declared, _ := graph["executorSlots"].([]any)
	out := []Object{}
	for _, item := range declared {
		if slot, ok := item.(map[string]any); ok {
			out = append(out, Object(slot))
		}
	}
	return out
}

// workflowSlotReference is one executor slot an agent node asks for, together with the nodes that ask
// for it. Both travel together because every diagnostic and every runner-facing message has to name
// the slot *and* the node: "strong is not configured" is only actionable once the author knows which
// step depends on it.
type workflowSlotReference struct {
	Name  string
	Nodes []string
}

// workflowReferencedSlots collects the executor slots a graph's agent nodes reference, in the order
// the nodes appear so a message lists them the way the canvas draws them. An agent node that names no
// slot references `default` (specs#98 D2), which is also how a node still carrying the pre-slot
// `agentConfig.executor` field is read: cloud does not migrate its stored documents, so an old graph
// keeps running as the one-slot graph it always was (specs#98 D5).
//
// The node list is a slice because a node id may be repeated in a malformed document, and an entry
// with no id is dropped: an unnamed node cannot be pointed at in a message.
func workflowReferencedSlots(nodes []any) []workflowSlotReference {
	index := map[string]int{}
	out := []workflowSlotReference{}
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		o := Object(node)
		data := o.O("data")
		if data.S("kind") != "agent" {
			continue
		}
		id := o.S("id")
		if id == "" {
			continue
		}
		name := data.O("agentConfig").S("executorSlot")
		if name == "" {
			name = workflowDefaultSlot
		}
		if i, ok := index[name]; ok {
			out[i].Nodes = append(out[i].Nodes, id)
			continue
		}
		index[name] = len(out)
		out = append(out, workflowSlotReference{Name: name, Nodes: []string{id}})
	}
	return out
}

// requireValidWorkflowGraph refuses a publish whose graph violates the canonical schema. The
// violations travel as parameters so the editor can mark each node, and the fault stays one stable
// code so a client cannot accidentally treat one kind of violation as another.
func requireValidWorkflowGraph(graph Object) {
	problems := validateWorkflowGraph(graph)
	if len(problems) == 0 {
		return
	}
	list := make([]any, 0, len(problems))
	for _, problem := range problems {
		list = append(list, problem.Object())
	}
	rejectWith(400, "workflow_graph_invalid", Object{"problems": list})
}
