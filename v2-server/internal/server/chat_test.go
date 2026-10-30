package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/wanglongan587/cloud/v2-server/internal/chat"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

var chatContract = sync.OnceValues(func() (routers.Router, error) {
	doc, e := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "chat.openapi.json"))
	if e != nil {
		return nil, e
	}
	if e := doc.Validate(context.Background()); e != nil {
		return nil, e
	}
	return legacy.NewRouter(doc)
})

// events is a finished session's retained events; Next replays them after the cursor, then EOF.
type events struct {
	list []execution.Event
	at   int
}

func (s *events) Next(context.Context) (execution.Event, error) {
	if s.at == len(s.list) {
		return execution.Event{}, io.EOF
	}
	s.at++
	return s.list[s.at-1], nil
}

func (s *events) Close() error { return nil }

// chatRuntime starts a session per conversation, runs one turn at a time until finish ends it,
// ends the session when a running turn is cancelled, and replays the same events for any session.
func chatRuntime() (rt *executiontest.Stub, finish func(sid string)) {
	var mu sync.Mutex
	running := map[string]bool{}
	ended := map[string]bool{}
	n := 0
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	finish = func(sid string) {
		mu.Lock()
		defer mu.Unlock()
		running[sid] = false
	}
	return &executiontest.Stub{
		ListExecutorsFunc: func(context.Context, execution.Scope) ([]execution.Executor, error) {
			return []execution.Executor{{ID: "fixture", Name: "OpenCode · Fixture", AgentName: "OpenCode", Model: execution.Model{ID: "fixture", Name: "Fixture"}}}, nil
		},
		StartFunc: func(_ context.Context, _ execution.Scope, req execution.StartRequest) (execution.Ref, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			sid := fmt.Sprintf("s%d", n)
			running[sid] = true
			return execution.Ref{SessionID: sid, TurnID: "t1"}, nil
		},
		AppendFunc: func(_ context.Context, _ execution.Scope, req execution.AppendRequest) (execution.Ref, error) {
			mu.Lock()
			defer mu.Unlock()
			switch {
			case ended[req.SessionID]:
				return execution.Ref{}, execution.ErrSessionEnded
			case running[req.SessionID]:
				return execution.Ref{}, execution.ErrWorkspaceBusy
			}
			running[req.SessionID] = true
			return execution.Ref{SessionID: req.SessionID, TurnID: "t2"}, nil
		},
		CancelFunc: func(_ context.Context, _ execution.Scope, ref execution.Ref) (execution.Result, error) {
			mu.Lock()
			defer mu.Unlock()
			running[ref.SessionID], ended[ref.SessionID] = false, true
			return execution.Result{Status: execution.StatusCancelled, StopReason: "cancelled", FailureCode: ""}, nil
		},
		ReadSessionFunc: func(_ context.Context, _ execution.Scope, sid string) (execution.Session, error) {
			mu.Lock()
			defer mu.Unlock()
			s := execution.Session{ID: sid}
			if ended[sid] {
				s.EndedAt = &at
			}
			return s, nil
		},
		EndFunc: func(_ context.Context, _ execution.Scope, sid string) error {
			mu.Lock()
			defer mu.Unlock()
			running[sid], ended[sid] = false, true
			return nil
		},
		SubscribeFunc: func(_ context.Context, _ execution.Scope, sid string, after uint64) (execution.Stream, error) {
			all := []execution.Event{
				{Ref: execution.Ref{SessionID: sid, TurnID: "t1"}, Sequence: 1, OccurredAt: at, Kind: execution.EventHistorySnapshot,
					History: &execution.HistorySnapshot{Records: []execution.HistoryRecord{{Sequence: 1, Data: json.RawMessage(`{"at":"2026-10-30T09:00:00Z","seq":1,"type":"turnEnded","stop_reason":"end_turn"}`)}}}},
				{Ref: execution.Ref{SessionID: sid, TurnID: "t1"}, Sequence: 2, OccurredAt: at, Kind: execution.EventTurnEnded,
					Result: &execution.Result{Status: execution.StatusCompleted, Answer: "好", AnswerComplete: true, StopReason: "end_turn"}},
				{Ref: execution.Ref{SessionID: sid}, Sequence: 3, OccurredAt: at, Kind: execution.EventSessionEnded},
			}
			return &events{list: all[min(int(after), len(all)):]}, nil
		},
	}, finish
}

type chatClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	tenant string
}

func newChatClient(t *testing.T, svc func(*store.Store) *chat.Service) *chatClient {
	t.Helper()
	s := storetest.Open(t)
	var c *chat.Service
	if svc != nil {
		c = svc(s)
	}
	srv := httptest.NewServer(server.New(s, server.Options{
		SessionTTL: time.Hour, DemoUser: store.Identity{Source: "dev", Subject: "chat", DisplayName: "Chat"}, Chat: c,
	}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	cl := &chatClient{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
	cl.raw("POST", "/auth/login", map[string]string{}, nil)
	_, me := cl.raw("GET", "/api/v1/me/tenants", nil, nil)
	cl.tenant = me["items"].([]any)[0].(map[string]any)["id"].(string)
	return cl
}

// withRuntime wires a chat service to chatRuntime; finish lets a test end a running turn.
func withRuntime(finish *func(string)) func(*store.Store) *chat.Service {
	return func(s *store.Store) *chat.Service {
		rt, f := chatRuntime()
		if finish != nil {
			*finish = f
		}
		return chat.New(s, struct {
			execution.Service
			execution.Catalog
		}{rt, rt}, chat.Options{})
	}
}

func (c *chatClient) raw(method, path string, body any, header map[string]string) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, e := c.http.Do(req)
	if e != nil {
		c.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if strings.Contains(path, "/chat/") {
		routes, e := chatContract()
		if e != nil {
			c.t.Fatal(e)
		}
		route, params, e := routes.FindRoute(req)
		if e != nil {
			c.t.Fatalf("%s %s is not in chat.openapi.json: %v", method, path, e)
		}
		in := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
			Status:                 res.StatusCode, Header: res.Header, Options: &openapi3filter.Options{IncludeResponseStatus: true},
		}
		in.SetBodyBytes(raw)
		if e := openapi3filter.ValidateResponse(context.Background(), in); e != nil {
			c.t.Fatalf("%s %s does not match chat.openapi.json: %v\n%s", method, path, e, raw)
		}
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func (c *chatClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	return c.raw(method, "/api/v2/tenants/"+c.tenant+"/chat"+path, body, nil)
}

func (c *chatClient) post(path, key string, body any) (int, map[string]any) {
	c.t.Helper()
	return c.raw("POST", "/api/v2/tenants/"+c.tenant+"/chat"+path, body, map[string]string{"Idempotency-Key": key})
}

func TestChatUnavailableWithoutRuntime(t *testing.T) {
	c := newChatClient(t, nil)
	if status, body := c.do("GET", "/conversations", nil); status != 503 || body["code"] != "execution_unavailable" {
		t.Fatalf("without a runtime = %d %v", status, body)
	}
}

func TestChatConversationLifecycle(t *testing.T) {
	var finish func(string)
	c := newChatClient(t, withRuntime(&finish))
	if status, body := c.do("GET", "/executors", nil); status != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("executors = %d %v", status, body)
	}
	input := map[string]any{"projectId": "demo", "executorId": "fixture", "prompt": "你好"}
	if status, body := c.do("POST", "/conversations", input); status != 400 || body["code"] != "idempotency_key_required" {
		t.Fatalf("create without key = %d %v", status, body)
	}
	if status, body := c.post("/conversations", "k0", map[string]any{"projectId": "demo", "executorId": "fixture", "prompt": " "}); status != 400 || body["code"] != "invalid_prompt" {
		t.Fatalf("blank prompt = %d %v", status, body)
	}
	status, created := c.post("/conversations", "k1", input)
	if status != 201 {
		t.Fatalf("create = %d %v", status, created)
	}
	conv := created["conversation"].(map[string]any)
	id := conv["id"].(string)
	if conv["title"] != "你好" || conv["session"].(map[string]any)["id"] != "s1" || created["turn"].(map[string]any)["sessionId"] != "s1" {
		t.Fatalf("created = %v", created)
	}
	if status, body := c.post("/conversations", "k2", input); status != 201 || body["turn"].(map[string]any)["sessionId"] != "s2" {
		t.Fatalf("a second conversation in the same project = %d %v", status, body)
	}
	if status, body := c.post("/conversations/"+id+"/messages", "k3", map[string]any{"prompt": "继续"}); status != 409 || body["code"] != "turn_running" {
		t.Fatalf("send while the turn runs = %d %v", status, body)
	}
	finish("s1")
	if status, body := c.post("/conversations/"+id+"/messages", "k3", map[string]any{"prompt": "继续"}); status != 201 || body["turn"].(map[string]any)["turnId"] != "t2" {
		t.Fatalf("send = %d %v", status, body)
	}
	if status, body := c.do("PATCH", "/conversations/"+id, map[string]any{"title": "改名"}); status != 200 || body["title"] != "改名" {
		t.Fatalf("rename = %d %v", status, body)
	}
	if status, body := c.post("/conversations/"+id+"/stop", "k4", nil); status != 200 || body["result"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("stop = %d %v", status, body)
	}
	if status, body := c.post("/conversations/"+id+"/stop", "k5", nil); status != 409 || body["code"] != "no_active_turn" {
		t.Fatalf("stop again = %d %v", status, body)
	}
	if status, body := c.post("/conversations/"+id+"/messages", "k6", map[string]any{"prompt": "再来"}); status != 409 || body["code"] != "conversation_ended" {
		t.Fatalf("send after stop = %d %v", status, body)
	}
	status, list := c.do("GET", "/conversations", nil)
	if items := list["items"].([]any); status != 200 || len(items) != 2 || items[0].(map[string]any)["session"].(map[string]any)["endReason"] != "stopped" {
		t.Fatalf("list = %d %v", status, list)
	}
	if status, _ := c.do("DELETE", "/conversations/"+id, nil); status != 204 {
		t.Fatalf("delete = %d", status)
	}
	if status, _ := c.do("PATCH", "/conversations/"+id, map[string]any{"title": "x"}); status != 404 {
		t.Fatalf("rename deleted = %d", status)
	}
	if status, _ := c.raw("GET", "/api/v2/tenants/00000000-0000-0000-0000-000000000001/chat/conversations", nil, nil); status != 404 {
		t.Fatalf("other tenant = %d", status)
	}
}

func TestChatEventStream(t *testing.T) {
	c := newChatClient(t, withRuntime(nil))
	_, created := c.post("/conversations", "k1", map[string]any{"projectId": "demo", "executorId": "fixture", "prompt": "你好"})
	id := created["conversation"].(map[string]any)["id"].(string)
	path := c.base + "/api/v2/tenants/" + c.tenant + "/chat/conversations/" + id + "/events"

	read := func(lastEventID string) []string {
		req, _ := http.NewRequest("GET", path, nil)
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		res, e := c.http.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("events = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		var lines []string
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if l := sc.Text(); l != "" {
				lines = append(lines, l)
			}
		}
		return lines
	}
	lines := read("")
	if len(lines) != 8 || lines[0] != "id: 1" || lines[2] != "id: 2" || lines[4] != "id: 3" || lines[6] != "event: end" {
		t.Fatalf("stream = %q", lines)
	}
	var first map[string]any
	_ = json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &first)
	records := first["history"].(map[string]any)["records"].([]any)
	if first["kind"] != "history_snapshot" || first["turnId"] != "t1" || records[0].(map[string]any)["data"].(map[string]any)["type"] != "turnEnded" {
		t.Fatalf("first event = %v", first)
	}
	if resumed := read("2"); len(resumed) != 4 || resumed[0] != "id: 3" {
		t.Fatalf("resumed after 2 = %q", resumed)
	}
	req, _ := http.NewRequest("GET", path+"?after=x", nil)
	if res, _ := c.http.Do(req); res.StatusCode != 400 {
		t.Fatalf("bad cursor = %d", res.StatusCode)
	}
	req, _ = http.NewRequest("GET", strings.Replace(path, id, "00000000-0000-0000-0000-000000000009", 1), nil)
	if res, _ := c.http.Do(req); res.StatusCode != 404 {
		t.Fatalf("unknown conversation = %d", res.StatusCode)
	}
}

func TestChatProjectOrder(t *testing.T) {
	c := newChatClient(t, withRuntime(nil))
	if status, body := c.do("GET", "/project-order", nil); status != 200 || len(body["projectIds"].([]any)) != 0 {
		t.Fatalf("empty order = %d %v", status, body)
	}
	if status, body := c.do("PUT", "/project-order", map[string]any{"projectIds": []string{"a", "a"}}); status != 400 || body["code"] != "invalid_project_order" {
		t.Fatalf("duplicate ids = %d %v", status, body)
	}
	c.do("PUT", "/project-order", map[string]any{"projectIds": []string{"b", "a"}})
	if _, body := c.do("GET", "/project-order", nil); body["projectIds"].([]any)[0] != "b" {
		t.Fatalf("saved order = %v", body)
	}
}
