package core

import (
	"reflect"
	"strings"
	"testing"
)

// faultOf runs fn and returns the Fault it refused with. A refusal that is not a Fault would reach a
// client as a 500, so it fails the test rather than being tolerated.
func faultOf(t *testing.T, fn func()) *Fault {
	t.Helper()
	var fault *Fault
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			f, ok := r.(*Fault)
			if !ok {
				t.Fatalf("panic is not a Fault: %v", r)
			}
			fault = f
		}()
		fn()
	}()
	if fault == nil {
		t.Fatal("expected a refusal, got none")
	}
	return fault
}

// noFault fails the test when fn refuses. It is how a case asserts that a legal document is admitted,
// which is the half of a validator that a matrix of violations cannot cover.
func noFault(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected refusal: %v", r)
		}
	}()
	fn()
}

// graphStart, graphAgent, graphEdge and graphSlot build the document shapes the validator reads. They
// are deliberately built by hand rather than decoded: the validator reads a document the API never
// interprets, so it has to be exercised against the shapes a hand-edited or imported document can
// actually have, not only against what the editor writes.
func graphStart(id string) Object {
	return Object{"id": id, "type": "workflow", "data": Object{"kind": "start"}}
}

// graphAgent builds an agent node. An empty slot leaves `executorSlot` off, which is the implicit
// `default`; `legacy` adds the pre-slot `agentConfig.executor` field a desktop-imported graph still
// carries.
func graphAgent(id, slot string, legacy bool) Object {
	config := Object{"schemaVersion": 1, "prompt": "review"}
	if slot != "" {
		config["executorSlot"] = slot
	}
	if legacy {
		config["executor"] = Object{"agentCli": "open_code", "modelId": "vendor/model"}
	}
	return Object{"id": id, "type": "workflow", "data": Object{"kind": "agent", "agentConfig": config}}
}

func graphEdge(source, target string) Object {
	return Object{"id": source + "->" + target, "source": source, "target": target}
}

func graphSlot(name, description string) Object {
	return Object{"name": name, "description": description}
}

// workflowGraph assembles a document. A nil `slots` leaves `executorSlots` off entirely, the way a
// graph authored before slots existed is stored.
func workflowGraph(nodes, edges, slots []any) Object {
	g := Object{"nodes": nodes, "edges": edges}
	if slots != nil {
		g["executorSlots"] = slots
	}
	return g
}

// problemObjects renders the validator's output the way the fault carries it, so a case compares the
// complete wire value rather than a count.
func problemObjects(problems []workflowGraphProblem) []any {
	out := []any{}
	for _, p := range problems {
		out = append(out, p.Object())
	}
	return out
}

// TestValidateWorkflowGraph is the canonical-schema matrix: every rule the publish gate enforces has a
// case that violates it, and the cases that must *not* be refused — the legacy field, an explicit
// `default`, a slot declared for later — are here too, because a validator that refuses a legal
// document is worse than one that misses a rule.
func TestValidateWorkflowGraph(t *testing.T) {
	longDescription := strings.Repeat("x", maxSlotDescription+1)

	cases := []struct {
		name  string
		graph Object
		want  []any
	}{
		{
			name:  "a start node and an agent node",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", "", false)}, []any{graphEdge("s", "a")}, nil),
		},
		{
			name: "a declared and referenced slot",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", "strong", false)}, []any{graphEdge("s", "a")},
				[]any{graphSlot("strong", "深度推理")}),
		},
		{
			// An author may be mid-edit or keeping a slot for later; refusing it would block publishing
			// for a mistake with no runtime consequence.
			name: "a declared but unreferenced slot",
			graph: workflowGraph([]any{graphStart("s")}, []any{},
				[]any{graphSlot("strong", "深度推理")}),
		},
		{
			// specs#98 D5: cloud accepts a desktop-imported graph and reads the node as `default`.
			name:  "the pre-slot executor field is accepted",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", "", true)}, []any{graphEdge("s", "a")}, nil),
		},
		{
			name:  "an explicit default reference",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", workflowDefaultSlot, false)}, []any{graphEdge("s", "a")}, nil),
		},
		{
			name: "a valid hint",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", "strong", false)}, []any{graphEdge("s", "a")},
				[]any{Object{"name": "strong", "description": "深度推理", "hint": Object{"agentCli": "official/ora-space.opencode", "modelId": "vendor/model"}}}),
		},
		{
			name:  "nodes is not a list",
			graph: workflowGraph(nil, []any{}, nil),
			want:  []any{Object{"code": "nodes_not_a_list"}, Object{"code": "start_node_count"}},
		},
		{
			name:  "edges is not a list",
			graph: Object{"nodes": []any{graphStart("s")}, "edges": "no"},
			want:  []any{Object{"code": "edges_not_a_list"}},
		},
		{
			name:  "a node that is not an object",
			graph: workflowGraph([]any{graphStart("s"), "not a node"}, []any{}, nil),
			want:  []any{Object{"code": "node_not_an_object"}},
		},
		{
			name:  "a node with no id",
			graph: workflowGraph([]any{graphStart("s"), Object{"data": Object{"kind": "output"}}}, []any{}, nil),
			want:  []any{Object{"code": "node_id_missing"}},
		},
		{
			name:  "a duplicated node id",
			graph: workflowGraph([]any{graphStart("s"), graphStart("s")}, []any{}, nil),
			want:  []any{Object{"code": "node_id_duplicate", "nodeId": "s"}},
		},
		{
			name:  "a node with no kind",
			graph: workflowGraph([]any{graphStart("s"), Object{"id": "a", "data": Object{}}}, []any{}, nil),
			want:  []any{Object{"code": "node_kind_missing", "nodeId": "a"}},
		},
		{
			// A node kind outside the first-phase executable subset is refused by name rather than
			// silently dropped from the frozen graph.
			name:  "an unsupported node kind",
			graph: workflowGraph([]any{graphStart("s"), Object{"id": "t", "data": Object{"kind": "tool"}}}, []any{}, nil),
			want:  []any{Object{"code": "node_kind_unsupported", "nodeId": "t", "kind": "tool"}},
		},
		{
			name:  "no start node",
			graph: workflowGraph([]any{Object{"id": "a", "data": Object{"kind": "output"}}}, []any{}, nil),
			want:  []any{Object{"code": "start_node_count"}},
		},
		{
			name:  "two start nodes",
			graph: workflowGraph([]any{graphStart("s1"), graphStart("s2")}, []any{}, nil),
			want:  []any{Object{"code": "start_node_count"}},
		},
		{
			name:  "an edge that is not an object",
			graph: workflowGraph([]any{graphStart("s")}, []any{"not an edge"}, nil),
			want:  []any{Object{"code": "edge_not_an_object"}},
		},
		{
			name:  "an edge to a node the document does not contain",
			graph: workflowGraph([]any{graphStart("s")}, []any{graphEdge("s", "gone")}, nil),
			want:  []any{Object{"code": "edge_endpoint_unknown", "nodeId": "gone"}},
		},
		{
			// The nameless slot's description is fine and is not also reported: a problem list the user
			// has to read is worth keeping to the violations that are actually there.
			name:  "a slot with no name",
			graph: workflowGraph([]any{graphStart("s")}, []any{}, []any{graphSlot("", "深度推理")}),
			want:  []any{Object{"code": "slot_name_missing"}},
		},
		{
			name:  "a slot name that is not a lowercase identifier",
			graph: workflowGraph([]any{graphStart("s")}, []any{}, []any{graphSlot("Strong", "深度推理")}),
			want:  []any{Object{"code": "slot_name_invalid", "slot": "Strong"}},
		},
		{
			// `default` is the one name every runner already has; declaring it would let an author
			// redefine it.
			name:  "a slot named default",
			graph: workflowGraph([]any{graphStart("s")}, []any{}, []any{graphSlot("default", "深度推理")}),
			want:  []any{Object{"code": "slot_name_reserved", "slot": "default"}},
		},
		{
			name: "a duplicated slot name",
			graph: workflowGraph([]any{graphStart("s")}, []any{},
				[]any{graphSlot("strong", "深度推理"), graphSlot("strong", "另一个")}),
			want: []any{Object{"code": "slot_name_duplicate", "slot": "strong"}},
		},
		{
			name:  "a slot with no description",
			graph: workflowGraph([]any{graphStart("s")}, []any{}, []any{graphSlot("strong", "   ")}),
			want:  []any{Object{"code": "slot_description_invalid", "slot": "strong"}},
		},
		{
			name:  "a slot description past the limit",
			graph: workflowGraph([]any{graphStart("s")}, []any{}, []any{graphSlot("strong", longDescription)}),
			want:  []any{Object{"code": "slot_description_invalid", "slot": "strong"}},
		},
		{
			name: "a hint of the wrong shape",
			graph: workflowGraph([]any{graphStart("s")}, []any{},
				[]any{Object{"name": "strong", "description": "深度推理", "hint": Object{"agentCli": 7}}}),
			want: []any{Object{"code": "slot_hint_invalid", "slot": "strong"}},
		},
		{
			// A reference to an undeclared slot is refused at publish rather than at run time: only the
			// author can say what the slot means, and a run against it could never resolve an executor.
			name:  "a reference to an undeclared slot",
			graph: workflowGraph([]any{graphStart("s"), graphAgent("a", "strong", false)}, []any{graphEdge("s", "a")}, nil),
			want:  []any{Object{"code": "slot_not_declared", "nodeId": "a", "slot": "strong"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// decodeGraph renders the hand-built document the way the jsonb column hands it back:
			// every nested object a plain map rather than the named Object type it was built from.
			// The validator reads stored documents, and that is the shape it actually sees.
			got := problemObjects(validateWorkflowGraph(decodeGraph(t, tc.graph)))
			if tc.want == nil {
				tc.want = []any{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("problems = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRequireValidWorkflowGraphIsOneStableFault covers the wire contract of the publish refusal: one
// code whatever went wrong, and every problem carried as a parameter so the editor can mark each
// node. A client that branched on a per-violation code would break the day a new rule was added.
func TestRequireValidWorkflowGraphIsOneStableFault(t *testing.T) {
	noFault(t, func() {
		requireValidWorkflowGraph(decodeGraph(t, workflowGraph([]any{graphStart("s"), graphAgent("a", "", false)}, []any{graphEdge("s", "a")}, nil)))
	})

	fault := faultOf(t, func() {
		requireValidWorkflowGraph(decodeGraph(t, workflowGraph([]any{graphStart("s"), Object{"id": "t", "data": Object{"kind": "tool"}}}, []any{}, nil)))
	})
	if fault.Code != "workflow_graph_invalid" || fault.Status != 400 {
		t.Fatalf("fault = %s/%d, want workflow_graph_invalid/400", fault.Code, fault.Status)
	}
	want := []any{Object{"code": "node_kind_unsupported", "nodeId": "t", "kind": "tool"}}
	if !reflect.DeepEqual(fault.Params["problems"], want) {
		t.Fatalf("problems = %v, want %v", fault.Params["problems"], want)
	}
}
