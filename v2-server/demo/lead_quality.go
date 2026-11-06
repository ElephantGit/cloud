//go:build linux

package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

// qualityFindings are the three demo problems the scan reports: two from the incremental scan over
// release.json's changed files, and one from the full scan's coverage rule. Keys stay stable across
// nights so the confirmation form can reuse the same items.
var qualityFindings = []any{
	map[string]any{
		"key": "src:hardcoded-token", "title": "源码中存在硬编码令牌",
		"description": "令牌应与代码分离，改为从部署环境读取",
		"evidence":    "src/sync.mjs:1 常量里直接写入了令牌", "severity": "高", "module": "src/sync.mjs",
	},
	map[string]any{
		"key": "src:eval-usage", "title": "使用 eval 执行动态代码",
		"description": "改为模板函数或显式分支，避免执行拼接出来的代码",
		"evidence":    "src/report.mjs:3 用 eval 渲染模板", "severity": "中", "module": "src/report.mjs",
	},
	map[string]any{
		"key": "test:uncovered-module", "title": "模块缺少测试覆盖",
		"description": "新增模块需要补上对应的 node --test 用例",
		"evidence":    "src/export.mjs 没有 test/export.test.mjs", "severity": "中", "module": "test",
	},
}

// QualityScanGraph scans the changed files and the whole repository, freezes a report, and waits for
// a person to confirm each finding. It creates nothing: confirming only records the decisions.
func QualityScanGraph(seed WorkflowSeed) (string, error) {
	report := agentStep("report", "汇总检测报告", "根据两步扫描的 JSON 结果写中文 Markdown 检测报告：开头先用一个 Markdown 表格分别列出增量扫描和全量扫描的文件数与发现数，再列出全部 findings，key 与字段原样保留。最终回答必须是结构化输出，字段 reportMarkdown 和 findings；不创建任务、不修改文件、不编造未扫到的结果。\n增量扫描：{{#incremental.output#}}\n全量扫描：{{#full.output#}}")
	properties := map[string]any{}
	for _, key := range []string{"key", "title", "description", "evidence", "severity", "module"} {
		properties[key] = map[string]string{"type": "string"}
	}
	report.Data["agentConfig"].(map[string]any)["outputContract"] = map[string]any{"type": "structured", "schema": map[string]any{
		"type": "object", "required": []string{"reportMarkdown", "findings"}, "additionalProperties": false,
		"properties": map[string]any{
			"reportMarkdown": map[string]string{"type": "string"},
			"findings":       map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"key", "title", "description", "evidence", "severity", "module"}, "additionalProperties": false, "properties": properties}},
		},
	}}
	return chainGraph(seed,
		demoNode{ID: "start", Data: map[string]any{"kind": "start", "title": "开始：版本质量扫描"}},
		agentStep("incremental", "增量扫描", "在项目工作目录里只读执行 `node scripts/scan.mjs --mode incremental`：它只检查 release.json 列出的变更文件。不要修改文件、安装依赖或访问外网，把命令输出的完整 JSON 原样作为最终回答。\n扫描要求：{{#start.input#}}"),
		agentStep("full", "全量扫描", "继续只读执行 `node scripts/scan.mjs --mode full`：它在增量规则之外检查全部源码的测试覆盖，同样把输出的完整 JSON 原样作为最终回答。不要修改文件。\n增量结果：{{#incremental.output#}}"),
		report,
		demoNode{ID: "confirm", Data: map[string]any{"kind": "form", "title": "确认扫描结果", "formConfig": map[string]any{
			"title": "版本质量扫描报告与问题确认", "submitLabel": "提交确认",
			"reportSelector": []string{"report", "structured_output", "reportMarkdown"},
			"itemsSelector":  []string{"report", "structured_output", "findings"},
			"fields":         []any{map[string]any{"name": "notes", "label": "备注", "type": "textarea", "required": false}},
		}}},
		demoNode{ID: "output", Data: map[string]any{"kind": "output", "title": "输出确认结果", "outputs": []any{
			map[string]any{"name": "report", "variableSelector": []string{"report", "structured_output", "reportMarkdown"}},
			map[string]any{"name": "answers", "variableSelector": []string{"confirm", "structured_output", "answers"}},
		}}},
	)
}

// SeedQualityScan installs 陈默's 「版本质量扫描」 workflow, its nightly schedule and three nights of
// history. The newest run stops at the confirmation form, so the recording can submit it.
func SeedQualityScan(ctx context.Context, seed WorkflowSeed) (SeededWorkflow, error) {
	if seed.Pool == nil || seed.Now.IsZero() {
		return SeededWorkflow{}, fmt.Errorf("quality scan history requires a database pool and an explicit clock")
	}
	graph, err := QualityScanGraph(seed)
	if err != nil {
		return SeededWorkflow{}, err
	}
	w, err := seedWorkflow(ctx, seed, "版本质量扫描", graph)
	if err != nil {
		return w, err
	}
	target, err := json.Marshal(map[string]string{
		"kind": "workflow", "workflowId": w.Workflow.ID, "snapshotId": w.Snapshot.ID,
		"input": "扫描协作看板服务的增量与全量代码质量，输出检测报告并等待确认。",
	})
	if err != nil {
		return w, err
	}
	rule := json.RawMessage(`{"kind":"daily","time":"02:00","days":[],"hours":1,"timezone":"Asia/Shanghai"}`)
	schedule, err := seed.Store.CreateScheduleConfig(ctx, seed.Scope.TenantID, seed.Scope.UserID, store.ScheduleInput{
		Name: "版本质量扫描 · 02:00", ProjectID: seed.ProjectID, Target: target, Rule: rule,
	})
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
		if err := seedQualityRun(ctx, seed, w, schedule, at); err != nil {
			return w, fmt.Errorf("seed quality history %s: %w", at, err)
		}
		// Every seeded night was confirmed, so the recording's own manual run is the one that stops at
		// the form; the seed never leaves a run waiting for input.
		if err := seedQualityConfirm(ctx, seed, w, schedule, at, true); err != nil {
			return w, err
		}
	}
	return w, nil
}

// qualityReport renders the deterministic report of one night. The visible text carries no marker;
// payload.demoSeed on the run and its nodes says it was seeded.
func qualityReport(at time.Time) string {
	return fmt.Sprintf(`# 版本质量扫描报告

基线：v0.3.0 · 扫描时间：%s

| 扫描 | 范围 | 结果 |
| --- | --- | --- |
| 增量扫描 | 3 个变更文件 | 2 项问题 |
| 全量扫描 | 4 个源码文件、9 条规则 | 1 项问题 |
| 合并去重 | 全部 findings | 3 项待确认 |

## 发现

1. **高** 源码中存在硬编码令牌 —— src/sync.mjs
2. **中** 使用 eval 执行动态代码 —— src/report.mjs
3. **中** 模块缺少测试覆盖 —— src/export.mjs

建议在发版前处理硬编码令牌和 eval 用法，并补上导出模块的测试。`, at.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04"))
}

func seedQualityRun(ctx context.Context, seed WorkflowSeed, w SeededWorkflow, schedule store.ScheduleConfig, at time.Time) error {
	schedule.Target, _ = json.Marshal(map[string]string{"kind": "workflow", "workflowId": w.Workflow.ID, "snapshotId": w.Snapshot.ID, "input": "版本质量扫描"})
	report := qualityReport(at)
	incremental := `{"mode":"incremental","scanned":{"files":3,"rules":6,"changed":3},"findings":[{"key":"src:eval-usage"},{"key":"src:hardcoded-token"}]}`
	full := `{"mode":"full","scanned":{"files":4,"rules":9,"changed":3},"findings":[{"key":"src:eval-usage"},{"key":"src:hardcoded-token"},{"key":"test:uncovered-module"}]}`
	return pgx.BeginFunc(ctx, seed.Pool, func(tx pgx.Tx) error {
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
		finished := at.Add(75 * time.Second)
		input := "扫描协作看板服务的增量与全量代码质量，输出检测报告并等待确认。"
		if err := tx.QueryRow(ctx, `INSERT INTO v2_workflow_runs(tenant_id,workflow_id,snapshot_id,project_id,name,status,state,input,output,payload,trigger_kind,trigger_ref,started_at,finished_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,'succeeded','{"current_nodes":[]}',$6,$7,'{"demoSeed":true}','schedule',$8,$9,$10,$9,$10) RETURNING id`, seed.Scope.TenantID, w.Workflow.ID, w.Snapshot.ID, seed.ProjectID, "版本质量扫描 · "+at.In(time.FixedZone("CST", 8*3600)).Format("01-02 15:04"), input, report, occurrenceID, at, finished).Scan(&runID); err != nil {
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
			{"incremental", "agent", "对变更文件做增量质量扫描。", incremental},
			{"full", "agent", "对全部源码做全量质量扫描。", full},
			{"report", "agent", "汇总两步扫描结果，输出检测报告。", report},
			{"output", "output", "汇总检测报告", report},
		} {
			payload, err := json.Marshal(map[string]any{"demoSeed": true, "task": node.task})
			if err != nil {
				return err
			}
			started := at.Add(time.Duration(i) * 12 * time.Second)
			ended := started.Add(11 * time.Second)
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

// seedQualityConfirm turns a seeded run into the new snapshot's confirmation run: with submitted it
// records the decisions and finishes the run, otherwise it leaves the form open for recording.
func seedQualityConfirm(ctx context.Context, seed WorkflowSeed, w SeededWorkflow, schedule store.ScheduleConfig, at time.Time, submitted bool) error {
	var runID, occurrenceID string
	if err := seed.Pool.QueryRow(ctx, `SELECT r.id::text,o.id::text FROM v2_workflow_runs r JOIN v2_schedule_occurrences o ON o.id::text=r.trigger_ref WHERE o.schedule_id=$1 AND o.scheduled_at=$2`, schedule.ID, at).Scan(&runID, &occurrenceID); err != nil {
		return err
	}
	snapshot, _ := json.Marshal(schedule)
	if _, err := seed.Pool.Exec(ctx, `UPDATE v2_schedule_occurrences SET snapshot=$2 WHERE id=$1`, occurrenceID, snapshot); err != nil {
		return err
	}
	graph, err := workflow.ParseGraph(w.Snapshot.Graph)
	if err != nil {
		return err
	}
	report := qualityReport(at)
	p := workflow.NewRunPayload(graph, workflow.LocaleZhCN)
	if err := p.MirrorInput(&report); err != nil {
		return err
	}
	for _, node := range []struct{ id, kind string }{{"incremental", "agent"}, {"full", "agent"}} {
		if err := p.Complete(node.id, node.kind, nil, &report, nil); err != nil {
			return err
		}
	}
	if err := p.Complete("report", "agent", nil, &report, map[string]any{"reportMarkdown": report, "findings": qualityFindings}); err != nil {
		return err
	}
	raw, err := p.Encode()
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	payload["demoSeed"] = true
	raw, _ = json.Marshal(payload)
	if _, err := seed.Pool.Exec(ctx, `DELETE FROM v2_workflow_node_runs WHERE run_id=$1 AND node_id='output'`, runID); err != nil {
		return err
	}
	if _, err := seed.Pool.Exec(ctx, `UPDATE v2_workflow_runs SET snapshot_id=$2,status='running',payload=$3,output=NULL,finished_at=NULL,started_by=$4 WHERE id=$1`, runID, w.Snapshot.ID, raw, seed.Scope.UserID); err != nil {
		return err
	}
	nodes, err := seed.Store.StartWorkflowEngineNodes(ctx, runID, []store.WorkflowEngineStart{{NodeID: "confirm", NodeType: "form"}})
	if err != nil {
		return err
	}
	node, _ := graph.Node("confirm")
	evidence, _ := json.Marshal(qualityFindings)
	form, err := seed.Store.OpenWorkflowForm(ctx, runID, nodes[0].ID, store.WorkflowForm{Config: node.FormConfig(), Findings: evidence, Report: report})
	if err != nil {
		return err
	}
	if !submitted {
		_, err = seed.Pool.Exec(ctx, `UPDATE v2_schedule_occurrences SET status='running',released=false,waiting_for_input=true WHERE id=$1`, occurrenceID)
		return err
	}
	answers := []any{}
	for i, row := range qualityFindings {
		finding := row.(map[string]any)
		decision := "confirmed"
		if i == 2 {
			decision = "rejected"
		}
		answers = append(answers, map[string]any{"key": finding["key"], "decision": decision, "assignee": map[string]any{"type": "human", "id": seed.Scope.UserID}, "values": map[string]string{}})
	}
	answerJSON, _ := json.Marshal(answers)
	if _, err := seed.Store.SaveWorkflowForm(ctx, seed.Scope.TenantID, seed.Scope.UserID, runID, form.Version, "demo-seed", answerJSON, true, func(store.WorkflowForm) error { return nil }, func(raw []byte, f store.WorkflowForm) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		text := string(answerJSON)
		if err := p.Complete(f.NodeID, "form", nil, &text, map[string]any{"answers": answers}); err != nil {
			return nil, err
		}
		return p.Encode()
	}); err != nil {
		return err
	}
	output, err := seed.Store.StartWorkflowEngineNodes(ctx, runID, []store.WorkflowEngineStart{{NodeID: "output", NodeType: "output"}})
	if err != nil {
		return err
	}
	if _, err := seed.Store.CompleteWorkflowEngineNode(ctx, runID, output[0].ID, &report, func(b []byte) ([]byte, error) { return b, nil }, func(b []byte, _ store.WorkflowNodeRun) ([]byte, error) { return b, nil }); err != nil {
		return err
	}
	if err := seed.Store.FinishWorkflowEngineRun(ctx, runID, &report); err != nil {
		return err
	}
	_, err = seed.Pool.Exec(ctx, `UPDATE v2_workflow_runs SET payload=payload||'{"demoSeed":true}'::jsonb WHERE id=$1`, runID)
	return err
}
