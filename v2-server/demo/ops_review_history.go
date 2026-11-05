package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
	"time"
)

// seedOpsReviewHistory replaces only a just-created seed history with the new
// snapshot. It is never an upgrade routine for existing user reports.
func seedOpsReviewHistory(ctx context.Context, seed WorkflowSeed, w SeededWorkflow, schedule store.ScheduleConfig, at time.Time, submitted bool) error {
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
	findings := []any{
		map[string]any{"key": "api:error-rate", "title": "接口错误率超过阈值", "description": "检查接口错误日志", "evidence": "错误率 2.5%，阈值 1%", "severity": "高", "module": "API"},
		map[string]any{"key": "storage:root-disk", "title": "磁盘占用超过阈值", "description": "检查磁盘增长", "evidence": "磁盘 86%，阈值 80%", "severity": "中", "module": "存储"},
		map[string]any{"key": "service:maintenance-notice", "title": "维护窗口内出现重启提示", "description": "结合维护记录判断", "evidence": "维护已完成，服务健康", "severity": "低", "module": "服务"},
	}
	report := "# 每日巡检报告\n\n接口错误率与磁盘占用超过阈值；维护窗口的重启提示需人工判断。"
	p := workflow.NewRunPayload(graph, workflow.LocaleZhCN)
	if err := p.MirrorInput(&report); err != nil {
		return err
	}
	if err := p.Complete("inspect", "agent", nil, &report, nil); err != nil {
		return err
	}
	if err := p.Complete("report", "agent", nil, &report, map[string]any{"reportMarkdown": report, "findings": findings}); err != nil {
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
	nodes, err := seed.Store.StartWorkflowEngineNodes(ctx, runID, []store.WorkflowEngineStart{{NodeID: "review", NodeType: "form"}})
	if err != nil {
		return err
	}
	node, _ := graph.Node("review")
	evidence, _ := json.Marshal(findings)
	form, err := seed.Store.OpenWorkflowForm(ctx, runID, nodes[0].ID, store.WorkflowForm{Config: node.FormConfig(), Findings: evidence, Report: report})
	if err != nil {
		return err
	}
	if !submitted {
		_, err = seed.Pool.Exec(ctx, `UPDATE v2_schedule_occurrences SET status='running',released=false,waiting_for_input=true WHERE id=$1`, occurrenceID)
		return err
	}
	answers := []any{}
	items := []store.FormTaskInput{}
	for i, row := range findings {
		finding := row.(map[string]any)
		key := finding["key"].(string)
		decision := "confirmed"
		if i == 2 {
			decision = "rejected"
		}
		answers = append(answers, map[string]any{"key": key, "decision": decision, "assignee": map[string]any{"type": "human", "id": seed.Scope.UserID}, "values": map[string]string{}})
		if i < 2 {
			items = append(items, store.FormTaskInput{Key: key, Title: finding["title"].(string), Description: fmt.Sprintf("巡检异常\n来源巡检：/chat?run=%s", runID), Assignee: &store.TaskAssigneeRef{Type: "human", ID: seed.Scope.UserID}})
		}
	}
	answerJSON, _ := json.Marshal(answers)
	_, err = seed.Store.SaveWorkflowForm(ctx, seed.Scope.TenantID, seed.Scope.UserID, runID, form.Version, "demo-seed", answerJSON, true, func(store.WorkflowForm) error { return nil }, func(raw []byte, f store.WorkflowForm) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		text := string(answerJSON)
		if err := p.Complete(f.NodeID, "form", nil, &text, map[string]any{"answers": answers}); err != nil {
			return nil, err
		}
		return p.Encode()
	})
	if err != nil {
		return err
	}
	tasks, err := seed.Store.StartWorkflowEngineNodes(ctx, runID, []store.WorkflowEngineStart{{NodeID: "tasks", NodeType: "createTasks"}})
	if err != nil {
		return err
	}
	err = seed.Store.CreateWorkflowFormTasks(ctx, runID, tasks[0].ID, items, func(raw []byte, n store.WorkflowNodeRun, results []store.FormTaskResult) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		r, _ := json.Marshal(map[string]any{"tasks": results})
		var value map[string]any
		_ = json.Unmarshal(r, &value)
		text := string(r)
		if err := p.Complete(n.NodeID, "createTasks", nil, &text, value); err != nil {
			return nil, err
		}
		return p.Encode()
	})
	if err != nil {
		return err
	}
	output, err := seed.Store.StartWorkflowEngineNodes(ctx, runID, []store.WorkflowEngineStart{{NodeID: "output", NodeType: "output"}})
	if err != nil {
		return err
	}
	_, err = seed.Store.CompleteWorkflowEngineNode(ctx, runID, output[0].ID, &report, func(b []byte) ([]byte, error) { return b, nil }, func(b []byte, _ store.WorkflowNodeRun) ([]byte, error) { return b, nil })
	if err != nil {
		return err
	}
	if err := seed.Store.FinishWorkflowEngineRun(ctx, runID, &report); err != nil {
		return err
	}
	_, err = seed.Pool.Exec(ctx, `UPDATE v2_workflow_runs SET payload=payload||'{"demoSeed":true}'::jsonb WHERE id=$1`, runID)
	return err
}
