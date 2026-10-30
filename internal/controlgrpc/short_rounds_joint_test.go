package controlgrpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/wanglongan587/cloud/internal/core"
)

// TestShortRoundsJointServer serves the real gRPC/Store boundary to the Rust integration test.
// Its HTTP listener stages declared preconditions and observes SQL; it never implements control rules.
func TestShortRoundsJointServer(t *testing.T) {
	metadata := os.Getenv("ORA_SHORT_ROUNDS_JOINT_METADATA")
	if metadata == "" {
		t.Skip("explicit cross-process fixture only")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for the explicit joint fixture")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*config)
	schema := "short_rounds_joint_" + uuid.NewString()[:8]
	if _, err := admin.Exec("CREATE SCHEMA " + pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	t.Cleanup(func() {
		_ = pool.Close()
		_, err := admin.Exec("DROP SCHEMA " + pgx.Identifier{schema}.Sanitize() + " CASCADE")
		if err != nil {
			t.Error(err)
		}
		_ = admin.Close()
	})
	// Like operationWaitDB, apply the actual SQL directly without migration-receipt checks.
	entries, err := os.ReadDir("../core/migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, err := os.ReadFile(filepath.Join("../core/migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
	}
	store := &core.Store{Pool: pool, Signals: core.NewControlHub()}
	core.BindBusinessHooks(store)
	address := os.Getenv("ORA_SHORT_ROUNDS_JOINT_BIND")
	if address == "" {
		address = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewDevelopment(store)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	fixture, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /scene", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Step   string `json:"step"`
			Target bool   `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		scene, err := seedShortRoundsScene(pool, request.Step, request.Target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(scene)
	})
	mux.HandleFunc("GET /operation/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		err := pool.QueryRow(`SELECT jsonb_build_object('id',o.id,'state',o.state,'step',o.step,'reason',o.error_code,
 'attempts',o.node_unavailable_attempts,'version',o.version,'retry_remaining',extract(epoch FROM o.retry_at-clock_timestamp()),
 'idle_reports',(SELECT count(*) FROM node_instances n WHERE n.workspace_id=o.workspace_id AND n.idle_admission_epoch IS NOT NULL),
 'terminated',(SELECT count(*) FROM sandbox_instances s WHERE s.workspace_id=o.workspace_id AND s.terminated_at IS NOT NULL),
 'executions',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',e.execution_id,'node',e.node_id,'input',e.input,'result',e.result,'created_at',e.created_at) ORDER BY e.created_at,e.execution_id) FROM clone_executions e WHERE e.operation_id=o.id),'[]'::jsonb))
 FROM operations o WHERE o.id=$1`, r.PathValue("id")).Scan(&body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST /expire/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Controlled age injection tests durable deadline classification, not a real 30-minute wait.
		result, err := pool.Exec("UPDATE clone_executions SET created_at=clock_timestamp()-interval '1801 seconds' WHERE operation_id=$1 AND result IS NULL", r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			http.Error(w, fmt.Sprintf("want one original execution, got %d", n), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		stopped <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /retire/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Scenario cleanup runs only after the Controller stopped and assertions were captured.
		if _, err := pool.Exec("UPDATE operations SET state='blocked',error_code='joint_fixture_finished' WHERE id=$1 AND state NOT IN ('succeeded','failed')", r.PathValue("id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(fixture) }()
	t.Cleanup(func() { _ = httpServer.Shutdown(context.Background()) })
	body, _ := json.Marshal(map[string]string{"grpc": "http://" + listener.Addr().String(), "fixture": "http://" + fixture.Addr().String(), "schema": schema})
	if err := os.WriteFile(metadata, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("joint fixture ready schema=%s", schema)
	select {
	case <-stopped:
	case <-time.After(15 * time.Minute):
		t.Fatal("joint fixture deadline")
	}
}

// seedShortRoundsScene stages a minimal valid project at the tested step; all later transitions use gRPC.
func seedShortRoundsScene(pool *sql.DB, step string, target bool) (map[string]string, error) {
	id := func() string { return uuid.NewString() }
	user, tenant, space, project, workspace, operation, sandbox, nodeRow := id(), id(), id(), id(), id(), id(), id(), id()
	node := "joint-" + nodeRow[:8]
	binding := id()
	tx, err := pool.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(query string, args ...any) error { _, err := tx.Exec(query, args...); return err }
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users(id,display_name,status) VALUES($1,'joint','active')`, []any{user}},
		{`INSERT INTO tenants(id,name,status) VALUES($1,'joint','active')`, []any{tenant}},
		{`INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES($1,$2,'admin','active')`, []any{tenant, user}},
		{`INSERT INTO collab_workspaces(id,tenant_id,name,slug,created_by) VALUES($1,$2,'joint',$1::uuid::text,$3)`, []any{space, tenant, user}},
		{`INSERT INTO projects(id,tenant_id,owner_user_id,space_id,name,lifecycle) VALUES($1,$2,$3,$4,'joint','active')`, []any{project, tenant, user, space}},
		{`INSERT INTO repository_bindings(id,tenant_id,project_id,repository_url,branch) VALUES($1,$2,$3,'https://example.invalid/joint.git','main')`, []any{binding, tenant, project}},
		{`INSERT INTO workspaces(id,tenant_id,owner_user_id,creator_user_id,creator_evidence,project_id,kind,desired_state,observed_state,runtime_generation,requested_ref) VALUES($1,$2,$3,$3,'verified_request',$4,'main','running','ready',1,'main')`, []any{workspace, tenant, user, project}},
	}
	for _, statement := range statements {
		if err := exec(statement.query, statement.args...); err != nil {
			return nil, err
		}
	}
	request := core.Object{"clone": core.Object{"kind": "clone", "repositoryUrl": "https://git.invalid/snapshot.git", "snapshotRef": "refs/ora/upstreams/" + binding + "/snapshots/0123456789abcdef0123456789abcdef01234567", "baseCommit": "0123456789abcdef0123456789abcdef01234567", "workspaceRef": core.GitWorkspaceRefPrefix(workspace) + "main"}}
	body, _ := json.Marshal(request)
	kind := "create_workspace"
	if step == "quiesce" {
		kind = "stop"
	}
	if err := exec(`INSERT INTO operations(id,tenant_id,actor_user_id,project_id,workspace_id,kind,state,step,request,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,'queued',$7,$8,$1::uuid::text,'fixture')`, operation, tenant, user, project, workspace, kind, step, string(body)); err != nil {
		return nil, err
	}
	if err := exec(`INSERT INTO runtime_controls(workspace_id,state,control_epoch,maintenance_operation_id) VALUES($1,'maintenance',1,$2)`, workspace, operation); err != nil {
		return nil, err
	}
	if target {
		if err := exec(`INSERT INTO sandbox_instances(id,workspace_id,generation,substrate_sandbox_id,observed_state) VALUES($1,$2,1,$1::uuid::text,'running')`, sandbox, workspace); err != nil {
			return nil, err
		}
		if err := exec(`INSERT INTO node_instances(id,workspace_id,sandbox_instance_id,service_subject,connection_state,protocol_version,initialized,node_id,node_incarnation_id) VALUES($1,$2,$3,'node','connected',1,true,$4,'joint-incarnation')`, nodeRow, workspace, sandbox, node); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]string{"operation": operation, "workspace": workspace, "node": node}, nil
}
