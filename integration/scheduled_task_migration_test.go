package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

const scheduledTaskMigration = "0060_scheduled_tasks.sql"

// seedScheduledTaskAgent inserts the one space_agents row a definition binds, returning its id.
func seedScheduledTaskAgent(t *testing.T, pool *sql.DB, s skeletonSeed) string {
	t.Helper()
	agentID := uuid.NewString()
	_, err := pool.Exec(`INSERT INTO space_agents(id, space_id, tenant_id, plugin_id, display_name, status)
		VALUES($1, $2, $3, 'official/ora-space.opencode', 'Codex', 'active')`, agentID, s.spaceID, s.tenantID)
	must(t, err)
	return agentID
}

// scheduledTaskColumns is every required definition column in the seeded order below.
const scheduledTaskColumns = "id, tenant_id, issue_id, creator_user_id, agent_id, name, prompt, schedule, timezone, revision, version"

func insertScheduledTask(t *testing.T, pool *sql.DB, s skeletonSeed, agentID, tail string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,'x','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1)`+tail, id, s.tenantID, s.issueID, s.userID, agentID)
	must(t, err)
	return id
}

func TestMigration0060ScheduledTasksFreshSchemaAndConstraints(t *testing.T) {
	_, pool := skeletonStore(t)
	s := seedAgentIssueRunSkeleton(t, pool)
	agentID := seedScheduledTaskAgent(t, pool, s)
	taskID := insertScheduledTask(t, pool, s, agentID, "")

	for _, table := range []string{"scheduled_tasks", "scheduled_task_occurrences"} {
		var n int
		must(t, pool.QueryRow(`SELECT count(*) FROM information_schema.tables
			WHERE table_schema=current_schema() AND table_name=$1`, table).Scan(&n))
		if n != 1 {
			t.Fatalf("table %s missing", table)
		}
	}
	for _, index := range []string{
		"scheduled_tasks_tenant_list", "scheduled_tasks_due",
		"scheduled_task_occurrences_run", "scheduled_task_deadlines_due",
	} {
		var n int
		must(t, pool.QueryRow(`SELECT count(*) FROM pg_indexes
			WHERE schemaname=current_schema() AND indexname=$1`, index).Scan(&n))
		if n != 1 {
			t.Fatalf("index %s missing", index)
		}
	}

	// The idempotent admission identity is (task_id, scheduled_at): a repeated point is refused.
	first := `INSERT INTO scheduled_task_occurrences
		(tenant_id, task_id, scheduled_at, outcome, task_revision, run_id, observed_at)
		VALUES($1,$2,'2026-01-02T03:04:05Z','admitted',1,$3,now())`
	_, err := pool.Exec(first, s.tenantID, taskID, s.runID)
	must(t, err)
	_, err = pool.Exec(`INSERT INTO scheduled_task_occurrences
		(tenant_id, task_id, scheduled_at, outcome, task_revision, observed_at)
		VALUES($1,$2,'2026-01-02T03:04:05Z','missed',1,now())`, s.tenantID, taskID)
	wantPGError(t, err, "23505")

	occurrence := func(fields, values, code string) {
		t.Helper()
		_, e := pool.Exec(`INSERT INTO scheduled_task_occurrences
			(tenant_id, task_id, scheduled_at, `+fields+`)
			VALUES($1,$2,'2026-01-03T00:00:00Z',`+values+`)`, s.tenantID, taskID)
		wantPGError(t, e, code)
	}
	occurrence("outcome, task_revision, observed_at", "'paused',1,now()", "23514")
	occurrence("outcome, failure_reason, task_revision, observed_at", "'failed','quota_exceeded',1,now()", "23514")

	// Definition shape guards: a complete insert that breaks exactly one guard each.
	guard := func(tail, code string) {
		t.Helper()
		_, e := pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,`+tail+`)`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
		wantPGError(t, e, code)
	}
	guard(`'','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1`, "23514") // empty name
	guard(`'x','','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1`, "23514") // empty prompt
	guard(`'x','x','[]'::jsonb,'Asia/Shanghai',1,1`, "23514")                                      // array schedule
	guard(`'x','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'',1,1`, "23514")             // empty timezone

	_, e := pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`,enabled) VALUES($1,$2,$3,$4,$5,'enabled-no-consent','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1,true)`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
	wantPGError(t, e, "23514")
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`,enabled,unattended_policy,unattended_consented_at) VALUES($1,$2,$3,$4,$5,'enabled-no-instant','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1,true,'full_auto_in_cloud_workspace',now())`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
	wantPGError(t, e, "23514")

	// Foreign agent and issue must hit the FK, not the (tenant, issue, creator, name) unique key.
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,'foreign-agent','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1)`, uuid.NewString(), s.tenantID, s.issueID, s.userID, uuid.NewString())
	wantPGError(t, e, "23503") // foreign agent
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,'foreign-issue','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1)`, uuid.NewString(), s.tenantID, uuid.NewString(), s.userID, agentID)
	wantPGError(t, e, "23503") // foreign issue

	// One definition per (tenant, issue, creator, name).
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,'Daily sync','Sync','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1)`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
	must(t, e)
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`) VALUES($1,$2,$3,$4,$5,'Daily sync','Sync','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1)`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
	wantPGError(t, e, "23505")

	// A disabled definition may omit consent and enabled_at and carries its own reason.
	_, e = pool.Exec(`INSERT INTO scheduled_tasks (`+scheduledTaskColumns+`,disabled_reason) VALUES($1,$2,$3,$4,$5,'Off','x','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1,'manual')`, uuid.NewString(), s.tenantID, s.issueID, s.userID, agentID)
	must(t, e)
}

func TestMigration0060ScheduledTasksUpgradeAndReplay(t *testing.T) {
	pool, _ := testSchema(t, "test_scheduled59u_")
	applyMigrationsUpTo(t, pool, migrationsBefore(t, scheduledTaskMigration))
	s := seedAgentIssueRunSkeleton(t, pool)
	before := scheduledMigrationBusinessSnapshot(t, pool)

	store := newStoreOnSchema(t, pool)
	must(t, store.Migrate(context.Background()))
	var tables int
	must(t, pool.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema=current_schema() AND table_name IN ('scheduled_tasks','scheduled_task_occurrences')`).Scan(&tables))
	if tables != 2 {
		t.Fatalf("upgrade added %d scheduled-task tables, want 2", tables)
	}
	if after := scheduledMigrationBusinessSnapshot(t, pool); before != after {
		t.Fatal("scheduled-task migration rewrote unrelated business facts")
	}

	agentID := seedScheduledTaskAgent(t, pool, s)
	taskID := insertScheduledTask(t, pool, s, agentID, "")
	_, err := pool.Exec(`INSERT INTO scheduled_task_occurrences
		(tenant_id, task_id, scheduled_at, outcome, task_revision, observed_at)
		VALUES($1,$2,'2026-01-02T03:04:05Z','skipped_busy',1,now())`, s.tenantID, taskID)
	must(t, err)

	// Migrate again plus a raw DDL re-execution: both are no-ops and preserve progress.
	must(t, store.Migrate(context.Background()))
	ddl, err := os.ReadFile(filepath.Join("..", "internal", "core", "migrations", scheduledTaskMigration))
	must(t, err)
	tx, err := pool.Begin()
	must(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(string(ddl))
	must(t, err)
	must(t, tx.Commit())
	var definitions, occurrences int
	must(t, pool.QueryRow(`SELECT count(*) FROM scheduled_tasks`).Scan(&definitions))
	must(t, pool.QueryRow(`SELECT count(*) FROM scheduled_task_occurrences`).Scan(&occurrences))
	if definitions != 1 || occurrences != 1 {
		t.Fatalf("replay left %d definitions / %d occurrences, want 1/1", definitions, occurrences)
	}
}

// scheduledMigrationBusinessSnapshot captures the business tables a definition references so the
// upgrade path can prove it rewrote none of them.
func scheduledMigrationBusinessSnapshot(t *testing.T, pool *sql.DB) string {
	t.Helper()
	var snapshot string
	must(t, pool.QueryRow(`SELECT jsonb_build_object(
		'users',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM users r),
		'tenants',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM tenants r),
		'issues',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM issues r),
		'space_agents',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM space_agents r),
		'issue_runs',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM issue_runs r)
		)::text`).Scan(&snapshot))
	return snapshot
}
