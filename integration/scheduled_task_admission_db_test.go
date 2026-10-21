package integration

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// These DB tests are the minimal direct verification the review asked for (reviewer blocking item
// #8). They drive the shipped worker and PATCH writer the way production does: EvaluateScheduledTasksOnce
// and Store.Public, against a real PostgreSQL schema. NOT_VERIFIED locally: this environment has no
// network to fetch the integration test dependencies (pgx), so the suite only runs in CI.

const scheduledTaskModelID = "vendor/model-v1"

// seedScheduledTaskModelChain gives the creator a usable personal default backed by an enabled,
// credential-bound connection, so creatorModelReady and bindRunModel both resolve.
func seedScheduledTaskModelChain(t *testing.T, store *core.Store, userID string) {
	t.Helper()
	conn := uuid.NewString()
	cred := uuid.NewString()
	_, err := store.Pool.Exec(`INSERT INTO personal_model_credentials(id,user_id,ciphertext,key_id)
		VALUES($1,$2,repeat('a',64),'scheduled-task-test')`, cred, userID)
	must(t, err)
	_, err = store.Pool.Exec(`INSERT INTO personal_model_connections(id,user_id,credential_id,name,protocol,base_url,auth_mode,models,enabled)
		VALUES($1,$2,$3,'Personal Provider','openai-completions','https://models.example.invalid/v1','bearer',$4::jsonb,true)`,
		conn, userID, cred, `[{"id":"`+scheduledTaskModelID+`","name":"Model V1","contextWindow":128000,"maxTokens":8192}]`)
	must(t, err)
	_, err = store.Pool.Exec(`INSERT INTO personal_model_defaults(user_id,connection_id,model_id,version) VALUES($1,$2,$3,1)`, userID, conn, scheduledTaskModelID)
	must(t, err)
}

// seedEnabledScheduledTask inserts an enabled every_hours task due `enabledAgoMinutes` ago.
func seedEnabledScheduledTask(t *testing.T, store *core.Store, s skeletonSeed, agentID, name string, enabledAgoMinutes int) string {
	t.Helper()
	taskID := uuid.NewString()
	_, err := store.Pool.Exec(`INSERT INTO scheduled_tasks
		(id,tenant_id,issue_id,creator_user_id,agent_id,name,prompt,schedule,timezone,revision,version,enabled,enabled_at,unattended_policy,unattended_consented_at)
		VALUES($1,$2,$3,$4,$5,$6,'Sync','{"kind":"every_hours","intervalHours":1}'::jsonb,'Asia/Shanghai',1,1,true,now() - $7 * interval '1 minute','full_auto_in_cloud_workspace',now())`,
		taskID, s.tenantID, s.issueID, s.userID, agentID, name, enabledAgoMinutes)
	must(t, err)
	return taskID
}

// scheduledTaskStore builds a migrated store with the issue bound to the project and the agent's
// model chain in place, ready for an admission.
func scheduledTaskStore(t *testing.T) (*core.Store, skeletonSeed, string) {
	t.Helper()
	pool, _ := testSchema(t, "test_scheduled_")
	store := newStoreOnSchema(t, pool)
	must(t, store.Migrate(context.Background()))
	s := seedAgentIssueRunSkeleton(t, pool)
	_, err := pool.Exec(`UPDATE issues SET project_ref=$1 WHERE id=$2`, s.projectID, s.issueID)
	must(t, err)
	agentID := seedScheduledTaskAgent(t, pool, s)
	seedScheduledTaskModelChain(t, store, s.userID)
	return store, s, agentID
}

func occurrenceCount(t *testing.T, store *core.Store, taskID string) (int, string) {
	t.Helper()
	var count int
	var outcome string
	err := store.Pool.QueryRow(`SELECT count(*), COALESCE(min(outcome),'') FROM scheduled_task_occurrences WHERE task_id=$1`, taskID).Scan(&count, &outcome)
	must(t, err)
	return count, outcome
}

// TestScheduledTaskAdmissionCreatesARun (FIX-1): the admission names the task UUID as the run's
// trigger evidence and creates an IssueRun.
func TestScheduledTaskAdmissionCreatesARun(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Admission", 65)
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))
	// A duplicate scan for the same due point must not create a second occurrence or IssueRun.
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))

	count, outcome := occurrenceCount(t, store, taskID)
	if count != 1 || outcome != "admitted" {
		t.Fatalf("want exactly one admitted occurrence, got %d with outcome %q", count, outcome)
	}
	var runID, ref string
	err := store.Pool.QueryRow(`SELECT r.id, r.trigger_evidence_ref_id::text FROM scheduled_task_occurrences o JOIN issue_runs r ON r.id=o.run_id WHERE o.task_id=$1`, taskID).Scan(&runID, &ref)
	must(t, err)
	if runID == "" || ref != taskID {
		t.Fatalf("admission must name the task UUID as trigger evidence, got run %q ref %q", runID, ref)
	}
}

// TestScheduledTaskMissedRecoverySingleOccurrence (FIX-5): a long outage writes exactly one missed.
func TestScheduledTaskMissedRecoverySingleOccurrence(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Missed recovery", 150)
	// Two passes: the second must not write another missed point.
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))

	count, outcome := occurrenceCount(t, store, taskID)
	if count != 1 || outcome != "missed" {
		t.Fatalf("outage must leave one missed occurrence, got %d with outcome %q", count, outcome)
	}
}

// TestScheduledTaskSingleFailureDoesNotBlock (FIX-4): a task whose agent is gone must not starve a
// healthy task in the same pass.
func TestScheduledTaskSingleFailureDoesNotBlock(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	badAgentID := seedScheduledTaskAgent(t, store.Pool, s)
	_, err := store.Pool.Exec(`UPDATE space_agents SET status='retired' WHERE id=$1`, badAgentID)
	must(t, err)
	badTask := seedEnabledScheduledTask(t, store, s, badAgentID, "Retired agent", 65)
	goodTask := seedEnabledScheduledTask(t, store, s, agentID, "Healthy agent", 65)
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))

	badCount, badOutcome := occurrenceCount(t, store, badTask)
	goodCount, goodOutcome := occurrenceCount(t, store, goodTask)
	if badCount != 1 || badOutcome != "failed" {
		t.Fatalf("bad task must record one failed occurrence, got %d %q", badCount, badOutcome)
	}
	if goodCount != 1 || goodOutcome != "admitted" {
		t.Fatalf("healthy task must admit despite the bad task, got %d %q", goodCount, goodOutcome)
	}
}

// TestScheduledTaskEvaluationProcessesMoreThanOnePage prevents a fixed first 100-task page from
// starving later enabled definitions forever.
func TestScheduledTaskEvaluationProcessesMoreThanOnePage(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	const taskCount = 101 // worker page size is 100; retain an explicit overflow case here.
	for i := 0; i < taskCount; i++ {
		seedEnabledScheduledTask(t, store, s, agentID, "Paged "+uuid.NewString(), 65)
	}
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))
	var count int
	must(t, store.Pool.QueryRow(`SELECT count(*) FROM scheduled_task_occurrences`).Scan(&count))
	if count != taskCount {
		t.Fatalf("evaluation must visit every enabled task, got %d", count)
	}
}

// TestScheduledTaskUpdateAppliesPatch (FIX-3): the PATCH writer's WHERE clause binds the right
// tenant, creator and version so a legal rename succeeds and bumps the version.
func TestScheduledTaskUpdateAppliesPatch(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Patch", 65)
	caller := scheduledTaskCaller(t, store, s.tenantID)

	out, status, err := store.Public(context.Background(), &core.PublicRequest{
		Method:          "PATCH",
		Key:             "patch-1",
		Path:            "/api/v1/tenants/" + s.tenantID + "/scheduled-tasks/" + taskID,
		TenantID:        s.tenantID,
		ScheduledTaskID: taskID,
		Body:            core.Object{"name": "Renamed"},
		Identity:        caller,
	})
	must(t, err)
	if status != 200 || out.O("resource").S("name") != "Renamed" {
		t.Fatalf("legal rename must succeed, got %d %v", status, out)
	}
	var version int
	must(t, store.Pool.QueryRow(`SELECT version FROM scheduled_tasks WHERE id=$1`, taskID).Scan(&version))
	if version != 2 {
		t.Fatalf("rename must bump version, got %d", version)
	}
}

// scheduledTaskCaller binds the seed's creator as a verified identity for the public PATCH path.
func scheduledTaskCaller(t *testing.T, store *core.Store, tenantID string) *core.Claims {
	t.Helper()
	var userID string
	must(t, store.Pool.QueryRow(`SELECT user_id FROM tenant_memberships WHERE tenant_id=$1 AND role='admin' AND status='active' ORDER BY user_id LIMIT 1`, tenantID).Scan(&userID))
	subject := "scheduled-" + userID[:8]
	_, err := store.Pool.Exec(`INSERT INTO user_identities(user_id,source,subject) VALUES($1,'test-scheduled',$2)`, userID, subject)
	must(t, err)
	return &core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: subject}, Source: "test-scheduled", Caller: "gateway", DisplayName: "Scheduled Caller"}
}
