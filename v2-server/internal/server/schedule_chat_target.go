package server

import (
	"encoding/json"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
	"github.com/wanglongan587/cloud/v2-server/internal/taskrun"
)

// ScheduleChatTarget contains only the currently supported conversation binding.
type ScheduleChatTarget struct {
	Kind       string `json:"kind"`
	Prompt     string `json:"prompt"`
	Agent      string `json:"agent"`
	AgentID    string `json:"agentId,omitempty"`
	Model      string `json:"model"`
	ExecutorID string `json:"executorId,omitempty"`
}

func decodeScheduleChat(raw json.RawMessage) (ScheduleChatTarget, string) {
	var target ScheduleChatTarget
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return target, "invalid_target"
	}
	var kind string
	if json.Unmarshal(fields["kind"], &kind) != nil {
		return target, "invalid_target"
	}
	// Only the personal task binding is writable. Unbound legacy references stay read-only.
	if kind == "task" {
		var t taskrun.ScheduledTarget
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.DisallowUnknownFields()
		if len(fields["taskId"]) == 0 || len(fields["executorId"]) == 0 {
			return target, "schedule_target_unavailable"
		}
		if d.Decode(&t) != nil || !scheduleUUID.MatchString(t.TaskID) || strings.TrimSpace(t.ExecutorID) == "" || len(t.ExecutorID) > 200 || len(t.Note) > 32000 || len(fields["note"]) == 0 || string(fields["note"]) == "null" {
			return target, "invalid_target"
		}
		return ScheduleChatTarget{Kind: kind}, ""
	}
	// A workflow target has its own shape (decodeScheduleWorkflow).
	if kind == "workflow" {
		return ScheduleChatTarget{Kind: kind}, ""
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&target) != nil || kind != "chat" || strings.TrimSpace(target.Prompt) == "" || len(target.Prompt) > 32000 {
		return target, "invalid_target"
	}
	for _, key := range []string{"kind", "prompt", "agent", "model"} {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return target, "invalid_target"
		}
	}
	if string(fields["agentId"]) == "null" || string(fields["executorId"]) == "null" {
		return target, "invalid_target"
	}
	return target, ""
}

// decodeScheduleWorkflow reads a workflow target strictly: a workflow, the published snapshot of
// it the schedule runs, and the kickoff input, which may be empty.
func decodeScheduleWorkflow(raw json.RawMessage) (scheduler.WorkflowTarget, string) {
	var target scheduler.WorkflowTarget
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&target) != nil || target.Kind != "workflow" || len(target.Input) > 32000 {
		return target, "invalid_target"
	}
	if !workflowUUID.MatchString(target.WorkflowID) || !workflowUUID.MatchString(target.SnapshotID) {
		return target, "invalid_target"
	}
	return target, ""
}
