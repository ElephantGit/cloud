package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

func workflowHTTP(t *testing.T, h http.Handler, token, method, path, key, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	routes, err := contract()
	if err != nil {
		t.Fatal(err)
	}
	route, params, err := routes.FindRoute(req)
	if err != nil {
		t.Fatal(err)
	}
	validation := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	validation.SetBodyBytes(w.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), validation); err != nil {
		t.Fatalf("%s %s: response violates api/openapi.json: %v\n%s", method, path, err, w.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return w.Code, out
}

func TestWorkflowHTTPDocumentLifecycleMatchesExistingContract(t *testing.T) {
	db, ctx := openStore(t), t.Context()
	token, err := db.Login(ctx, store.Identity{Source: "test", Subject: "workflow", DisplayName: "Workflow author"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := db.Tenants(ctx, u.ID)
	if err != nil || len(tenants) != 1 {
		t.Fatalf("tenants=%+v err=%v", tenants, err)
	}
	h := server.New(db, server.Options{})
	base := "/api/v1/tenants/" + tenants[0].ID + "/workflows"
	if status, _ := workflowHTTP(t, h, "", "GET", base, "", ""); status != 401 {
		t.Fatalf("anonymous read: %d", status)
	}
	call := func(method, path, key, body string, want int) map[string]any {
		t.Helper()
		status, out := workflowHTTP(t, h, token, method, path, key, body)
		if status != want {
			t.Fatalf("%s %s: %d want %d: %v", method, path, status, want, out)
		}
		return out
	}
	call("POST", base, "", `{"name":"Draft"}`, 400)
	created := call("POST", base, "create", `{"name":"Draft","graph":{"unfinished":true}}`, 200)
	doc := created["resource"].(map[string]any)
	id := doc["id"].(string)
	path := base + "/" + id
	if replay := call("POST", base, "create", `{"graph":{"unfinished":true},"name":"Draft"}`, 200); !reflect.DeepEqual(replay, created) {
		t.Fatalf("create response not replayed: %v", replay)
	}
	call("POST", base, "create", `{"name":"Changed"}`, 409)
	call("PUT", path, "", `{"name":"Missing version"}`, 428)
	call("PUT", path, "", `{"name":"Stale","version":0}`, 409)
	call("GET", path, "", "", 200)
	invalid := call("POST", path+"/publish", "publish", `{}`, 400)
	if invalid["code"] != "workflow_graph_invalid" || len(invalid["params"].(map[string]any)["problems"].([]any)) == 0 {
		t.Fatalf("missing graph diagnostics: %v", invalid)
	}
	graph := `{"nodes":[{"id":"s","type":"workflow","data":{"kind":"start"}}],"edges":[],"viewport":{"x":100,"y":200}}`
	call("PUT", path, "", `{"name":"Draft","version":1,"graph":`+graph+`}`, 200)
	snapshot := call("POST", path+"/publish", "publish", `{}`, 200)["resource"].(map[string]any)
	snapshotPath := path + "/snapshots/" + snapshot["id"].(string)
	call("GET", snapshotPath, "", "", 200)
	history := call("GET", path+"/snapshots?limit=1", "", "", 200)
	if len(history["items"].([]any)) != 1 || history["nextCursor"] != "" {
		t.Fatal(history)
	}
	call("PUT", path, "", `{"name":"Edited","version":2,"graph":{}}`, 200)
	restored := call("PUT", snapshotPath+"/restore", "", `{"version":3}`, 200)
	if restored["name"] != "Edited" || restored["version"] != float64(4) || !reflect.DeepEqual(restored["graph"], snapshot["graph"]) {
		t.Fatal(restored)
	}
	archived := call("DELETE", path, "delete", `{"version":4}`, 200)
	if replay := call("DELETE", path, "delete", `{"version":4}`, 200); !reflect.DeepEqual(archived, replay) {
		t.Fatal(replay)
	}
	call("GET", path, "", "", 404)
	call("GET", snapshotPath, "", "", 404)
	if page := call("GET", base, "", "", 200); len(page["items"].([]any)) != 0 {
		t.Fatal(page)
	}
}

// The validation probe implements the real Store shape but refuses all calls that
// should have been stopped by the HTTP boundary. It needs no PostgreSQL server.
type workflowValidationProbe struct {
	*store.Store
	denied bool
	calls  int
}

func (p *workflowValidationProbe) SessionUser(context.Context, string) (store.User, error) {
	return store.User{ID: "00000000-0000-4000-8000-000000000002", Status: "active"}, nil
}
func (p *workflowValidationProbe) CheckExecutionScope(context.Context, execution.Scope) error {
	if p.denied {
		return execution.ErrNotFound
	}
	return nil
}
func (p *workflowValidationProbe) CreateWorkflow(context.Context, execution.Scope, string, workflow.CreateDocument) (workflow.Document, error) {
	p.calls++
	return workflow.Document{}, execution.ErrInvalidRequest
}

func TestWorkflowHTTPRejectsMalformedDocumentsAndUnauthorizedRequests(t *testing.T) {
	p := &workflowValidationProbe{}
	h := server.New(p, server.Options{})
	base := "/api/v1/tenants/00000000-0000-4000-8000-000000000001/workflows"
	for _, body := range []string{`null`, `[]`, `{}`, `{"name":null}`, `{"name":"n","description":null}`, `{"name":"n","graph":null}`, `{"name":"n","unknown":1}`, `{"name":"n"}{}`, `{"name":"n","graph":[]}`} {
		t.Run(body, func(t *testing.T) {
			status, _ := workflowHTTP(t, h, "session", "POST", base, "key", body)
			if status != 400 {
				t.Fatalf("accepted invalid body %s: %d", body, status)
			}
		})
	}
	// Syntactically valid but missing names and nonobject graphs reach the domain
	// validator; malformed envelopes and unknown fields must not reach storage.
	if p.calls != 2 {
		t.Fatalf("malformed envelopes reached storage: %d", p.calls)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=bad", "?after=bad", "?limit=2&limit=3"} {
		if status, _ := workflowHTTP(t, h, "session", "GET", base+query, "", ""); status != 400 {
			t.Fatal(query, status)
		}
	}
	p.denied = true
	if status, _ := workflowHTTP(t, h, "session", "POST", base, "key", `{"name":"valid"}`); status != 404 || p.calls != 2 {
		t.Fatalf("unauthorized request reached storage: %d calls=%d", status, p.calls)
	}
}
