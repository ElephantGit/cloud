package server_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// desktopCanvas starts with a real Desktop export, including its round Start and port metadata.
func desktopCanvas(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("testdata/desktop-workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var graph map[string]any
	if err := json.Unmarshal(raw, &graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

func TestDesktopCanvasSavePublishRestoreAndRestart(t *testing.T) {
	s, config := openStoreConfig(t)
	c := workflowClient(t, s)
	root := loginWorkflow(t, c)
	graph := desktopCanvas(t)
	// Complete the eight-kind fixture and exercise fields a renderer must not discard.
	nodes := graph["nodes"].([]any)
	nodes = append(nodes,
		map[string]any{"id": "agg", "type": "workflow", "position": map[string]any{"x": 10, "y": 20}, "data": map[string]any{"kind": "aggregator", "aggregatorConfig": map[string]any{"variables": [][]string{{"start", "flag"}, {"start", "count"}}}}},
		map[string]any{"id": "iter", "type": "workflow", "position": map[string]any{"x": 30, "y": 40}, "data": map[string]any{"kind": "iteration", "iterationConfig": map[string]any{"iteratorSelector": []string{"start", "items"}, "collectSelector": []string{"iter-agent", "output"}, "maxIterations": 50, "errorStrategy": "continue"}}},
		map[string]any{"id": "iter-agent", "type": "workflow", "parentId": "iter", "position": map[string]any{"x": 50, "y": 60}, "data": map[string]any{"kind": "agent", "title": "Iteration agent"}},
	)
	graph["nodes"] = nodes
	graph["schemaVersion"] = 2
	graph["viewport"] = map[string]any{"x": -120.5, "y": 82.25, "zoom": 0.75}
	graph["annotations"] = []any{map[string]any{"id": "note", "type": "annotation", "position": map[string]any{"x": 1, "y": 2}, "data": map[string]any{"text": "Canvas note", "theme": "blue"}}}
	graph["futureExtension"] = map[string]any{"values": []any{nil, false, 0, "", []any{}, map[string]any{}}}
	graph["edges"] = append(graph["edges"].([]any), map[string]any{"id": "branch", "source": "condition-1", "target": "output-1", "sourceHandle": "case-1", "targetHandle": "input", "data": map[string]any{"futureFlag": false}})
	// Normalize the expected JSON value to the HTTP decoder's representation.
	raw, _ := json.Marshal(graph)
	_ = json.Unmarshal(raw, &graph)
	status, body := c.do("POST", root, map[string]any{"name": "Canvas compatibility", "graph": graph})
	if status != 200 {
		t.Fatal(status, body)
	}
	wf := body["resource"].(map[string]any)
	path := root + "/" + wf["id"].(string)
	if !reflect.DeepEqual(wf["graph"], graph) {
		t.Fatal("create changed canvas fields", wf)
	}
	status, published := c.do("POST", path+"/publish", map[string]any{})
	if status != 200 {
		t.Fatal("Desktop loop must publish with its round Start", status, published)
	}
	snapshot := published["resource"].(map[string]any)
	if !reflect.DeepEqual(snapshot["graph"], graph) {
		t.Fatal("publish changed canvas", snapshot)
	}
	if status, body = c.do("PUT", path, map[string]any{"name": "Edited", "version": 1, "graph": draftGraph("Changed")}); status != 200 {
		t.Fatal(status, body)
	}
	// Restore is fenced like an edit; the immutable snapshot and current metadata remain intact.
	restorePath := path + "/snapshots/" + snapshot["id"].(string) + "/restore"
	if status, body = c.do("PUT", restorePath, map[string]any{}); status != 428 {
		t.Fatal(status, body)
	}
	if status, body = c.do("PUT", restorePath, map[string]any{"version": 1}); status != 409 {
		t.Fatal(status, body)
	}
	status, restored := c.do("PUT", restorePath, map[string]any{"version": 2})
	if status != 200 || !reflect.DeepEqual(restored["graph"], graph) || restored["name"] != "Edited" || restored["version"] != float64(3) {
		t.Fatal(status, restored)
	}
	s.Close()
	reopened, err := store.OpenConfig(context.Background(), config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	c = workflowClient(t, reopened)
	loginWorkflow(t, c)
	if status, body = c.do("GET", path, nil); status != 200 || !reflect.DeepEqual(body, restored) {
		t.Fatal("restart lost restored canvas", status, body)
	}
	if status, body = c.do("GET", path+"/snapshots/"+snapshot["id"].(string), nil); status != 200 || !reflect.DeepEqual(body, snapshot) {
		t.Fatal("restore mutated frozen snapshot", status, body)
	}
}

func TestRestoreRejectsForeignAndArchivedSnapshots(t *testing.T) {
	c := newClient(t)
	root := loginWorkflow(t, c)
	first := createDraft(t, c, root, "First")
	second := createDraft(t, c, root, "Second")
	firstPath := root + "/" + first["id"].(string)
	secondPath := root + "/" + second["id"].(string)
	_, body := c.do("POST", firstPath+"/publish", map[string]any{})
	sid := body["resource"].(map[string]any)["id"].(string)
	if status, body := c.do("PUT", secondPath+"/snapshots/"+sid+"/restore", map[string]any{"version": 1}); status != 404 {
		t.Fatal(status, body)
	}
	if status, body := c.do("GET", secondPath, nil); status != 200 || body["version"] != float64(1) {
		t.Fatal("failed restore wrote the draft", status, body)
	}
	if status, body := c.do("DELETE", firstPath, map[string]any{"version": 1}); status != 200 {
		t.Fatal(status, body)
	}
	if status, body := c.do("PUT", firstPath+"/snapshots/"+sid+"/restore", map[string]any{"version": 2}); status != 404 {
		t.Fatal(status, body)
	}
}

func TestCanvasPublishRejectsBrokenContainerOwnership(t *testing.T) {
	c := newClient(t)
	root := loginWorkflow(t, c)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing parent", func(g map[string]any) { g["nodes"].([]any)[4].(map[string]any)["parentId"] = "missing" }},
		{"mismatched loop owner", func(g map[string]any) {
			delete(g["nodes"].([]any)[4].(map[string]any)["data"].(map[string]any), "containerId")
		}},
		{"container cycle", func(g map[string]any) {
			n := g["nodes"].([]any)[1].(map[string]any)
			n["parentId"] = "loop-1"
			n["data"].(map[string]any)["containerId"] = "loop-1"
		}},
		{"duplicate root start", func(g map[string]any) {
			g["nodes"] = append(g["nodes"].([]any), map[string]any{"id": "second-root", "type": "workflow", "data": map[string]any{"kind": "start"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := desktopCanvas(t)
			tc.mutate(graph)
			status, body := c.do("POST", root, map[string]any{"name": tc.name, "graph": graph})
			if status != 200 {
				t.Fatal("unfinished drafts must remain saveable", status, body)
			}
			path := root + "/" + body["resource"].(map[string]any)["id"].(string)
			if status, body = c.do("POST", path+"/publish", map[string]any{}); status != 400 || body["code"] != "workflow_graph_invalid" {
				t.Fatal(status, body)
			}
		})
	}
}
