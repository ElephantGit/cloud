package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// seedPausedTaskConversation pauses the issue's task conversation on a finished run whose work was
// saved, the state the owner leaves with an explicit resume. It returns that run.
func seedPausedTaskConversation(t *testing.T, store *core.Store, s skeletonSeed, agentID string) string {
	t.Helper()
	var paused string
	must(t, store.Pool.QueryRow(`INSERT INTO issue_runs(id, tenant_id, issue_id, executor_type, executor_id, status, phase, result)
		VALUES(gen_random_uuid(),$1,$2,'agent',$3,'failed','done','{"deliveryState":"saved"}') RETURNING id`, s.tenantID, s.issueID, agentID).Scan(&paused))
	_, err := store.Pool.Exec(`INSERT INTO task_conversations(issue_id, tenant_id, paused_run_id) VALUES($1,$2,$3)`, s.issueID, s.tenantID, paused)
	must(t, err)
	return paused
}

// conversationPost drives one task conversation endpoint the way the Web client does and requires
// it to succeed.
func conversationPost(t *testing.T, store *core.Store, s skeletonSeed, caller *core.Claims, suffix, messageID string, body core.Object) {
	t.Helper()
	_, _, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "POST", Key: uuid.NewString(),
		Path:     "/api/v1/tenants/" + s.tenantID + "/issues/" + s.issueID + "/conversation" + suffix,
		TenantID: s.tenantID, IssueID: s.issueID, ConversationMessageID: messageID,
		Identity: caller, Body: body,
	})
	must(t, err)
}

// TestScheduledTaskAdmitsTheNextDuePointAfterConversationRecovery walks refused → recovered →
// admitted through the shipped recovery endpoints. Recovery only reopens the gate for later due
// points: the refused one keeps its failure and never gets a run of its own, and a second pass after
// the admission adds nothing.
func TestScheduledTaskAdmitsTheNextDuePointAfterConversationRecovery(t *testing.T) {
	cases := []struct {
		name    string
		refuse  func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) (recoverConversation func(caller *core.Claims))
		refusal string
	}{
		{
			name:    "paused conversation resumed",
			refusal: "task_paused",
			refuse: func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) func(*core.Claims) {
				paused := seedPausedTaskConversation(t, store, s, agentID)
				return func(caller *core.Claims) {
					conversationPost(t, store, s, caller, "/resume", "", core.Object{"runId": paused})
				}
			},
		},
		{
			name:    "failed message cancelled",
			refusal: "task_paused",
			refuse: func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) func(*core.Claims) {
				seedTaskConversationMessage(t, store, s, agentID, "failed")
				var messageID string
				must(t, store.Pool.QueryRow(`SELECT id FROM task_conversation_messages WHERE issue_id=$1 AND seq=1`, s.issueID).Scan(&messageID))
				return func(caller *core.Claims) {
					conversationPost(t, store, s, caller, "/messages/"+messageID+"/cancel", messageID, core.Object{})
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, s, agentID := scheduledTaskStore(t)
			recoverConversation := tc.refuse(t, store, s, agentID)
			taskID := seedEnabledScheduledTask(t, store, s, agentID, "Recovered conversation", 65)
			assertConversationRejects(t, store, s.issueID, taskID, tc.refusal)

			recoverConversation(scheduledTaskCaller(t, store, s.tenantID))
			// Move the refused ledger point back one interval so the worker evaluates the next due point,
			// as an hour passing would.
			_, err := store.Pool.Exec(`UPDATE scheduled_task_occurrences SET scheduled_at=scheduled_at - interval '1 hour' WHERE task_id=$1`, taskID)
			must(t, err)
			must(t, store.EvaluateScheduledTasksOnce(context.Background()))
			must(t, store.EvaluateScheduledTasksOnce(context.Background()))

			rows, err := store.Pool.Query(`SELECT outcome, COALESCE(failure_reason,''), COALESCE(run_id::text,'')
				FROM scheduled_task_occurrences WHERE task_id=$1 ORDER BY scheduled_at`, taskID)
			must(t, err)
			defer func() { _ = rows.Close() }()
			type occurrence struct{ outcome, reason, runID string }
			var got []occurrence
			for rows.Next() {
				var o occurrence
				must(t, rows.Scan(&o.outcome, &o.reason, &o.runID))
				got = append(got, o)
			}
			must(t, rows.Err())
			if len(got) != 2 || got[0] != (occurrence{"failed", tc.refusal, ""}) || got[1].outcome != "admitted" || got[1].runID == "" {
				t.Fatalf("after recovery the refused point must stay failed/%s without a run and only the next point admit, got %+v", tc.refusal, got)
			}
			var issueRuns int
			must(t, store.Pool.QueryRow(`SELECT count(*) FROM issue_runs WHERE trigger_evidence_ref_id=$1`, taskID).Scan(&issueRuns))
			if issueRuns != 1 {
				t.Fatalf("recovery must admit exactly one scheduled IssueRun and never replay the refused point, got %d", issueRuns)
			}
		})
	}
}

// TestScheduledAdmissionAndDirectInputAgreeOnTheConversationGate drives both entries on one
// conversation state: a direct agent run request (the 409 the Web client shows) and the scheduled
// worker. They must refuse the same states with the same reason and accept the same states, so a
// refusal condition added to only one entry fails here.
func TestScheduledAdmissionAndDirectInputAgreeOnTheConversationGate(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, store *core.Store, s skeletonSeed, agentID string)
		// refusal is the shared reason code; empty means both entries accept.
		refusal string
	}{
		{"no conversation", func(*testing.T, *core.Store, skeletonSeed, string) {}, ""},
		{"settled conversation", func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) {
			seedTaskConversationMessage(t, store, s, agentID, "cancelled")
		}, ""},
		{"paused conversation", func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) {
			seedPausedTaskConversation(t, store, s, agentID)
		}, "task_paused"},
		{"failed message", func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) {
			seedTaskConversationMessage(t, store, s, agentID, "failed")
		}, "task_paused"},
		{"waiting message", func(t *testing.T, store *core.Store, s skeletonSeed, agentID string) {
			seedTaskConversationMessage(t, store, s, agentID, "waiting")
		}, "task_handoff_pending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, s, agentID := scheduledTaskStore(t)
			tc.seed(t, store, s, agentID)
			taskID := seedEnabledScheduledTask(t, store, s, agentID, "Gate agreement", 65)
			before := taskConversationSnapshot(t, store, s.issueID)

			out, _, err := store.Public(context.Background(), &core.PublicRequest{
				Method: "POST", Key: uuid.NewString(),
				Path:     "/api/v1/tenants/" + s.tenantID + "/issues/" + s.issueID + "/runs",
				TenantID: s.tenantID, IssueID: s.issueID, Identity: scheduledTaskCaller(t, store, s.tenantID),
				Body: core.Object{"executorType": "agent", "executorId": agentID},
			})
			if tc.refusal == "" {
				must(t, err)
				direct := out.O("resource").S("id")
				if direct == "" {
					t.Fatalf("direct input must be accepted, got %v", out)
				}
				// Settle the accepted direct run: the pending-executor dedup, not the conversation gate,
				// would otherwise refuse the scheduled input for the same Agent.
				_, err = store.Pool.Exec(`UPDATE issue_runs SET status='completed', phase='done' WHERE id=$1`, direct)
				must(t, err)
				must(t, store.EvaluateScheduledTasksOnce(context.Background()))
				if count, outcome := occurrenceCount(t, store, taskID); count != 1 || outcome != "admitted" {
					t.Fatalf("a state direct input accepts must also admit the scheduled input, got %d %q", count, outcome)
				}
				return
			}
			if fault := core.ErrorCode(err); fault.Status != 409 || fault.Code != tc.refusal {
				t.Fatalf("direct input must refuse with 409 %s, got %v", tc.refusal, err)
			}
			if after := taskConversationSnapshot(t, store, s.issueID); after != before {
				t.Fatalf("a refused direct input rewrote the task conversation:\nbefore %s\nafter  %s", before, after)
			}
			assertConversationRejects(t, store, s.issueID, taskID, tc.refusal)
		})
	}
}
