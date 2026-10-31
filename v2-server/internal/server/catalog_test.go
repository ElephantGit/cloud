package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestExecutionCatalogHTTP(t *testing.T) {
	s := openStore(t)
	runtime, err := executionruntime.New(t.Context(), executionruntime.Config{Store: s, Models: map[string]executionruntime.ModelChoice{
		"fixture": {Model: execution.Model{ID: "fixture", Name: "Fixture", BindingVersion: "fixture-v1"}, Binding: nodeservices.Binding{Model: nodeservices.Model{ID: "private-upstream-model", Name: "Private configuration", MaxTokens: 1024}, Provider: nodeservices.FixtureProvider{}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	options := server.Options{SessionTTL: time.Hour, DemoUser: store.Identity{Source: "dev", Subject: "catalog-owner", DisplayName: "Owner"}, Catalog: runtime}
	srv := httptest.NewServer(server.New(s, options))
	defer func() { srv.Close() }()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	doc, err := openapi3.NewLoader().LoadFromFile("catalog.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatal(err)
	}
	routes, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, input any) (int, map[string]any) {
		t.Helper()
		body, _ := json.Marshal(input)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(path, "/api/v2/") {
			route, params, err := routes.FindRoute(req)
			if err != nil {
				t.Fatal(err)
			}
			v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: res.StatusCode, Header: res.Header, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
			v.SetBodyBytes(raw)
			if err := openapi3filter.ValidateResponse(t.Context(), v); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			if strings.Contains(string(raw), "private-upstream-model") || strings.Contains(string(raw), "Private configuration") {
				t.Fatal("provider binding leaked into catalog")
			}
		}
		out := map[string]any{}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, out
	}
	const unknownTenant = "11111111-1111-4111-8111-111111111111"
	for _, kind := range []string{"models", "executors"} {
		if status, _ := call("GET", "/api/v2/tenants/"+unknownTenant+"/execution/"+kind, nil); status != 401 {
			t.Fatalf("anonymous %s=%d", kind, status)
		}
	}
	call("POST", "/auth/login", map[string]string{})
	_, tenants := call("GET", "/api/v1/me/tenants", nil)
	tenant := tenants["items"].([]any)[0].(map[string]any)["id"].(string)
	base := "/api/v2/tenants/" + tenant + "/execution/"
	if status, out := call("GET", base+"models", nil); status != 200 || !reflect.DeepEqual(out, map[string]any{"items": []any{map[string]any{"id": "fixture", "name": "Fixture", "bindingVersion": "fixture-v1"}}}) {
		t.Fatalf("models=%d %v", status, out)
	}
	if status, out := call("GET", base+"executors", nil); status != 200 || !reflect.DeepEqual(out, map[string]any{"items": []any{}}) {
		t.Fatalf("unconfigured Agent executors=%d %v", status, out)
	}
	for _, kind := range []string{"models", "executors"} {
		for _, tid := range []string{unknownTenant, "invalid"} {
			if status, out := call("GET", "/api/v2/tenants/"+tid+"/execution/"+kind, nil); status != 404 || out["code"] != "not_found" {
				t.Fatalf("tenant %s %s=%d %v", tid, kind, status, out)
			}
		}
	}
	srv.Close()
	options.Catalog = nil
	srv = httptest.NewServer(server.New(s, options))
	call("POST", "/auth/login", map[string]string{})
	for _, kind := range []string{"models", "executors"} {
		if status, out := call("GET", base+kind, nil); status != 503 || out["code"] != "execution_catalog_unavailable" {
			t.Fatalf("nil catalog %s=%d %v", kind, status, out)
		}
	}
}
