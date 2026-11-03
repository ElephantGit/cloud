package server_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/taskdecomp"
)

// finishingRuns is a workflow host whose runs end as soon as they start, with a fixed output.
type finishingRuns struct {
	store  *store.Store
	output string
}

func (r finishingRuns) Start(ctx context.Context, tenant, _, runID string) error {
	if _, e := r.store.StartWorkflowRun(ctx, tenant, runID); e != nil {
		return e
	}
	_, e := r.store.FinishWorkflowRun(ctx, tenant, runID, store.WorkflowStatusSucceeded, &r.output, nil)
	return e
}

func (finishingRuns) Subscribe(string) (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}

func TestTaskDecompositionHTTP(t *testing.T) {
	f := newSquadServer(t)
	decompositions := taskdecomp.New(f.store, finishingRuns{store: f.store,
		output: `{"tasks":[{"title":"登录页","description":"邮箱登录"},{"title":"注册页","description":"校验邮箱"}]}`}, nil)
	t.Cleanup(decompositions.Close)
	f.srv.Close()
	f.srv = httptest.NewServer(server.New(f.store, server.Options{SessionTTL: time.Hour, TaskDecompositions: decompositions}))
	t.Cleanup(f.srv.Close)
	c := f.taskClient()
	c.login()
	tid := f.tenant(t)
	f.project(t)
	w, _, e := f.store.CreateWorkflow(f.ctx, tid, "需求拆分", `{"nodes":[{"id":"start","type":"workflow","data":{"kind":"start"}}],"edges":[]}`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.PublishWorkflow(f.ctx, tid, w.ID, "v1", nil); e != nil {
		t.Fatal(e)
	}
	base := "/api/v2/tenants/" + tid + "/tasks"
	status, parent := c.do("POST", base, map[string]any{"title": "账号体系", "description": "登录和注册", "projectId": taskProjectID})
	if status != 201 || parent["parentId"] != nil {
		t.Fatalf("create = %d %v", status, parent)
	}
	split := base + "/" + parent["id"].(string) + "/decompositions"
	key := map[string]string{"Idempotency-Key": "split-1"}

	if status, body := c.do("POST", split, map[string]any{"workflowId": w.ID}); status != 400 || body["code"] != "idempotency_key_required" {
		t.Fatalf("no key = %d %v", status, body)
	}
	if status, body := c.doHeaders("POST", split, map[string]any{"workflowId": "00000000-0000-4000-8000-000000000999"}, key); status != 400 || body["code"] != "invalid_workflow" {
		t.Fatalf("unknown workflow = %d %v", status, body)
	}
	status, started := c.doHeaders("POST", split, map[string]any{"workflowId": w.ID}, key)
	if status != 201 || started["workflowName"] != "需求拆分" || started["workflowRunId"] == "" {
		t.Fatalf("start = %d %v", status, started)
	}

	deadline := time.Now().Add(5 * time.Second)
	var done map[string]any
	for {
		status, list := c.do("GET", split, nil)
		items, _ := list["items"].([]any)
		if status != 200 || len(items) != 1 {
			t.Fatalf("list = %d %v", status, list)
		}
		if done = items[0].(map[string]any); done["status"] != store.DecompositionRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never settled: %v", done)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if done["status"] != store.DecompositionSucceeded || done["createdCount"] != float64(2) || done["failure"] != "" {
		t.Fatalf("settled = %v", done)
	}

	_, list := c.do("GET", base, nil)
	var child string
	subtasks := 0
	for _, item := range list["items"].([]any) {
		task := item.(map[string]any)
		if task["parentId"] == parent["id"] {
			subtasks++
			child = task["id"].(string)
		}
	}
	if subtasks != 2 {
		t.Fatalf("tasks = %v", list)
	}
	if status, body := c.doHeaders("POST", base+"/"+child+"/decompositions", map[string]any{"workflowId": w.ID}, key); status != 409 || body["code"] != "subtask_not_decomposable" {
		t.Fatalf("split a subtask = %d %v", status, body)
	}
}
