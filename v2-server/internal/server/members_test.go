package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

// contractClient sends requests to v2-server and validates every response against Cloud's own
// api/openapi.json, so a contract route cannot drift from the document it implements.
type contractClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	routes routers.Router
}

func newContractClient(t *testing.T, base string) *contractClient {
	t.Helper()
	doc, e := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e := doc.Validate(context.Background()); e != nil {
		t.Fatal(e)
	}
	routes, e := legacy.NewRouter(doc)
	if e != nil {
		t.Fatal(e)
	}
	jar, _ := cookiejar.New(nil)
	return &contractClient{t: t, base: base, http: &http.Client{Jar: jar}, routes: routes}
}

func (c *contractClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, c.base+path, reader)
	if e != nil {
		c.t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := c.http.Do(req)
	if e != nil {
		c.t.Fatal(e)
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(res.Body)
	if e != nil {
		c.t.Fatal(e)
	}
	// The contract pins its own server origin; route the lookup copy through it.
	routeReq := req.Clone(context.Background())
	routeReq.URL.Scheme, routeReq.URL.Host = "http", "localhost:8080"
	route, params, e := c.routes.FindRoute(routeReq)
	if e != nil {
		c.t.Fatalf("%s %s is not in api/openapi.json: %v", method, path, e)
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 res.StatusCode, Header: res.Header,
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	in.SetBodyBytes(raw)
	if e := openapi3filter.ValidateResponse(context.Background(), in); e != nil {
		c.t.Fatalf("%s %s does not match api/openapi.json: %v", method, path, e)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func (c *contractClient) login() {
	c.t.Helper()
	// The Gateway's login routes are not part of Cloud's contract, so they bypass validation.
	res, e := c.http.Post(c.base+"/auth/login", "application/json", bytes.NewReader([]byte("{}")))
	if e != nil {
		c.t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		c.t.Fatalf("login = %d %s", res.StatusCode, raw)
	}
}

func (c *contractClient) tenant() string {
	c.t.Helper()
	status, body := c.do("GET", "/api/v1/me/tenants", nil)
	if status != 200 {
		c.t.Fatalf("tenants = %d %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		c.t.Fatalf("tenants = %v", body)
	}
	return items[0].(map[string]any)["id"].(string)
}

func TestMembersRoute(t *testing.T) {
	f := newSquadServer(t)
	if _, e := f.db.Exec(f.ctx, `INSERT INTO users (source, subject, display_name) VALUES ('test', 'second', '第二人')`); e != nil {
		t.Fatal(e)
	}
	c := newContractClient(t, f.srv.URL)
	c.login()
	tenant := c.tenant()
	if _, e := f.db.Exec(f.ctx, `INSERT INTO memberships (tenant_id, user_id, role)
		SELECT $1, id, 'member' FROM users WHERE subject = 'second'`, tenant); e != nil {
		t.Fatal(e)
	}

	status, body := c.do("GET", "/api/v1/tenants/"+tenant+"/members", nil)
	if status != 200 {
		t.Fatalf("members = %d %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("members = %v", body)
	}
	names := map[string]map[string]any{}
	for _, raw := range items {
		m := raw.(map[string]any)
		names[m["displayName"].(string)] = m
	}
	if names["演示用户"] == nil || names["第二人"] == nil {
		t.Fatalf("members = %v", body)
	}
	if second := names["第二人"]; second["id"] != second["userId"] || second["tenantId"] != tenant || second["version"] != float64(1) {
		t.Fatalf("member identity = %v", second)
	}

	// A caller outside the workspace is refused rather than told the roster is empty.
	if status, body := c.do("GET", "/api/v1/tenants/00000000-0000-0000-0000-000000000001/members", nil); status != 403 || body["code"] != "forbidden" {
		t.Fatalf("foreign members = %d %v", status, body)
	}
	if status, body := c.do("GET", "/api/v1/tenants/not-a-uuid/members", nil); status != 404 || body["code"] != "not_found" {
		t.Fatalf("malformed members = %d %v", status, body)
	}
}
