package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/requestmetrics"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

func TestDashboardHTTPContractAndAuthorization(t *testing.T) {
	ctx := context.Background()
	s := storetest.Open(t)
	token := storetest.SignIn(t, s, "dashboard", "Dashboard", store.RoleOwner)
	user, err := s.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := s.Tenants(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.Scope{TenantID: tenants[0].ID, UserID: user.ID}
	if _, err := s.CreateProject(ctx, scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, scope.TenantID, scope.UserID, store.TaskInput{ProjectID: "dashboard-project", Title: "Dashboard fixture"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateTaskRun(ctx, store.NewTaskRun{TaskID: task.ID, TenantID: scope.TenantID, StartedBy: scope.UserID, StartKey: "dashboard", ExecutorID: "opencode:fixture", ExecutorName: "OpenCode"})
	if err != nil {
		t.Fatal(err)
	}
	ref := execution.Ref{SessionID: "dashboard-session", TurnID: "dashboard-turn"}
	err = s.CreateExecutionSession(ctx, scope, execution.Session{ID: ref.SessionID, Model: execution.Model{ID: "fixture"}, Executor: &execution.Executor{AgentID: "opencode"}}, execution.Turn{Ref: ref, Caller: execution.Caller{Kind: execution.CallerTask, ID: run.ID, AttemptID: "one"}, Prompt: execution.TextPrompt("synthetic")})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTaskTurn(ctx, run.ID, ref.SessionID, ref.TurnID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginModelRequest(ctx, ref, requestmetrics.Attempt{Protocol: "openai-completions", Usage: requestmetrics.Usage{InputSemantics: "unknown", Availability: "unavailable"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "dashboard.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	routes, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	handler := server.New(s, server.Options{SessionTTL: time.Hour})
	base := "/api/v2/tenants/" + scope.TenantID + "/dashboard/sessions"
	for _, suffix := range []string{"", "/dashboard-session", "/dashboard-session/history", "/dashboard-session/statistics", "/dashboard-session/model-calls"} {
		request := httptest.NewRequest("GET", base+suffix, nil)
		request.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", suffix, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("cacheable analysis")
		}
		route, params, err := routes.FindRoute(request)
		if err != nil {
			t.Fatal(err)
		}
		validation := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, PathParams: params, Route: route}, Status: response.Code, Header: response.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		validation.SetBodyBytes(response.Body.Bytes())
		if err := openapi3filter.ValidateResponse(ctx, validation); err != nil {
			t.Fatalf("%s: %v\n%s", suffix, err, response.Body.String())
		}
	}
	for _, query := range []string{"?status=not-a-status", "?from=broken", "?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "?after=bad", "?unknown=1"} {
		request := httptest.NewRequest("GET", base+query, nil)
		request.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("invalid query %s: %d %s", query, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest("GET", base, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal("unauthenticated read")
	}
	var body any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
}
