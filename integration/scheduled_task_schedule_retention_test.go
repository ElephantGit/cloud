package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// seedDailyScheduledTask seeds an enabled daily 09:00 task in the given IANA timezone.
func seedDailyScheduledTask(t *testing.T, store *core.Store, s skeletonSeed, agentID, name, timezone string, enabledAgoDays int) string {
	t.Helper()
	taskID := uuid.NewString()
	_, err := store.Pool.Exec(`INSERT INTO scheduled_tasks
		(id,tenant_id,issue_id,creator_user_id,agent_id,name,prompt,schedule,timezone,revision,version,enabled,enabled_at,unattended_policy,unattended_consented_at)
		VALUES($1,$2,$3,$4,$5,$6,'Sync','{"kind":"daily","hour":9,"minute":0}'::jsonb,$7,1,1,true,now() - $8 * interval '1 day','full_auto_in_cloud_workspace',now())`,
		taskID, s.tenantID, s.issueID, s.userID, agentID, name, timezone, enabledAgoDays)
	must(t, err)
	return taskID
}

// TestScheduledTaskDisabledIntervalIsNotEvaluated pins D2 停用区间不评估、不补记：停用期间被 worker 跳过，
// 重新启用时 re-anchor enabled_at，因此被吞掉的到期点永远不会回填成 missed。
func TestScheduledTaskDisabledIntervalIsNotEvaluated(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Disabled interval", 150)
	_, err := store.Pool.Exec(`UPDATE scheduled_tasks SET enabled=false, enabled_at=NULL WHERE id=$1`, taskID)
	must(t, err)

	must(t, store.EvaluateScheduledTasksOnce(context.Background()))
	if count, _ := occurrenceCount(t, store, taskID); count != 0 {
		t.Fatalf("a disabled task must evaluate to no occurrence, got %d", count)
	}

	_, err = store.Pool.Exec(`UPDATE scheduled_tasks SET enabled=true, enabled_at=now() WHERE id=$1`, taskID)
	must(t, err)
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))
	if count, outcome := occurrenceCount(t, store, taskID); count != 0 {
		t.Fatalf("re-enable must not back-fill missed for the disabled window, got %d %q", count, outcome)
	}
}

// TestScheduledTaskDailyScheduleUsesTaskTimezone pins D2 时区语义：worker 从任务行加载 IANA 时区，daily 到期点
// 落在该时区的 09:00，而不是连接/服务器的本地时区。
func TestScheduledTaskDailyScheduleUsesTaskTimezone(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedDailyScheduledTask(t, store, s, agentID, "Daily tz", "Asia/Shanghai", 2)
	must(t, store.EvaluateScheduledTasksOnce(context.Background()))

	var scheduledAt time.Time
	must(t, store.Pool.QueryRow(`SELECT scheduled_at FROM scheduled_task_occurrences WHERE task_id=$1 ORDER BY scheduled_at DESC LIMIT 1`, taskID).Scan(&scheduledAt))
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	must(t, err)
	local := scheduledAt.In(shanghai)
	if local.Hour() != 9 || local.Minute() != 0 {
		t.Fatalf("daily due point must be 09:00 in the task timezone, got %s", local.Format("15:04"))
	}
}

// TestScheduledTaskHistoryRetentionPrunesBeyond100 pins D6 100 条保留：第 101 个 occurrence 写入后在同一事务
// 从最旧的非活跃记录开始清理，任务侧记录保持有界、不影响新接纳。
func TestScheduledTaskHistoryRetentionPrunesBeyond100(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Retention", 65)
	_, err := store.Pool.Exec(`INSERT INTO scheduled_task_occurrences
		(tenant_id, task_id, scheduled_at, outcome, task_revision, observed_at)
		SELECT $1, $2, now() - (g * interval '1 hour'), 'missed', 1, now()
		FROM generate_series(1, 100) AS g`, s.tenantID, taskID)
	must(t, err)

	var before int
	must(t, store.Pool.QueryRow(`SELECT count(*) FROM scheduled_task_occurrences WHERE task_id=$1`, taskID).Scan(&before))
	if before != 100 {
		t.Fatalf("seed must leave exactly 100 occurrences, got %d", before)
	}

	must(t, store.EvaluateScheduledTasksOnce(context.Background()))

	var total, admitted int
	must(t, store.Pool.QueryRow(`SELECT count(*), count(*) FILTER (WHERE outcome='admitted') FROM scheduled_task_occurrences WHERE task_id=$1`, taskID).Scan(&total, &admitted))
	if total != 100 || admitted != 1 {
		t.Fatalf("retention must keep 100 occurrences and admit exactly one, got total=%d admitted=%d", total, admitted)
	}
}
