package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// OpsGraph inspects the local demonstration service, never a production environment.
func OpsGraph(seed WorkflowSeed) (string, error) {
	return chainGraph(seed,
		demoNode{ID: "start", Data: map[string]any{"kind": "start", "title": "开始：每日巡检"}},
		agentStep("inspect", "检查服务与资源", "只读巡检当前仓库中的演示服务。读取 README.md、ops/metrics.json 和 ops/service.log，运行 node scripts/inspect.mjs。不要访问外网、改动配置或修复系统。下面是巡检要求：{{#start.input#}}\n逐项列出实际观察到的服务状态、错误率、磁盘和备份信息，不把示例数据说成生产环境状态。"),
		agentStep("report", "汇总风险与巡检报告", "根据上一步的实际巡检证据生成中文 Markdown 报告，含总体结论、检查项表格、异常与处置建议。不得编造未采集的指标，不执行修复。\n巡检证据：{{#inspect.output#}}"),
		outputStep("report", "report"),
	)
}

// SeedOps installs C's published baseline, nightly schedule and three past seeded runs.
// The mutable draft remains available for the live publish step (publish it as v2).
func SeedOps(ctx context.Context, seed WorkflowSeed) (SeededWorkflow, error) {
	if seed.Pool == nil || seed.Now.IsZero() {
		return SeededWorkflow{}, fmt.Errorf("ops history requires a database pool and an explicit clock")
	}
	graph, err := OpsGraph(seed)
	if err != nil {
		return SeededWorkflow{}, err
	}
	w, err := seedWorkflow(ctx, seed, "每日巡检", graph)
	if err != nil {
		return w, err
	}
	target, err := json.Marshal(map[string]string{"kind": "workflow", "workflowId": w.Workflow.ID, "snapshotId": w.Snapshot.ID, "input": "检查服务可用性、错误率、磁盘占用和备份；输出报告，不执行修复。"})
	if err != nil {
		return w, err
	}
	rule := json.RawMessage(`{"kind":"daily","time":"02:00","days":[],"hours":1,"timezone":"Asia/Shanghai"}`)
	schedule, err := seed.Store.CreateScheduleConfig(ctx, seed.Scope.TenantID, seed.Scope.UserID, store.ScheduleInput{Name: "每晚巡检 · 02:00", ProjectID: seed.ProjectID, Target: target, Rule: rule})
	if err != nil {
		return w, err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return w, err
	}
	today := seed.Now.In(loc)
	created := time.Date(today.Year(), today.Month(), today.Day(), 2, 0, 0, 0, loc).AddDate(0, 0, -3).Add(-time.Hour).UTC()
	if _, err := seed.Pool.Exec(ctx, `UPDATE v2_schedule_configs SET created_at=$2 WHERE id=$1`, schedule.ID, created); err != nil {
		return w, err
	}
	schedule.CreatedAt = created
	for days := 3; days >= 1; days-- {
		at := time.Date(today.Year(), today.Month(), today.Day(), 2, 0, 0, 0, loc).AddDate(0, 0, -days).UTC()
		if err := seedOpsHistory(ctx, seed, w, schedule, at); err != nil {
			return w, fmt.Errorf("seed ops history %s: %w", at, err)
		}
	}
	return w, nil
}

func seedOpsHistory(ctx context.Context, seed WorkflowSeed, w SeededWorkflow, schedule store.ScheduleConfig, at time.Time) error {
	// Historical reports are prepared recording data, not a claim of real past execution.
	report := fmt.Sprintf("# 每日巡检报告\n\n日期：%s（预置演示历史）\n\n| 检查项 | 结果 |\n| --- | --- |\n| 服务 | 健康，3/3 服务可用 |\n| 错误率 | 0.2%%，低于 1%% 阈值 |\n| 磁盘 | 61%%，低于 80%% 阈值 |\n| 备份 | 已完成，校验通过 |\n\n结论：无需人工处置。继续关注磁盘增长。", at.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02"))
	return pgx.BeginFunc(ctx, seed.Pool, func(tx pgx.Tx) error {
		// Only the historical ledger bypasses Store: all scene configuration uses its normal APIs.
		snapshot, err := json.Marshal(schedule)
		if err != nil {
			return err
		}
		var occurrenceID string
		if err := tx.QueryRow(ctx, `INSERT INTO v2_schedule_occurrences(schedule_id,kind,trigger_key,scheduled_at,config_version,snapshot,status,recorded_at,released)
			VALUES($1,'scheduled',$2,$3,1,$4,'succeeded',$3,true) RETURNING id`, schedule.ID, at.Format(time.RFC3339Nano), at, snapshot).Scan(&occurrenceID); err != nil {
			return err
		}
		var runID, scopeID string
		finished := at.Add(45 * time.Second)
		input := "检查服务可用性、错误率、磁盘占用和备份；输出报告，不执行修复。"
		if err := tx.QueryRow(ctx, `INSERT INTO v2_workflow_runs(tenant_id,workflow_id,snapshot_id,project_id,name,status,state,input,output,payload,trigger_kind,trigger_ref,started_at,finished_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,'succeeded','{"current_nodes":[]}',$6,$7,'{"demoSeed":true}','schedule',$8,$9,$10,$9,$10) RETURNING id`, seed.Scope.TenantID, w.Workflow.ID, w.Snapshot.ID, seed.ProjectID, "每日巡检 · "+at.In(time.FixedZone("CST", 8*3600)).Format("01-02 15:04"), input, report, occurrenceID, at, finished).Scan(&runID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO v2_workflow_scopes(id,tenant_id,run_id) VALUES(gen_random_uuid()::text,$1,$2) RETURNING id`, seed.Scope.TenantID, runID).Scan(&scopeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO v2_workflow_run_roots(run_id,scope_id) VALUES($1,$2)`, runID, scopeID); err != nil {
			return err
		}
		for i, node := range []struct{ id, kind, task, answer string }{
			{"start", "start", input, input},
			{"inspect", "agent", "读取本地演示服务状态与日志，检查可用性、错误率、磁盘和备份。", "预置演示历史：3/3 服务健康；错误率 0.2%；磁盘 61%；备份校验通过。"},
			{"report", "agent", "根据巡检证据汇总报告，不执行修复。", report},
			{"output", "output", "汇总巡检报告", report},
		} {
			payload, err := json.Marshal(map[string]any{"demoSeed": true, "task": node.task})
			if err != nil {
				return err
			}
			started := at.Add(time.Duration(i) * 10 * time.Second)
			ended := started.Add(9 * time.Second)
			if _, err := tx.Exec(ctx, `INSERT INTO v2_workflow_node_runs(tenant_id,run_id,scope_id,node_id,node_type,status,input,output,payload,started_at,finished_at,created_at,updated_at)
				VALUES($1,$2,$3,$4,$5,'succeeded',$6,$7,$8,$9,$10,$9,$10)`, seed.Scope.TenantID, runID, scopeID, node.id, node.kind, node.task, node.answer, payload, started, ended); err != nil {
				return err
			}
		}
		ref, err := json.Marshal(map[string]string{"workflowRunId": runID})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE v2_schedule_occurrences SET execution_ref=$2 WHERE id=$1`, occurrenceID, string(ref))
		return err
	})
}
