package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/wanglongan587/cloud/internal/core"
)

// TestPluginRouteRegistration pins the marketplace routes into the allowlist
// with their exact field whitelists: the router rejects anything not listed
// before the core ever sees it.
func TestPluginRouteRegistration(t *testing.T) {
	found := map[string][]string{}
	for _, route := range Routes() {
		if !strings.Contains(route.Path, "/plugins") {
			continue
		}
		found[route.Method+" "+route.Path] = route.Fields
	}
	want := map[string][]string{
		"GET /api/v1/tenants/:tid/spaces/:spaceId/plugins/catalog": nil,
		"GET /api/v1/tenants/:tid/spaces/:spaceId/plugins/assets":  nil,
		"GET /api/v1/tenants/:tid/spaces/:spaceId/plugins":         nil,
		"POST /api/v1/tenants/:tid/spaces/:spaceId/plugins":        {"identifier", "pluginVersion", "version"},
		"DELETE /api/v1/tenants/:tid/spaces/:spaceId/plugins":      {"identifier", "version"},
	}
	if len(found) != len(want) {
		t.Fatalf("plugin routes = %v", found)
	}
	for route, fields := range want {
		got, ok := found[route]
		if !ok {
			t.Fatalf("route %s missing", route)
		}
		if strings.Join(got, ",") != strings.Join(fields, ",") {
			t.Fatalf("route %s fields = %v, want %v", route, got, fields)
		}
	}
}

// TestPluginFieldValidation pins the boundary types the router accepts for the
// new plugin fields: identifier/pluginVersion are strings, DELETE version is
// an integer, and the plugin effect evidence keys are typed strictly.
func TestPluginFieldValidation(t *testing.T) {
	if !validField("identifier", "official/hello-world") {
		t.Fatal("identifier must accept a canonical plugin id string")
	}
	if validField("identifier", json.Number("1")) {
		t.Fatal("identifier must reject non-string values")
	}
	if !validField("pluginVersion", "1.2.3") {
		t.Fatal("pluginVersion must accept a semver string")
	}
	if validField("pluginVersion", json.Number("1")) {
		t.Fatal("pluginVersion must reject non-string values")
	}
	if !validField("version", json.Number("3")) {
		t.Fatal("version must accept an integer")
	}
	if validField("version", "3") {
		t.Fatal("version must reject strings")
	}
	if !validField("result", map[string]any{"installed": true, "version": "1.2.3", "error": "external_failure", "diagnostic": "sha256 mismatch"}) {
		t.Fatal("plugin effect evidence must be accepted")
	}
	if validField("result", map[string]any{"installed": "yes"}) {
		t.Fatal("installed evidence must be boolean")
	}
	if validField("result", map[string]any{"unknownEvidence": true}) {
		t.Fatal("unknown evidence keys must be rejected")
	}
}

// TestScheduledTaskFieldValidation pins the boundary types of the scheduled-task definition: the
// cadence and the unattended-consent record are the objects the OpenAPI contract and the browser
// send, while a string (the shape the transport used to fall through to) never reaches the core.
func TestScheduledTaskFieldValidation(t *testing.T) {
	number := func(v int64) json.Number { return json.Number(strconv.FormatInt(v, 10)) }
	valid := []struct {
		name  string
		field string
		value any
	}{
		{"daily cadence", "schedule", map[string]any{"kind": "daily", "hour": number(3), "minute": number(5)}},
		{"every-hours cadence", "schedule", map[string]any{"kind": "every_hours", "intervalHours": number(2)}},
		{"weekly cadence", "schedule", map[string]any{"kind": "weekly", "hour": number(9), "minute": number(0), "weekdays": []any{"monday", "friday"}}},
		{"nullable cadence fields", "schedule", map[string]any{"kind": "daily", "hour": number(3), "minute": number(5), "intervalHours": nil, "weekdays": nil}},
		{"documented approval", "approval", map[string]any{"policy": "full_auto_in_cloud_workspace"}},
	}
	for _, test := range valid {
		if !validField(test.field, test.value) {
			t.Fatalf("%s must be accepted", test.name)
		}
	}
	invalid := []struct {
		name  string
		field string
		value any
	}{
		{"schedule as a string", "schedule", `{"kind":"daily","hour":3,"minute":5}`},
		{"unknown schedule key", "schedule", map[string]any{"kind": "daily", "hour": number(3), "minute": number(5), "cron": "* * * * *"}},
		{"hour as a string", "schedule", map[string]any{"kind": "daily", "hour": "3", "minute": number(5)}},
		{"weekdays as a string", "schedule", map[string]any{"kind": "weekly", "hour": number(9), "minute": number(0), "weekdays": "monday"}},
		{"weekday as a number", "schedule", map[string]any{"kind": "weekly", "hour": number(9), "minute": number(0), "weekdays": []any{number(1)}}},
		{"approval as a string", "approval", "full_auto_in_cloud_workspace"},
		{"unknown approval key", "approval", map[string]any{"policy": "full_auto_in_cloud_workspace", "scope": "all"}},
		{"policy as a number", "approval", map[string]any{"policy": number(1)}},
	}
	for _, test := range invalid {
		if validField(test.field, test.value) {
			t.Fatalf("%s must be rejected", test.name)
		}
	}
}

// TestScheduledTaskDocumentedBodyPassesFieldValidation decodes the body the creation form and the
// OpenAPI contract describe and runs every field through the same gate the route applies. It is
// the regression guard for the boundary that once answered 400 invalid_field_type for both object
// fields while every domain test called Store.Public directly.
func TestScheduledTaskDocumentedBodyPassesFieldValidation(t *testing.T) {
	const body = `{"issueId":"c0b8b8e4-6a2f-4f6f-9d6d-0d0b5f0c9a11",
		"agentId":"3f0d0a6e-8f0e-4a53-9a4e-9f1b0d3f5c22","name":"Nightly acceptance",
		"prompt":"write the acceptance report","model":"vendor/model-v1","timezone":"Asia/Shanghai",
		"schedule":{"kind":"daily","hour":3,"minute":5},
		"approval":{"policy":"full_auto_in_cloud_workspace"},"enabled":true}`
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode documented body: %v", err)
	}
	fields := map[string]bool{
		"issueId": true, "name": true, "prompt": true, "agentId": true,
		"model": true, "schedule": true, "timezone": true, "approval": true, "enabled": true,
	}
	if len(decoded) != len(fields) {
		t.Fatalf("documented body fields = %v", decoded)
	}
	for field, value := range decoded {
		if !fields[field] {
			t.Fatalf("documented body carries unexpected field %q", field)
		}
		if !validField(field, value) {
			t.Fatalf("documented field %q was rejected by the transport gate", field)
		}
	}
	patch := `{"version":1,"name":"Renamed","prompt":"new prompt","agentId":"3f0d0a6e-8f0e-4a53-9a4e-9f1b0d3f5c22",
		"model":"vendor/model-v2","schedule":{"kind":"every_hours","intervalHours":2},"timezone":"Asia/Shanghai"}`
	decoder = json.NewDecoder(strings.NewReader(patch))
	decoder.UseNumber()
	var decodedPatch map[string]any
	if err := decoder.Decode(&decodedPatch); err != nil {
		t.Fatalf("decode documented patch: %v", err)
	}
	for field, value := range decodedPatch {
		if !validField(field, value) {
			t.Fatalf("documented patch field %q was rejected by the transport gate", field)
		}
	}
}

func TestUnexpectedPanicIncludesValueAndStackInLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logCore, recorded := observer.New(zap.ErrorLevel)
	engine := New(nil, nil, zap.New(logCore))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer triggers-nil-authenticator")

	engine.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	entries := recorded.FilterMessage("request panic").All()
	if len(entries) != 1 {
		t.Fatalf("expected one panic log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["panic"] == nil || fields["stack"] == "" {
		t.Fatalf("panic log omitted value or stack: %v", fields)
	}
}

// TestNodeSessionQueryAllowlist pins the node session read to the three query parameters the
// decision defines and to exactly one public GET route. A session is addressed by the run, the graph
// node and the attempt only, so neither a session nor an execution identifier is a parameter this
// callers can supply — and the path carries neither.
func TestNodeSessionQueryAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"no query", "", true},
		{"limit", "limit=50", true},
		{"attempt and before", "attempt=2&before=40", true},
		{"limit at the route cap", "limit=" + strconv.Itoa(core.NodeSessionPageMax), true},
		{"session id", "sessionId=abc", false},
		{"execution id", "executionId=abc", false},
		{"forward cursor", "after=3", false},
		{"unknown parameter", "foo=1", false},
		{"repeated parameter", "limit=1&limit=2", false},
		{"empty value", "attempt=", false},
		{"storage path", "path=/tmp/session", false},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, WorkflowNodeSessionPath+"?"+tc.raw, nil)
		if got := validSessionQuery(c, WorkflowNodeSessionPath); got != tc.want {
			t.Fatalf("%s: validSessionQuery(%q) = %v; want %v", tc.name, tc.raw, got, tc.want)
		}
	}

	// The allowlist applies only to session reads: an ordinary route keeps the shared query handling.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/:tid/workflows?foo=1", nil)
	if !validSessionQuery(c, "/api/v1/tenants/:tid/workflows") {
		t.Fatal("an ordinary route was subjected to the session query allowlist")
	}

	// The node session route adds before and attempt to no other session read: the dashboard list
	// rejects before as invalid_pagination (a filter fault) and attempt is not a parameter it has,
	// so both must still fail the allowlist there rather than reaching the core.
	for _, raw := range []string{"before=40", "attempt=2"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, AgentSessionsPath+"?"+raw, nil)
		if validSessionQuery(c, AgentSessionsPath) {
			t.Fatalf("dashboard list accepted %q; before and attempt belong to the node session read only", raw)
		}
	}

	routes := 0
	for _, route := range Routes() {
		if route.Path != WorkflowNodeSessionPath {
			continue
		}
		routes++
		if route.Method != http.MethodGet || route.Action != "" {
			t.Fatalf("node session route = %s %s action %q; want one public GET", route.Method, route.Path, route.Action)
		}
		for _, segment := range []string{"sessionId", "executionId", "attempt"} {
			if strings.Contains(route.Path, ":"+segment) {
				t.Fatalf("node session path carries :%s; the attempt, session and execution must not be addressable", segment)
			}
		}
	}
	if routes != 1 {
		t.Fatalf("%d routes registered for %s; want exactly one", routes, WorkflowNodeSessionPath)
	}
}
