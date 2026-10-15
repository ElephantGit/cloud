package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/wanglongan587/cloud/internal/api/router"
	"github.com/wanglongan587/cloud/internal/core"
	"github.com/wanglongan587/cloud/internal/simulator"
)

// productionServer is the cmd/server composition as a running server: a real core.NewStore over an
// isolated PostgreSQL schema, the production branch of configureCollaboration (development fixtures
// off), and the production router. Requests carry the same gateway + user credentials the
// integration harness uses, so the test drives the shipped HTTP contract instead of a stripped
// substitute. Only the store wiring, the schema, and the credentials are test-owned.
type productionServer struct {
	t      *testing.T
	store  *core.Store
	client *simulator.Client
	tid    string
	uid    string
	space  string
	user   core.Claims
}

// newProductionServer builds that server, or skips when no PostgreSQL is configured (REQUIRE_POSTGRES=1
// turns the missing database into a failure so the gate can never pass by skipping).
func newProductionServer(t *testing.T) *productionServer {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_POSTGRES") == "1" {
			t.Fatal("TEST_DATABASE_URL is required; the production directory test must not skip under REQUIRE_POSTGRES")
		}
		t.Skip("real PostgreSQL: set TEST_DATABASE_URL")
	}
	log := zap.NewNop()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin := stdlib.OpenDB(*config)
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping admin: %v", err)
	}
	schema := "server_directory_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	t.Cleanup(func() {
		_ = pool.Close()
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	// The composition under test: a real NewStore first, then the production branch of the gate.
	store, err := core.NewStore(db)
	if err != nil {
		t.Fatalf("core.NewStore: %v", err)
	}
	configureCollaboration(store, false, log)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	credentials, err := simulator.NewCredentials()
	if err != nil {
		t.Fatalf("simulator credentials: %v", err)
	}
	auth, err := core.NewAuthenticator("ora-cloud", credentials.Trust)
	if err != nil {
		t.Fatalf("authenticator: %v", err)
	}
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(router.New(store, auth, log))
	t.Cleanup(server.Close)
	bootstrap, err := store.Bootstrap(context.Background(), "Production tenant", "corp", "alice", "Alice")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return &productionServer{
		t:      t,
		store:  store,
		client: &simulator.Client{URL: server.URL, Credentials: credentials, HTTP: &http.Client{Timeout: 10 * time.Second}, Subject: "gateway-a"},
		tid:    bootstrap.S("tenantId"),
		uid:    bootstrap.S("userId"),
		space:  bootstrap.S("spaceId"),
		user:   core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "alice"}, Source: "corp", DisplayName: "Alice"},
	}
}

// claimsFor signs the user token for a tenant member bootstrapped alongside the first one.
func (s *productionServer) claimsFor(subject, display string) core.Claims {
	return core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: subject}, Source: "corp", DisplayName: display}
}

// call drives one authenticated request as the tenant's member and fails on any status but want.
func (s *productionServer) call(method, path string, body core.Object, key string, want int) core.Object {
	s.t.Helper()
	return s.callAs(&s.user, method, path, body, key, want)
}

// callAs is call for an explicit caller: the second tenant's member, or a plain path outside `path`.
func (s *productionServer) callAs(user *core.Claims, method, path string, body core.Object, key string, want int) core.Object {
	s.t.Helper()
	out, status, err := s.client.Call(context.Background(), method, path, "gateway", core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "gateway-a"}}, user, key, body)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	if status != want {
		s.t.Fatalf("%s %s: want %d got %d %v", method, path, want, status, out)
	}
	return out
}

func (s *productionServer) path(p string) string { return "/api/v1/tenants/" + s.tid + p }

// seedAgent inserts a space_agents roster row directly: the marketplace install lifecycle is not what
// this test covers, and IssueRun D1 makes the roster row the whole agent target.
func (s *productionServer) seedAgent(display, pluginID, status string) string {
	s.t.Helper()
	return s.seedAgentIn(s.space, s.tid, display, pluginID, status)
}

// seedAgentIn is seedAgent against an explicit space and tenant, so a case can plant a roster row in
// another tenant and assert the directory never serves it.
func (s *productionServer) seedAgentIn(space, tid, display, pluginID, status string) string {
	s.t.Helper()
	id := uuid.NewString()
	if _, err := s.store.Pool.Exec(`INSERT INTO space_agents(id, space_id, tenant_id, plugin_id, display_name, status) VALUES($1,$2,$3,$4,$5,$6)`,
		id, space, tid, pluginID, display, status); err != nil {
		s.t.Fatalf("seed space_agents: %v", err)
	}
	return id
}

// publishWorkflow creates and publishes a Start-only workflow — the shape #65 reproduced the missing
// @ target with. Returns the workflow id.
func (s *productionServer) publishWorkflow(name, key string) string {
	s.t.Helper()
	wid := s.call("POST", s.path("/workflows"), core.Object{"name": name, "description": name, "graph": startOnlyGraph()}, key, 200).O("resource").S("id")
	s.call("POST", s.path("/workflows/"+wid+"/publish"), core.Object{"name": name + " v1"}, key+"-publish", 200)
	return wid
}

// startOnlyGraph is a valid Start-only graph the way the editor saves one: one Start node declaring
// the workflow's input form, no edges.
func startOnlyGraph() core.Object {
	return core.Object{
		"nodes": []any{core.Object{
			"id": "start-1", "type": "workflow", "deletable": false,
			"position": core.Object{"x": 0, "y": 0},
			"data": core.Object{
				"kind": "start", "title": "开始", "description": "",
				"inputVariables": []any{core.Object{
					"name": "topic", "displayName": "Topic", "fieldType": "text-input", "valueType": "string", "required": true,
				}},
			},
		}},
		"edges":    []any{},
		"viewport": core.Object{"x": 0, "y": 0, "zoom": 1},
	}
}

// targetItems unwraps the `items` array of a list response.
func targetItems(o core.Object) []core.Object {
	raw, _ := o["items"].([]any)
	out := make([]core.Object, 0, len(raw))
	for _, v := range raw {
		out = append(out, core.Object(v.(map[string]any)))
	}
	return out
}

// TestConfigureCollaborationProductionServesAgentsAndWorkflows is #65's direct evidence. With the
// development fixtures off and a real pool, the composition gate must serve BOTH real target types —
// the roster-backed Space Agent and the database-backed workflow — through discovery, search and
// resolution, while archived workflows, retired agents and other tenants stay invisible. Replacing
// the directory with the agent-only one (the reported regression) fails the workflow half; dropping
// the roster fails the agent half.
func TestConfigureCollaborationProductionServesAgentsAndWorkflows(t *testing.T) {
	s := newProductionServer(t)
	active := s.seedAgent("Hello World", "official/hello-world", "active")
	retired := s.seedAgent("Retired Agent", "official/retired", "retired")
	triage := s.publishWorkflow("Incident Triage", "triage")
	archived := s.publishWorkflow("Archived Review", "archived")
	s.call("DELETE", s.path("/workflows/"+archived), core.Object{"version": 1}, "archived-delete", 200)

	// A second tenant with a live workflow of its own: same store, different tenant, never visible here.
	other, err := s.store.Bootstrap(context.Background(), "Other tenant", "corp", "bob", "Bob")
	if err != nil {
		t.Fatalf("bootstrap other tenant: %v", err)
	}
	bob := s.claimsFor("bob", "Bob")
	foreign := s.callAs(&bob, "POST", "/api/v1/tenants/"+other.S("tenantId")+"/workflows",
		core.Object{"name": "Foreign Triage", "description": "other tenant", "graph": startOnlyGraph()}, "foreign", 200).O("resource").S("id")

	// Discovery: one HTTP read must carry the active agent and the live workflow, with the frozen
	// descriptors — task mode for the agent, form mode whose formRef is the workflow id.
	items := targetItems(s.call("GET", s.path("/collaboration/targets"), nil, "", 200))
	byID := map[string]core.Object{}
	for _, it := range items {
		byID[it.S("id")] = it
	}
	agent, ok := byID[active]
	if !ok || agent.S("type") != "agent" || agent.S("displayName") != "Hello World" || agent.O("interactionDescriptor").S("mode") != "task" {
		t.Fatalf("active Space Agent must be discoverable: %v", items)
	}
	wf, ok := byID[triage]
	if !ok || wf.S("type") != "workflow" || wf.S("displayName") != "Incident Triage" || wf.O("interactionDescriptor").S("mode") != "form" || wf.O("interactionDescriptor").S("formRef") != triage {
		t.Fatalf("published workflow must be discoverable as a Form target: %v", items)
	}
	for _, hidden := range []string{retired, archived, foreign} {
		if _, ok := byID[hidden]; ok {
			t.Fatalf("target %s must not be discoverable: %v", hidden, items)
		}
	}

	// Search by workflow name finds the live workflow and still hides archived/foreign rows.
	hits := targetItems(s.call("GET", s.path("/collaboration/targets")+"?q=Triage", nil, "", 200))
	found := map[string]bool{}
	for _, hit := range hits {
		found[hit.S("id")] = true
	}
	if !found[triage] || found[archived] || found[foreign] || found[retired] {
		t.Fatalf("workflow name search wrong: %v", hits)
	}

	// #126: the same request must filter the roster-backed agent too. "Hello World" does not match
	// `Triage`, so a search that found the workflow must not answer with the agent; before the fix the
	// unfiltered agent came back here next to the filtered members and workflows.
	if found[active] {
		t.Fatalf("agent search must narrow the roster like the other types: %v", hits)
	}
	agentHits := targetItems(s.call("GET", s.path("/collaboration/targets")+"?q=Hello", nil, "", 200))
	if len(agentHits) != 1 || agentHits[0].S("id") != active {
		t.Fatalf("agent name search must return the matching roster row alone: %v", agentHits)
	}

	// Resolution through the shipped HTTP path: a workflow @ records a configured-but-unconfirmed
	// Form interaction and executes nothing.
	iid := s.call("POST", s.path("/issues"), core.Object{"title": "Directory issue"}, "issue", 200).O("resource").S("id")
	s.call("POST", s.path("/issues/"+iid+"/comments"), core.Object{"body": "x", "targets": []any{core.Object{"type": "workflow", "id": triage}}}, "wf-comment", 200)
	interactions := targetItems(s.call("GET", s.path("/issues/"+iid+"/interactions"), nil, "", 200))
	if len(interactions) != 1 || interactions[0].S("targetId") != triage || interactions[0].S("mode") != "form" || interactions[0]["runId"] != nil {
		t.Fatalf("workflow @ must record exactly one unconfirmed Form interaction: %v", interactions)
	}

	// The active agent still resolves. The issue carries no project, so the run is refused by the
	// project gate (409) — a 404 here would mean the roster's agent was lost, which is what this
	// assertion separates.
	s.call("POST", s.path("/issues/"+iid+"/comments"), core.Object{"body": "x", "targets": []any{core.Object{"type": "agent", "id": active, "task": "t"}}}, "agent-comment", 409)

	// Archived, foreign and retired targets resolve to nothing at all.
	s.call("POST", s.path("/issues/"+iid+"/comments"), core.Object{"body": "x", "targets": []any{core.Object{"type": "workflow", "id": archived}}}, "archived-comment", 404)
	s.call("POST", s.path("/issues/"+iid+"/comments"), core.Object{"body": "x", "targets": []any{core.Object{"type": "workflow", "id": foreign}}}, "foreign-comment", 404)
	s.call("POST", s.path("/issues/"+iid+"/comments"), core.Object{"body": "x", "targets": []any{core.Object{"type": "agent", "id": retired, "task": "t"}}}, "retired-comment", 404)
}

// TestProductionDirectorySearchFiltersAgentsLikeOtherTargets is #126's direct evidence: one search
// request must narrow the roster-backed Space Agent exactly the way it narrows the members and
// workflows it answers with. Before the fix SpaceAgentDirectory dropped `q`, so a query for one agent
// by name returned every active agent while the rest of the same response was filtered — the
// "unmatched agent survived the search" failure these cases assert against. Every case drives the
// shipped HTTP path, so the filtering is observed where a client observes it.
func TestProductionDirectorySearchFiltersAgentsLikeOtherTargets(t *testing.T) {
	s := newProductionServer(t)
	backend := s.seedAgent("Backend Builder", "official/backend", "active")
	frontend := s.seedAgent("Frontend Builder", "official/frontend", "active")
	// Retired, and sharing the word the searches below use: a query for that word must return the
	// active row alone, so a dropped status filter cannot hide behind the new name filter.
	retired := s.seedAgent("Backend Retired", "official/backend-retired", "retired")
	triage := s.publishWorkflow("Incident Triage", "triage")

	// A second tenant over the same store with a roster row of its own: the tenant scope of the read
	// must survive the added filter.
	other, err := s.store.Bootstrap(context.Background(), "Other tenant", "corp", "bob", "Bob")
	if err != nil {
		t.Fatalf("bootstrap other tenant: %v", err)
	}
	foreign := s.seedAgentIn(other.S("spaceId"), other.S("tenantId"), "Backend Foreign", "official/foreign", "active")

	// search returns the response's target ids in wire order: the tenant's members first, then the
	// directory chain (roster agents, then workflows).
	search := func(rawQuery string) []string {
		s.t.Helper()
		items := targetItems(s.call("GET", s.path("/collaboration/targets")+rawQuery, nil, "", 200))
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.S("id"))
		}
		return ids
	}

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty q lists every active target", "", []string{s.uid, backend, frontend, triage}},
		{"whitespace-only q lists every active target", "?q=%20%20", []string{s.uid, backend, frontend, triage}},
		{"agent name substring narrows the roster", "?q=Frontend", []string{frontend}},
		{"agent name match ignores case", "?q=bAcKeNd", []string{backend}},
		{"member name narrows the members", "?q=Alice", []string{s.uid}},
		{"workflow name still narrows the workflows", "?q=Triage", []string{triage}},
		{"a literal % matches no name", "?q=%25", nil},
		{"a literal _ matches no name", "?q=_", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := search(c.query)
			if !slices.Equal(got, c.want) {
				t.Fatalf("GET /collaboration/targets%s: want %v got %v", c.query, c.want, got)
			}
			for _, hidden := range []string{retired, foreign} {
				if slices.Contains(got, hidden) {
					t.Fatalf("GET /collaboration/targets%s must never expose %s: %v", c.query, hidden, got)
				}
			}
		})
	}

	// The handler still owns q validation, and the directory only ever sees an already-trimmed,
	// length-checked term: an over-long query is refused before any directory is consulted.
	s.call("GET", s.path("/collaboration/targets")+"?q="+strings.Repeat("a", 201), nil, "", 400)
}
