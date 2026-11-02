package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/taskrun"
)

// endedStream replays nothing and reports the session over, so the events route sends "end".
type endedStream struct{}

func (endedStream) Next(context.Context) (execution.Event, error) { return execution.Event{}, io.EOF }
func (endedStream) Close() error                                  { return nil }

// taskRunServer serves the fixture's store with task runs on an execution stub whose turns run
// until stopped.
func (f *squadFixture) taskRunServer(t *testing.T) {
	t.Helper()
	runtime := &executiontest.Stub{
		ListExecutorsFunc: func(context.Context, execution.Scope) ([]execution.Executor, error) {
			return []execution.Executor{{ID: "opencode:fixture", Name: "OpenCode / Fixture", AgentID: "official/ora-space.opencode", AgentName: "OpenCode",
				Model: execution.Model{ID: "fixture", Name: "Fixture"}}}, nil
		},
		StartFunc: func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
			return execution.Ref{SessionID: "session-1", TurnID: "turn-1"}, nil
		},
		SubscribeFunc: func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
			return endedStream{}, nil
		},
		CancelFunc: func(context.Context, execution.Scope, execution.Ref) (execution.Result, error) {
			return execution.Result{Status: execution.StatusCancelled}, nil
		},
		EndFunc: func(context.Context, execution.Scope, string) error { return nil },
		ReadTurnsFunc: func(context.Context, execution.Scope, string, string, int) (execution.TurnPage, error) {
			return execution.TurnPage{}, nil
		},
	}
	host := taskrun.New(f.store, runtime, runtime, nil)
	t.Cleanup(host.Close)
	f.srv.Close()
	f.srv = httptest.NewServer(server.New(f.store, server.Options{SessionTTL: time.Hour, Execution: runtime, Catalog: runtime, TaskRuns: host}))
	t.Cleanup(f.srv.Close)
}

func TestTaskRunHTTP(t *testing.T) {
	f := newSquadServer(t)
	f.taskRunServer(t)
	tid := f.tenant(t)
	f.project(t)

	// The @ directory follows Cloud's contract: members are mentioned, installed Agents take tasks.
	contract := newContractClient(t, f.srv.URL)
	contract.login()
	status, targets := contract.do("GET", "/api/v1/tenants/"+tid+"/collaboration/targets", nil)
	if status != 200 {
		t.Fatalf("targets = %d %v", status, targets)
	}
	var userID, agentID string
	for _, item := range targets["items"].([]any) {
		target := item.(map[string]any)
		mode := target["interactionDescriptor"].(map[string]any)["mode"]
		switch {
		case target["type"] == "user" && mode == "mention":
			userID = target["id"].(string)
		case target["type"] == "agent" && mode == "task" && target["description"] == "official/ora-space.opencode":
			agentID = target["id"].(string)
		}
	}
	if userID == "" || agentID == "" {
		t.Fatalf("targets = %v", targets)
	}

	c := f.taskClient()
	c.login()
	base := "/api/v2/tenants/" + tid + "/tasks"
	status, created := c.do("POST", base, map[string]any{"title": "修复登录页", "projectId": taskProjectID,
		"assignee": map[string]any{"type": "agent", "id": agentID}})
	if status != 201 {
		t.Fatalf("create = %d %v", status, created)
	}
	one := base + "/" + created["id"].(string)

	if status, body := c.do("GET", one+"/executors", nil); status != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("executors = %d %v", status, body)
	}
	if status, body := c.do("POST", one+"/runs", map[string]any{}); status != 400 || body["code"] != "idempotency_key_required" {
		t.Fatalf("start without key = %d %v", status, body)
	}
	status, run := c.doHeaders("POST", one+"/runs", map[string]any{"note": "先看样式"}, map[string]string{"Idempotency-Key": "start-1"})
	if status != 201 || run["state"] != store.TaskRunRunning || run["startedByName"] != "演示用户" {
		t.Fatalf("start = %d %v", status, run)
	}
	status, task := c.do("GET", one, nil)
	active, _ := task["activeRun"].(map[string]any)
	if status != 200 || task["status"] != store.TaskRunning || active["id"] != run["id"] {
		t.Fatalf("task while running = %d %v", status, task)
	}
	if status, body := c.do("POST", one+"/status", map[string]any{"status": "review", "version": task["version"]}); status != 409 || body["code"] != "run_active" {
		t.Fatalf("manual move while running = %d %v", status, body)
	}

	status, comment := c.doHeaders("POST", one+"/comments", map[string]any{"body": "@演示用户 看一下", "mentions": []any{map[string]any{"type": "user", "id": userID}}},
		map[string]string{"Idempotency-Key": "comment-1"})
	if status != 201 || comment["runId"] != "" || len(comment["mentions"].([]any)) != 1 {
		t.Fatalf("comment = %d %v", status, comment)
	}
	if status, body := c.doHeaders("POST", one+"/comments", map[string]any{"body": "再改改", "toAgent": true},
		map[string]string{"Idempotency-Key": "comment-2"}); status != 409 || body["code"] != "run_active" {
		t.Fatalf("follow-up while running = %d %v", status, body)
	}
	if status, body := c.doHeaders("POST", one+"/comments", map[string]any{"body": "x", "mentions": []any{map[string]any{"type": "team", "id": userID}}},
		map[string]string{"Idempotency-Key": "comment-3"}); status != 400 || body["code"] != "invalid_mention" {
		t.Fatalf("squad mention = %d %v", status, body)
	}

	status, timeline := c.do("GET", one+"/timeline", nil)
	if status != 200 || len(timeline["runs"].([]any)) != 1 || len(timeline["comments"].([]any)) != 1 {
		t.Fatalf("timeline = %d %v", status, timeline)
	}

	// Every member reads the run's stream in the chat stream's shape.
	res, e := c.http.Get(c.base + one + "/runs/" + run["id"].(string) + "/events")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), "event: end") {
		t.Fatalf("events = %d %s", res.StatusCode, raw)
	}

	status, stopped := c.do("POST", one+"/runs/"+run["id"].(string)+"/stop", nil)
	if status != 200 || stopped["state"] != store.TaskRunEnded || stopped["endReason"] != store.TaskRunStopped {
		t.Fatalf("stop = %d %v", status, stopped)
	}
	if status, task := c.do("GET", one, nil); status != 200 || task["status"] != store.TaskPending || task["activeRun"] != nil {
		t.Fatalf("task after stop = %d %v", status, task)
	}
}

func TestTaskRunsUnavailableWithoutAHost(t *testing.T) {
	f := newSquadServer(t)
	tid := f.tenant(t)
	c := f.taskClient()
	c.login()
	path := "/api/v2/tenants/" + tid + "/tasks/00000000-0000-4000-8000-000000000001/runs"
	if status, body := c.doHeaders("POST", path, map[string]any{}, map[string]string{"Idempotency-Key": "k"}); status != 503 || body["code"] != "execution_unavailable" {
		t.Fatalf("start without a host = %d %v", status, body)
	}
}
