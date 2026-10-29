package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleConfigurationLifecycle(t *testing.T) {
	s := openStore(t)
	srv := httptest.NewServer(server.New(s, server.Options{SessionTTL: time.Hour, DemoUser: store.Identity{Source: "dev", Subject: "schedule-owner", DisplayName: "Owner"}}))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	doc, e := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schedules.openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = doc.Validate(context.Background()); e != nil {
		t.Fatal(e)
	}
	routes, e := legacy.NewRouter(doc)
	if e != nil {
		t.Fatal(e)
	}
	call := func(method, path string, input any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(input)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		res, e := c.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if len(path) > 7 && path[:7] == "/api/v2" {
			route, params, e := routes.FindRoute(req)
			if e != nil {
				t.Fatal(e)
			}
			v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: res.StatusCode, Header: res.Header, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
			v.SetBodyBytes(raw)
			if e = openapi3filter.ValidateResponse(context.Background(), v); e != nil {
				t.Fatal(e)
			}
		}
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}
	call("POST", "/auth/login", map[string]string{})
	_, tenants := call("GET", "/api/v1/me/tenants", nil)
	tid := tenants["items"].([]any)[0].(map[string]any)["id"].(string)
	path := "/api/v2/tenants/" + tid + "/schedule-configs"
	input := map[string]any{"name": "每日简报", "projectId": "", "target": map[string]any{"kind": "chat", "prompt": "总结提供的内容", "agent": "", "model": ""}, "rule": map[string]any{"kind": "daily", "time": "09:00", "days": []int{1}, "hours": 6, "timezone": "Asia/Shanghai"}}
	status, created := call("POST", path, input)
	if status != 201 {
		t.Fatalf("create %d %v", status, created)
	}
	id := created["id"].(string)
	input["version"] = 1
	input["name"] = "编辑后的简报"
	status, updated := call("PUT", path+"/"+id, input)
	if status != 200 || updated["version"] != float64(2) {
		t.Fatalf("edit %d %v", status, updated)
	}
	if status, _ = call("PUT", path+"/"+id, input); status != 409 {
		t.Fatalf("stale update = %d", status)
	}
	status, paused := call("PATCH", path+"/"+id+"/enabled", map[string]any{"enabled": false, "version": 2})
	if status != 200 || paused["enabled"] != false {
		t.Fatalf("pause %d %v", status, paused)
	}
	// Recreate HTTP service against the same database: configuration survives server replacement.
	srv.Close()
	srv = httptest.NewServer(server.New(s, server.Options{SessionTTL: time.Hour}))
	defer srv.Close()
	token, e := s.Login(context.Background(), store.Identity{Source: "dev", Subject: "schedule-owner", DisplayName: "Owner"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	jar.SetCookies(req.URL, []*http.Cookie{{Name: server.SessionCookie, Value: token}})
	status, list := call("GET", path, nil)
	items := list["items"].([]any)
	if status != 200 || len(items) != 1 || items[0].(map[string]any)["name"] != "编辑后的简报" || items[0].(map[string]any)["enabled"] != false {
		t.Fatalf("persisted = %d %v", status, list)
	}
	status, resumed := call("PATCH", path+"/"+id+"/enabled", map[string]any{"enabled": true, "version": 3})
	if status != 200 || resumed["enabled"] != true {
		t.Fatalf("resume %d %v", status, resumed)
	}
	// A second member of the same tenant cannot read or mutate the owner's configuration.
	token, e = s.Login(context.Background(), store.Identity{Source: "dev", Subject: "other", DisplayName: "Other"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	jar.SetCookies(req.URL, []*http.Cookie{{Name: server.SessionCookie, Value: token}})
	_, list = call("GET", path, nil)
	if len(list["items"].([]any)) != 0 {
		t.Fatal("cross-user read")
	}
	if status, _ = call("PATCH", path+"/"+id+"/enabled", map[string]any{"enabled": false, "version": 4}); status != 404 {
		t.Fatalf("cross-user update %d", status)
	}
	if status, _ = call("GET", "/api/v2/tenants/00000000-0000-0000-0000-000000000001/schedule-configs", nil); status != 404 {
		t.Fatalf("cross-tenant list %d", status)
	}
	delete(input, "version")
	input["userId"] = "forged"
	if status, _ = call("POST", path, input); status != 400 {
		t.Fatalf("forged identity %d", status)
	}
	delete(input, "userId")
	input["rule"].(map[string]any)["timezone"] = "not/a/zone"
	if status, _ = call("POST", path, input); status != 400 {
		t.Fatalf("invalid timezone %d", status)
	}
	jar.SetCookies(req.URL, []*http.Cookie{{Name: server.SessionCookie, Value: "invalid"}})
	if status, _ = call("GET", path, nil); status != 401 {
		t.Fatalf("unauthenticated list %d", status)
	}
}
