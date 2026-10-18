package core

// Earlier Agent prose arrives as an explicit Node projection; Cloud never parses ACP content or
// guesses whether a tool/thought record is user-facing text. This reader freezes only completed
// runs and never treats a prior Workspace or Git Revision as proof that chat history was restored.

const (
	taskConversationHistoryTurns = 20
	taskConversationPromptBudget = 64 << 10
)

func conversationHistory(t *transaction, input Object, tid, iid string) {
	rows := t.list(`
		WITH recent AS (
		  SELECT te.run_id,te.seq,te.turn_id,te.visible_text,te.visible_text_truncated,te.created_at
		  FROM issue_runs r JOIN thread_entries te ON te.run_id=r.id
		  WHERE r.tenant_id=$1 AND r.issue_id=$2 AND r.executor_type='agent'
		    AND r.phase='done' AND r.deleted_at IS NULL AND te.visible_text IS NOT NULL
		  ORDER BY te.created_at DESC,te.run_id DESC,te.seq DESC LIMIT 500
		)
		SELECT run_id,turn_id::text AS turn_id,string_agg(visible_text,'' ORDER BY seq) AS assistant_text,
		       bool_or(visible_text_truncated) AS truncated, max(created_at) AS observed_at
		FROM recent GROUP BY run_id,turn_id ORDER BY observed_at DESC,run_id DESC LIMIT $3`,
		tid, iid, taskConversationHistoryTurns)
	// A past session with no projection is a known gap, including pre-upgrade rows. It remains
	// readable through its Thread/verified archive, but cannot be described as recovered context.
	unavailable := t.one(`SELECT 1 FROM issue_runs r WHERE r.tenant_id=$1 AND r.issue_id=$2 AND r.executor_type='agent'
		AND r.phase='done' AND r.deleted_at IS NULL AND EXISTS (SELECT 1 FROM thread_entries te WHERE te.run_id=r.id AND te.source='node')
		AND NOT EXISTS (SELECT 1 FROM thread_entries te WHERE te.run_id=r.id AND te.visible_text IS NOT NULL) LIMIT 1`, tid, iid) != nil
	chosen := make([]Object, 0, len(rows))
	truncated := len(rows) == taskConversationHistoryTurns
	for _, row := range rows {
		user := t.one(`SELECT m.body FROM task_conversation_messages m
			JOIN thread_entries te ON te.run_id=m.run_id AND te.seq=COALESCE(m.thread_seq,1)
			WHERE m.run_id=$1 AND m.state='submitted' AND te.turn_id::text=$2
			ORDER BY m.seq DESC LIMIT 1`, row.S("runId"), row.S("turnId"))
		turn := Object{"id": row.S("runId") + ":" + row.S("turnId"), "assistant": row.S("assistantText")}
		if user != nil {
			turn["user"] = user.S("body")
		}
		if row.B("truncated") {
			turn["assistantTruncated"] = true
		}
		candidate := append([]Object{turn}, chosen...)
		probe := make(Object, len(input)+1)
		for k, value := range input {
			probe[k] = value
		}
		probe["conversationHistory"] = candidate
		// Leave room for a frozen Revision reference added only at Session Start. JSON escaping is
		// measured by the real prompt renderer rather than guessing from plain-text byte counts.
		if len(renderAgentInitialTurn(probe, nil)) > taskConversationPromptBudget-1024 {
			truncated = true
			break
		}
		chosen = candidate
	}
	if len(chosen) != 0 {
		input["conversationHistory"] = chosen
	}
	if truncated || unavailable {
		input["conversationHistoryIncomplete"] = true
	}
}
