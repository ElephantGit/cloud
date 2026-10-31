package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// decodeGraph keeps the original regression cases at the JSON persistence boundary.
func decodeGraph(t *testing.T, graph Object) Object {
	t.Helper()
	b, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	var out Object
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func encodeGraph(t *testing.T, graph Object) []byte {
	t.Helper()
	raw, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	return raw
}

func TestFreezeGraphPreservesEditorNumbersAndNormalizesFiles(t *testing.T) {
	raw := json.RawMessage(`{"nodes":[{"id":"s","type":"workflow","data":{"kind":"start"}}],"edges":[],"editor":{"revision":9007199254740993},"globalVariables":[{"name":"env.file","valueType":"file","value":"docs/input.md"}]}`)
	frozen, err := FreezeGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(frozen, []byte(`9007199254740993`)) || !bytes.Contains(frozen, []byte(`"kind":"workspace_file"`)) || !bytes.Contains(frozen, []byte(`"path":"docs/input.md"`)) {
		t.Fatalf("frozen graph lost authored data: %s", frozen)
	}
	if bytes.Contains(raw, []byte(`workspace_file`)) {
		t.Fatal("publication changed the draft")
	}
}

func TestFreezeGraphRejectsIncompleteAndOversizedExecutableGraphs(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`null`), json.RawMessage(`[]`)} {
		var invalid *GraphError
		if _, err := FreezeGraph(raw); !errors.As(err, &invalid) || len(invalid.Problems) == 0 {
			t.Fatalf("published %s: %v", raw, err)
		}
	}
	nodes := make([]any, 257)
	for i := range nodes {
		nodes[i] = map[string]any{"id": "s", "data": map[string]any{"kind": "start"}}
	}
	raw, _ := json.Marshal(map[string]any{"nodes": nodes, "edges": []any{}})
	var invalid *GraphError
	if _, err := FreezeGraph(raw); !errors.As(err, &invalid) || invalid.Problems[0].S("code") != "graph_execution_limit" {
		t.Fatalf("execution bound: %v", err)
	}
}
