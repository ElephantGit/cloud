package server_test

import (
	"bytes"
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
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestScheduleSharedResourcesAndBindingAuthorization(t *testing.T) {
	db := openStore(t)
	ctx := context.Background()
	token, err := db.Login(ctx, store.Identity{Source: "test", Subject: "resources", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := db.Tenants(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.Scope{TenantID: ts[0].ID, UserID: u.ID}
	project, err := db.CreateProject(ctx, scope, store.Project{ID: "11111111-1111-4111-8111-111111111111", Name: "Authorized project", RepositoryURL: "https://example.invalid/repository", Commit: "pinned-commit", MirrorPath: "/private/mirror"})
	if err != nil {
		t.Fatal(err)
	}
	allowed := true
	model := execution.Model{ID: "model-id", Name: "Real catalog model", BindingVersion: "private-binding"}
	stub := &executiontest.Stub{
		ListModelsFunc: func(_ context.Context, s execution.Scope) ([]execution.Model, error) {
			if s != scope || !allowed {
				return nil, execution.ErrNotFound
			}
			return []execution.Model{model}, nil
		},
		ListExecutorsFunc: func(_ context.Context, s execution.Scope) ([]execution.Executor, error) {
			if s != scope || !allowed {
				return nil, execution.ErrNotFound
			}
			return []execution.Executor{{ID: "executor-id", Name: "Configured executor", AgentName: "Agent", Model: model}}, nil
		},
	}
	h := server.New(db, server.Options{Catalog: stub})
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schedules.openapi.json"))
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
	call := func(handler http.Handler, method, path string, input any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		route, params, err := routes.FindRoute(req)
		if err != nil {
			t.Fatal(err)
		}
		v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: rec.Code, Header: rec.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		v.SetBodyBytes(rec.Body.Bytes())
		if err = openapi3filter.ValidateResponse(ctx, v); err != nil {
			t.Fatalf("contract: %v %s", err, rec.Body)
		}
		return rec
	}
	base := "/api/v2/tenants/" + scope.TenantID
	w := call(h, "GET", base+"/schedule-resources", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Configured executor") {
		t.Fatalf("resources: %d %s", w.Code, w.Body)
	}
	for _, secret := range []string{"private-binding", "/private/mirror", "example.invalid"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("resource projection leaked backend data")
		}
	}
	w = call(server.New(db, server.Options{}), "GET", base+"/schedule-resources", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"models":false`) {
		t.Fatalf("unavailable not distinguished: %d %s", w.Code, w.Body)
	}
	target := map[string]any{"kind": "chat", "prompt": "inspect", "agent": "Agent", "model": "model-id", "executorId": "executor-id"}
	in := map[string]any{"name": "Bound task", "projectId": project.ID, "target": target, "rule": map[string]any{"kind": "hours", "time": "09:00", "days": []int{}, "hours": 1, "timezone": "UTC"}}
	w = call(h, "POST", base+"/schedule-configs", in)
	if w.Code != 201 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	var saved store.ScheduleConfig
	if err = json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	in["version"] = 1
	in["name"] = "Forbidden edit"
	target["model"] = "other-model"
	if w = call(h, "PUT", base+"/schedule-configs/"+saved.ID, in); w.Code != 404 {
		t.Fatalf("mismatched binding: %d", w.Code)
	}
	target["model"] = "model-id"
	allowed = false
	if w = call(h, "PUT", base+"/schedule-configs/"+saved.ID, in); w.Code != 404 {
		t.Fatalf("revoked binding: %d", w.Code)
	}
	items, err := db.ScheduleConfigs(ctx, scope.TenantID, scope.UserID)
	if err != nil || len(items) != 1 || items[0].Name != "Bound task" {
		t.Fatal("revocation overwrote configuration", err)
	}
	allowed = true
	in["projectId"] = "22222222-2222-4222-8222-222222222222"
	if w = call(h, "PUT", base+"/schedule-configs/"+saved.ID, in); w.Code != 404 {
		t.Fatalf("foreign project: %d", w.Code)
	}
	in["projectId"] = ""
	delete(target, "executorId")
	delete(in, "version")
	if w = call(h, "POST", base+"/schedule-configs", in); w.Code != 201 {
		t.Fatalf("model-only: %d %s", w.Code, w.Body)
	}
	target["model"] = "guessed-model"
	if w = call(h, "POST", base+"/schedule-configs", in); w.Code != 404 {
		t.Fatalf("guessed model: %d", w.Code)
	}
}
