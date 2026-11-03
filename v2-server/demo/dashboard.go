//go:build linux

package demo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// seedDashboard provides a labelled task history example for B and the combined demo. No
// provider is called and no request usage is invented; the metrics tab explicitly stays uncollected.
func seedDashboard(ctx context.Context, s *Seeder, scope execution.Scope, project store.Project) error {
	sid := "demo-dashboard-" + project.ID
	if _, err := s.Store.ReadExecutionSession(ctx, scope, sid); err == nil {
		return nil
	} else if !errors.Is(err, execution.ErrNotFound) {
		return err
	}
	agents, err := spaceAgents(ctx, s, scope)
	if err != nil {
		return err
	}
	task, err := s.Store.CreateTask(ctx, scope.TenantID, scope.UserID, store.TaskInput{ProjectID: project.ID, Assignee: &store.TaskAssigneeRef{Type: store.AssigneeAgent, ID: agents["ora-space.opencode"]}, Title: "预置分析样例 · 看板变更检查（非现场执行）"})
	if err != nil {
		return err
	}
	run, err := s.Store.CreateTaskRun(ctx, store.NewTaskRun{TaskID: task.ID, TenantID: scope.TenantID, StartedBy: scope.UserID, StartKey: sid, ExecutorID: "opencode:fixture", ExecutorName: "OpenCode"})
	if err != nil {
		return err
	}
	at := s.Now.Add(-time.Hour)
	ref := execution.Ref{SessionID: sid, TurnID: sid + "-turn"}
	session := execution.Session{ID: sid, CreatedAt: at, Target: &execution.Target{ProjectID: project.ID}, Executor: &execution.Executor{ID: "opencode:fixture", AgentID: "opencode", AgentName: "OpenCode", Model: execution.Model{ID: "fixture", Name: "Fixture"}}, Model: execution.Model{ID: "fixture", Name: "Fixture"}}
	turn := execution.Turn{Ref: ref, StartedAt: at, Caller: execution.Caller{Kind: execution.CallerTask, ID: run.ID, AttemptID: sid}, Prompt: execution.TextPrompt("预置样例：检查看板状态转换。")}
	if err := s.Store.CreateExecutionSession(ctx, scope, session, turn); err != nil {
		return err
	}
	if err := s.Store.RecordTaskTurn(ctx, run.ID, ref.SessionID, ref.TurnID); err != nil {
		return err
	}
	snapshot := execution.HistorySnapshot{Records: []execution.HistoryRecord{}}
	updates := []map[string]any{
		{"sessionUpdate": "user_message_chunk", "content": map[string]string{"type": "text", "text": "预置样例：检查看板状态转换。"}},
		{"sessionUpdate": "agent_thought_chunk", "content": map[string]string{"type": "text", "text": "先读取 README，再查看状态转换约束。"}},
		{"sessionUpdate": "tool_call", "toolCallId": "demo-read", "kind": "read", "title": "README.md", "status": "completed", "rawInput": map[string]string{"path": "README.md"}, "rawOutput": "预置示例：状态依次推进。"},
		{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "预置样例报告：看板状态约束已检查。本样例未调用模型，Token 与请求数未采集。"}},
	}
	for i, update := range updates {
		raw, err := json.Marshal(map[string]any{"seq": i + 1, "at": at.Add(time.Duration(i) * time.Second), "type": "update", "update": update, "_meta": map[string]bool{"demoSeed": true}})
		if err != nil {
			return err
		}
		snapshot.Records = append(snapshot.Records, execution.HistoryRecord{Sequence: uint32(i + 1), Data: raw})
	}
	if _, err := s.Store.AppendExecutionEvent(ctx, scope, execution.Event{Ref: ref, Kind: execution.EventHistorySnapshot, History: &snapshot}); err != nil {
		return err
	}
	if _, err := s.Store.FinishExecutionTurn(ctx, scope, ref, execution.Result{Status: execution.StatusCompleted, Answer: "预置样例已完成，未调用真实模型。", AnswerComplete: true, StopReason: "stop"}); err != nil {
		return err
	}
	if err := s.Store.EndExecutionSession(ctx, scope, sid); err != nil {
		return err
	}
	if _, err := s.Store.SettleTaskTurn(ctx, run.ID, ref.TurnID, "completed", ""); err != nil {
		return err
	}
	for _, transition := range [][2]string{{store.TaskPending, store.TaskRunning}, {store.TaskRunning, store.TaskReview}} {
		if _, err := s.Store.TransitionTaskStatus(ctx, scope.TenantID, task.ID, transition[0], transition[1]); err != nil {
			return err
		}
	}
	return s.Store.EndTaskRun(ctx, run.ID, store.TaskRunClosed)
}
