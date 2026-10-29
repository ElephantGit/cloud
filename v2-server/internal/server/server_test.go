package server_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// openStore gives the test its own PostgreSQL schema on TEST_DATABASE_URL, dropped afterwards.
func openStore(t *testing.T) *store.Store {
	s, _ := openStoreConfig(t)
	return s
}

func openStoreConfig(t *testing.T) (*store.Store, *pgxpool.Config) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { admin.Close(ctx) })
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "v2test_" + hex.EncodeToString(b[:])
	if _, e := admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s, config
}

// contract is Cloud's api/openapi.json; every response from a route it describes must match it.
var contract = sync.OnceValues(func() (routers.Router, error) {
	doc, e := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi.json"))
	if e != nil {
		return nil, e
	}
	doc.Servers = nil
	return legacy.NewRouter(doc)
})

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T) *client {
	t.Helper()
	srv := httptest.NewServer(server.New(openStore(t), server.Options{
		SessionTTL: time.Hour,
		DemoUser:   store.Identity{Source: "dev", Subject: "demo", DisplayName: "演示用户"},
	}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

// do sends a request and returns status and body, validating the response against the contract
// when the route is part of it (the /auth routes belong to the Gateway and are not).
func (c *client) do(method, path string, body any) (int, map[string]any) {
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
	raw, _ := io.ReadAll(res.Body)
	if !strings.HasPrefix(path, "/auth/") {
		routes, e := contract()
		if e != nil {
			c.t.Fatal(e)
		}
		route, params, e := routes.FindRoute(req)
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
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestHealth(t *testing.T) {
	c := newClient(t)
	if status, body := c.do("GET", "/healthz", nil); status != 200 || body["status"] != "ok" {
		t.Fatalf("healthz = %d %v", status, body)
	}
}

func TestLoginSessionLogout(t *testing.T) {
	c := newClient(t)

	if status, body := c.do("GET", "/api/v1/me", nil); status != 401 || body["code"] != "unauthenticated" {
		t.Fatalf("me before login = %d %v", status, body)
	}
	if status, body := c.do("GET", "/auth/providers", nil); status != 200 || body["default"] != "dev" {
		t.Fatalf("providers = %d %v", status, body)
	}
	status, body := c.do("POST", "/auth/login", map[string]string{"provider": "dev", "returnTo": "/workflows?x=1"})
	if status != 200 || body["authorizationUrl"] != "/workflows?x=1" {
		t.Fatalf("login = %d %v", status, body)
	}
	if status, body := c.do("GET", "/api/v1/me", nil); status != 200 || body["displayName"] != "演示用户" {
		t.Fatalf("me after login = %d %v", status, body)
	}
	status, body = c.do("GET", "/api/v1/me/tenants", nil)
	items, _ := body["items"].([]any)
	if status != 200 || len(items) != 1 || items[0].(map[string]any)["name"] != store.DemoTenant || items[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("tenants = %d %v", status, body)
	}
	// A second login is the same user in the same tenant, not a duplicate.
	c.do("POST", "/auth/login", map[string]string{})
	if _, body := c.do("GET", "/api/v1/me/tenants", nil); len(body["items"].([]any)) != 1 {
		t.Fatalf("tenants after second login = %v", body)
	}
	if status, _ := c.do("POST", "/auth/logout", nil); status != 204 {
		t.Fatalf("logout = %d", status)
	}
	if status, _ := c.do("GET", "/api/v1/me", nil); status != 401 {
		t.Fatalf("me after logout = %d", status)
	}
}

func TestLoginRejectsOtherProvidersAndForeignRedirects(t *testing.T) {
	c := newClient(t)
	if status, body := c.do("POST", "/auth/login", map[string]string{"provider": "github"}); status != 400 || body["code"] != "unknown_provider" {
		t.Fatalf("github login = %d %v", status, body)
	}
	for _, target := range []string{"https://evil.example/", "//evil.example/", "/\\evil.example", "relative"} {
		if _, body := c.do("POST", "/auth/login", map[string]string{"returnTo": target}); body["authorizationUrl"] != "/" {
			t.Fatalf("returnTo %q redirected to %v", target, body["authorizationUrl"])
		}
	}
}

func TestUnknownAPIRouteIsNotFound(t *testing.T) {
	c := newClient(t)
	req, _ := http.NewRequest("GET", c.base+"/api/v1/nope", nil)
	res, e := c.http.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("unknown route = %d", res.StatusCode)
	}
}
