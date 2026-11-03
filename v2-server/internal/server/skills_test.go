package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

type skillClient struct {
	t      *testing.T
	base   string
	http   *http.Client
	routes routers.Router
}

func newSkillClient(t *testing.T, base string) *skillClient {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "skills.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	routes, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	return &skillClient{t: t, base: base, http: &http.Client{Jar: jar}, routes: routes}
}

func (c *skillClient) login(subject string) {
	c.t.Helper()
	body, _ := json.Marshal(map[string]string{"provider": "email", "email": storetest.Email(subject)})
	res, err := c.http.Post(c.base+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		c.t.Fatalf("login = %d %s", res.StatusCode, raw)
	}
}

func (c *skillClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	routeReq := req.Clone(context.Background())
	routeReq.URL.Scheme, routeReq.URL.Host = "http", "localhost:8080"
	route, params, err := c.routes.FindRoute(routeReq)
	if err != nil {
		c.t.Fatalf("%s %s is not in skills.openapi.json: %v", method, path, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 res.StatusCode, Header: res.Header,
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	input.SetBodyBytes(raw)
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		c.t.Fatalf("%s %s does not match skills.openapi.json: %v", method, path, err)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func (c *skillClient) tenant() string {
	c.t.Helper()
	res, err := c.http.Get(c.base + "/api/v1/me/tenants")
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		c.t.Fatalf("tenants = %d", res.StatusCode)
	}
	body := map[string]any{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		c.t.Fatal(err)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		c.t.Fatalf("tenants = %v", body)
	}
	return items[0].(map[string]any)["id"].(string)
}

func TestSkillsHTTP(t *testing.T) {
	f := newSquadServer(t)
	c := newSkillClient(t, f.srv.URL)
	c.login("squad-demo")
	tenant := c.tenant()
	base := "/api/v2/tenants/" + tenant + "/skills"

	status, body := c.do("GET", base, nil)
	if status != 200 || len(body["items"].([]any)) != 3 {
		t.Fatalf("initial skills = %d %v", status, body)
	}
	status, body = c.do("POST", base, map[string]any{
		"name": "review", "description": "检查改动", "content": "# Review",
	})
	if status != 201 {
		t.Fatalf("create = %d %v", status, body)
	}
	id := body["id"].(string)
	if body["name"] != "review" || body["version"] != float64(1) {
		t.Fatalf("created = %v", body)
	}
	status, body = c.do("POST", base, map[string]any{
		"name": "REVIEW", "description": "重复", "content": "",
	})
	if status != 409 || body["code"] != "skill_name_conflict" {
		t.Fatalf("duplicate = %d %v", status, body)
	}
	status, body = c.do("PUT", base+"/"+id, map[string]any{
		"name": "review", "description": "更新说明", "content": "# Review v2",
	})
	if status != 200 || body["version"] != float64(2) || body["description"] != "更新说明" {
		t.Fatalf("update = %d %v", status, body)
	}
	status, body = c.do("GET", base+"/"+id, nil)
	if status != 200 || body["content"] != "# Review v2" {
		t.Fatalf("get = %d %v", status, body)
	}

	imported := "---\nname: incident-runbook\ndescription: 事故处理手册\n---\n\n# Runbook\n"
	status, body = c.do("POST", base+"/import", map[string]any{
		"fileName": "SKILL.md", "content": imported,
	})
	if status != 201 || body["name"] != "incident-runbook" || body["content"] != "# Runbook\n" {
		t.Fatalf("import = %d %v", status, body)
	}
	conflict := "---\nname: review\ndescription: 覆盖\n---\n\n# Replacement\n"
	status, body = c.do("POST", base+"/import", map[string]any{
		"fileName": "SKILL.md", "content": conflict,
	})
	if status != 409 || body["code"] != "skill_name_conflict" {
		t.Fatalf("import conflict = %d %v", status, body)
	}
	status, body = c.do("POST", base+"/import", map[string]any{
		"fileName": "SKILL.md", "content": conflict, "overwrite": true,
	})
	if status != 201 || body["id"] != id || body["version"] != float64(3) {
		t.Fatalf("import overwrite = %d %v", status, body)
	}
	status, body = c.do("POST", base+"/import", map[string]any{
		"fileName": "SKILL.md", "content": "# no front matter",
	})
	if status != 400 || body["code"] != "skill_front_matter_missing" {
		t.Fatalf("invalid import = %d %v", status, body)
	}

	other := newSkillClient(t, f.srv.URL)
	storetest.SignIn(t, f.store, "skills-http-other", "other", "member")
	other.login("skills-http-other")
	status, body = other.do("GET", base, nil)
	if status != 200 || len(body["items"].([]any)) != 3 {
		t.Fatalf("other user's skills = %d %v", status, body)
	}
	for _, raw := range body["items"].([]any) {
		if raw.(map[string]any)["name"] == "review" {
			t.Fatalf("other user sees the owner's skill: %v", raw)
		}
	}

	status, _ = c.do("DELETE", base+"/"+id, nil)
	if status != 204 {
		t.Fatalf("delete = %d", status)
	}
	status, body = c.do("GET", base+"/"+id, nil)
	if status != 404 || body["code"] != "skill_not_found" {
		t.Fatalf("deleted = %d %v", status, body)
	}
}

func TestSkillsHTTPRejectsInvalidPayloads(t *testing.T) {
	f := newSquadServer(t)
	c := newSkillClient(t, f.srv.URL)
	c.login("squad-demo")
	base := "/api/v2/tenants/" + c.tenant() + "/skills"
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"bad name", map[string]any{"name": "../review", "description": "x", "content": ""}, "skill_name_invalid"},
		{"missing description", map[string]any{"name": "review", "description": "", "content": ""}, "skill_description_missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, body := c.do("POST", base, test.body)
			if status != 400 || body["code"] != test.code {
				t.Fatalf("status = %d body = %v", status, body)
			}
		})
	}
	status, body := c.do("POST", base+"/import", map[string]any{
		"fileName": "SKILL.txt", "content": "---\nname: review\ndescription: x\n---\n",
	})
	if status != 400 || body["code"] != "skill_file_type" {
		t.Fatalf("file type = %d %v", status, body)
	}
	status, body = c.do("POST", base, map[string]any{
		"name": "review", "description": "x", "content": strings.Repeat("a", 256<<10),
		"extra": true,
	})
	if status != 400 || body["code"] != "invalid_json" {
		t.Fatalf("unknown field = %d %v", status, body)
	}
}
