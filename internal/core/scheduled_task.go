package core

import (
	"encoding/json"
	"fmt"
	"time"

	// Embedded IANA timezone data: the scheduler evaluates schedules in the task's own
	// timezone and must not depend on the host's zone database being present or current.
	_ "time/tzdata"
)

// Cloud 首期 Scheduled Task 的领域规则（specs decisions/cloud/scheduled-task/0-…，approved）。
// 本文承载 D1/D2/D5 的纯函数部分：schedule 的解析与下一次到期点求值（任务时区、DST 语义）、
// 10 分钟迟到阈值、停用区间不评估。接纳事务、截止写入与公开 API 在同包的其余文件里。

// maxScheduledOccurrenceLateness 是 ADR D2 冻结的迟到阈值：观察晚于 scheduled_at 超过该值的到期点
// 记 missed，不执行也不补跑。固定值，不提供配置。
const maxScheduledOccurrenceLateness = 10 * time.Minute

// scheduledTaskDeadline 是 ADR D4 冻结的运行截止：接纳时刻 + 60 分钟，不可配置。
const scheduledTaskDeadline = 60 * time.Minute

// unattendedPolicyCloud 是首版唯一的无人值守策略：在一次性隔离运行 Workspace 中全自动批准工具
// 调用。策略名与同意时间随任务持久化；撤销同意自动停用任务。
const unattendedPolicyCloud = "full_auto_in_cloud_workspace"

// scheduledTaskSchedule 是三种受限周期之一的解析结果（ADR D1：不存储或解释 cron 字符串）。
type scheduledTaskSchedule struct {
	Kind          string
	IntervalHours int
	Hour          int
	Minute        int
	Weekdays      []time.Weekday
}

// parseScheduledTaskSchedule 校验并解析 schedule jsonb。三种形状之外的一切输入都被拒绝，
// 使调度器永远不需要解释 cron 字符串或猜测缺省字段。
func parseScheduledTaskSchedule(raw any) (scheduledTaskSchedule, error) {
	body, err := json.Marshal(raw)
	if err != nil {
		return scheduledTaskSchedule{}, err
	}
	var shape struct {
		Kind          string   `json:"kind"`
		IntervalHours int      `json:"intervalHours"`
		Hour          int      `json:"hour"`
		Minute        int      `json:"minute"`
		Weekdays      []string `json:"weekdays"`
	}
	if err := json.Unmarshal(body, &shape); err != nil {
		return scheduledTaskSchedule{}, err
	}
	switch shape.Kind {
	case "every_hours":
		if shape.IntervalHours < 1 {
			return scheduledTaskSchedule{}, fmt.Errorf("every_hours interval must be at least 1 hour")
		}
		return scheduledTaskSchedule{Kind: shape.Kind, IntervalHours: shape.IntervalHours}, nil
	case "daily":
		if shape.Hour < 0 || shape.Hour > 23 || shape.Minute < 0 || shape.Minute > 59 {
			return scheduledTaskSchedule{}, fmt.Errorf("daily schedule needs hour 0-23 and minute 0-59")
		}
		return scheduledTaskSchedule{Kind: shape.Kind, Hour: shape.Hour, Minute: shape.Minute}, nil
	case "weekly":
		if shape.Hour < 0 || shape.Hour > 23 || shape.Minute < 0 || shape.Minute > 59 || len(shape.Weekdays) == 0 {
			return scheduledTaskSchedule{}, fmt.Errorf("weekly schedule needs weekdays and a wall-clock time")
		}
		var weekdays []time.Weekday
		for _, name := range shape.Weekdays {
			day, ok := scheduledWeekdayByName[name]
			if !ok {
				return scheduledTaskSchedule{}, fmt.Errorf("unknown weekday %q", name)
			}
			weekdays = append(weekdays, day)
		}
		return scheduledTaskSchedule{Kind: shape.Kind, Hour: shape.Hour, Minute: shape.Minute, Weekdays: weekdays}, nil
	default:
		return scheduledTaskSchedule{}, fmt.Errorf("schedule kind must be every_hours, daily or weekly")
	}
}

var scheduledWeekdayByName = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
	"wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday,
	"saturday": time.Saturday,
}

// nextDue returns the next scheduled point strictly after `after`, evaluated in the task's own
// timezone (ADR D2: DST 跳过的不补跑——time.Date 把不存在的本地时间规约到其后存在的时刻，天然满足；
// 回拨的重复本地时间只接纳第一个对应的 UTC 点，因为本函数只产生严格递增的 UTC 序列)。
func (s scheduledTaskSchedule) nextDue(after time.Time, loc *time.Location) time.Time {
	local := after.In(loc)
	switch s.Kind {
	case "every_hours":
		return local.Add(time.Duration(s.IntervalHours) * time.Hour)
	case "daily":
		return nextWallClock(local, loc, s.Hour, s.Minute, 1)
	case "weekly":
		for step := 1; step <= 7; step++ {
			candidate := nextWallClock(local, loc, s.Hour, s.Minute, step)
			for _, day := range s.Weekdays {
				if candidate.Weekday() == day {
					return candidate
				}
			}
		}
		return local.Add(8 * 24 * time.Hour)
	default:
		return local.Add(24 * time.Hour)
	}
}

// nextWallClock returns the next instant after `after` whose local wall clock reads H:M, stepping
// `days` calendar days at a time. Constructing in the task's location is what gives the DST
// semantics: a spring-forward skipped wall clock normalizes forward and is not retried that day.
func nextWallClock(after time.Time, loc *time.Location, hour, minute, days int) time.Time {
	local := after.In(loc)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if !candidate.After(local) {
		candidate = candidate.AddDate(0, 0, days)
	}
	return candidate
}

// duePoint computes the latest due point for one task: the next scheduled instant strictly after
// `resume` (the latest materialized point, or the enable anchor when nothing has been
// materialized), evaluated in the task timezone. The caller decides lateness; this function never
// looks at wall clocks.
func duePoint(schedule scheduledTaskSchedule, resume *time.Time, anchor time.Time, loc *time.Location) time.Time {
	after := anchor
	if resume != nil && resume.After(after) {
		after = *resume
	}
	return schedule.nextDue(after, loc)
}

// scheduledWeekdayNames renders the persisted weekday set for storage.
func scheduledWeekdayNames(weekdays []time.Weekday) []string {
	byName := map[time.Weekday]string{
		time.Sunday: "sunday", time.Monday: "monday", time.Tuesday: "tuesday",
		time.Wednesday: "wednesday", time.Thursday: "thursday", time.Friday: "friday",
		time.Saturday: "saturday",
	}
	out := make([]string, 0, len(weekdays))
	for _, day := range weekdays {
		out = append(out, byName[day])
	}
	return out
}

// scheduleJSON normalizes the stored schedule document so the REST shape and the worker's parser
// agree on one canonical form.
func scheduleJSON(schedule scheduledTaskSchedule) (Object, error) {
	switch schedule.Kind {
	case "every_hours":
		return Object{"kind": "every_hours", "intervalHours": schedule.IntervalHours}, nil
	case "daily":
		return Object{"kind": "daily", "hour": schedule.Hour, "minute": schedule.Minute}, nil
	case "weekly":
		return Object{"kind": "weekly", "weekdays": scheduledWeekdayNames(schedule.Weekdays), "hour": schedule.Hour, "minute": schedule.Minute}, nil
	default:
		return nil, fmt.Errorf("unknown schedule kind %q", schedule.Kind)
	}
}

// scheduledTaskLocation resolves the persisted IANA timezone. An unknown zone is a save-time
// rejection (the API validates before insert), so a worker-side failure here is invariant
// corruption and is surfaced as a panic like every other store invariant.
func scheduledTaskLocation(task Object, taskID string) *time.Location {
	zone := task.S("timezone")
	if zone == "" {
		panic(databaseFailure{fmt.Errorf("scheduled task %s has no timezone", taskID)})
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		panic(databaseFailure{fmt.Errorf("scheduled task %s has an unresolvable timezone %q: %w", taskID, zone, err)})
	}
	return loc
}

// creatorMembershipActive reports whether the task creator is still an effective tenant member
// (ADR D5/D1: 失效即自动停用，不转移、不改绑)。A failed probe counts as active so a transient
// database error never disables a task that might still be legitimate.
func creatorMembershipActive(t *transaction, taskID, tenantID, creatorID string) bool {
	return t.one(`SELECT 1
		FROM tenant_memberships tm
		JOIN tenants tn ON tn.id = tm.tenant_id AND tn.status='active' AND tn.deleted_at IS NULL
		JOIN users u ON u.id = tm.user_id AND u.status='active' AND u.deleted_at IS NULL
		WHERE tm.tenant_id=$1 AND tm.user_id=$2 AND tm.status='active'`, tenantID, creatorID) != nil
}

// scheduledTaskIssue loads the bound issue. A deleted or missing issue disables the task
// (D1: 自动停用，不自动解绑或改绑).
func scheduledTaskIssue(t *transaction, tenantID, issueID string) Object {
	return t.one("SELECT * FROM issues WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL", issueID, tenantID)
}

// scheduledTaskAgentReady re-checks the space_agents row and its desired plugin version in the
// same transaction that will create the run (ADR D5: 接纳时快照 Space 当时值). ok=false maps to
// target_unavailable.
func scheduledTaskAgentReady(t *transaction, tenantID, agentID string) (Object, bool) {
	return agentRunEvidence(t, tenantID, agentID)
}

// creatorModelReady approximates the save-time model-configured check of ADR D5: the creator has
// either a declared personal agent configuration for the agent's plugin backed by an enabled
// connection with a credential, or a personal default backed by one. The authoritative freeze and
// the final word stay with bindRunModel inside the admission transaction.
func creatorModelReady(t *transaction, creatorID, pluginID string) bool {
	if t.one(`SELECT 1
		FROM personal_agent_configurations pac
		JOIN personal_model_connections c
		  ON c.id = pac.connection_id AND c.user_id = pac.user_id
		WHERE pac.user_id=$1 AND pac.plugin_id=$2
		  AND c.enabled AND c.deleted_at IS NULL AND c.credential_id IS NOT NULL`, creatorID, pluginID) != nil {
		return true
	}
	return t.one(`SELECT 1
		FROM personal_model_defaults d
		JOIN personal_model_connections c
		  ON c.id = d.connection_id AND c.user_id = d.user_id
		WHERE d.user_id=$1
		  AND c.enabled AND c.deleted_at IS NULL AND c.credential_id IS NOT NULL`, creatorID) != nil
}

// scheduledFailureReason maps an eligibility failure to the root decision D7 reason set. Unknown
// inputs never widen the set: anything not specifically a model or consent problem is the target
// being unavailable.
func scheduledFailureReason(kind string) string {
	switch kind {
	case "model_unavailable", "approval_policy_invalid":
		return kind
	default:
		return "target_unavailable"
	}
}

// scheduledTaskBusy reports whether an earlier admitted run of this task is still live (ADR D6:
// skipped_busy，不排队不补跑). Two probes are needed: the occurrence ledger names this task's own
// runs, and the IssueRun pending-executor uniqueness also covers runs of the same issue+agent that
// predate the ledger (a manual run, or a run whose occurrence row was already pruned). `due` is the
// RFC3339 text of the point being admitted so the task's own just-inserted occurrence never makes
// it look busy.
func scheduledTaskBusy(t *transaction, taskID, issueID, agentID, due string) bool {
	if t.one(`SELECT 1
		FROM scheduled_task_occurrences o
		JOIN issue_runs r ON r.id = o.run_id
		WHERE o.task_id=$1 AND o.run_id IS NOT NULL AND o.scheduled_at <> $3::timestamptz
		  AND (r.status IN ('queued','dispatched')
		       OR r.phase IN ('provisioning','starting','running','delivering'))`, taskID, issueID, due) != nil {
		return true
	}
	return t.one(`SELECT 1
		FROM issue_runs
		WHERE issue_id=$1 AND executor_type='agent' AND executor_id=$2
		  AND status IN ('queued','dispatched') AND deleted_at IS NULL`, issueID, agentID) != nil
}

// strings.Contains helpers live in the api layer; the store never sees raw paths.
var _ = strings.TrimSpace
