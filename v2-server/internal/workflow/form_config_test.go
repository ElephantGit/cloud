package workflow

import (
	"encoding/json"
	"testing"
)

func reviewGraph(t *testing.T, change func(map[string]any)) string {
	t.Helper()
	g := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start", "data": map[string]any{"kind": "start"}},
			map[string]any{"id": "review", "data": map[string]any{"kind": "form", "formConfig": map[string]any{"title": "Review", "submitLabel": "Submit", "reportSelector": []string{"start", "input"}, "itemsSelector": []string{"start", "input"}, "fields": []any{}}}},
			map[string]any{"id": "tasks", "data": map[string]any{"kind": "createTasks", "createTasksConfig": map[string]any{"formNodeId": "review"}}},
			map[string]any{"id": "output", "data": map[string]any{"kind": "output"}},
		},
		"edges": []any{map[string]any{"source": "start", "target": "review"}, map[string]any{"source": "review", "target": "tasks"}, map[string]any{"source": "tasks", "target": "output"}},
	}
	if change != nil {
		change(g)
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReviewGraphPublication(t *testing.T) {
	g, err := ParseGraph(reviewGraph(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	n, ok := g.Node("review")
	if !ok || len(n.FormConfig()) == 0 {
		t.Fatal("missing form configuration")
	}
	tasks, _ := g.Node("tasks")
	if tasks.TaskFormNode() != "review" {
		t.Fatal("lost form reference")
	}

}

func TestReviewGraphRejectsUnsafeTail(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"wrong form", func(g map[string]any) {
			g["nodes"].([]any)[2].(map[string]any)["data"].(map[string]any)["createTasksConfig"] = map[string]any{"formNodeId": "other"}
		}},
		{"missing configuration", func(g map[string]any) {
			delete(g["nodes"].([]any)[1].(map[string]any)["data"].(map[string]any), "formConfig")
		}},
		{"automatic work after review", func(g map[string]any) {
			g["nodes"].([]any)[3].(map[string]any)["data"] = map[string]any{"kind": "tool"}
		}},
		{"second form", func(g map[string]any) {
			g["nodes"].([]any)[3].(map[string]any)["data"] = g["nodes"].([]any)[1].(map[string]any)["data"]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseGraph(reviewGraph(t, tc.edit)); err == nil {
				t.Fatal("accepted unsafe review graph")
			}
		})
	}
}
