package server

import (
	"encoding/json"
	"strings"
)

// ScheduleChatTarget contains only the currently supported conversation binding.
type ScheduleChatTarget struct {
	Kind       string `json:"kind"`
	Prompt     string `json:"prompt"`
	Agent      string `json:"agent"`
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
	// Legacy references remain readable; their provider contract is not defined here.
	if kind == "workflow" || kind == "task" {
		return target, "schedule_target_unavailable"
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
	if string(fields["executorId"]) == "null" {
		return target, "invalid_target"
	}
	return target, ""
}
