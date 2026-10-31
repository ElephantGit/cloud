package workflow

import (
	"encoding/json"
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
