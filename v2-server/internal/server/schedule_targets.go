package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ErrScheduleResourcesUnavailable means the owning task/workflow service is not wired yet.
var ErrScheduleResourcesUnavailable = errors.New("schedule resources unavailable")

// ScheduleTargetResolver validates live references with current caller permissions. It is
// implemented by the shared task/workflow modules, never by the schedule configuration store.
// It must reject unavailable projects, unpublished/incompatible snapshots and invalid inputs.
// For tasks it returns the source task's current project; this is checked again at dispatch.
type ScheduleTargetResolver interface {
	ResolveScheduleTarget(context.Context, string, string, ScheduleTarget) (string, error)
}

// ScheduleTarget is a configuration reference, not an execution request or proof of readiness.
type ScheduleTarget struct {
	Kind                    string            `json:"kind"`
	Prompt                  string            `json:"prompt,omitempty"`
	Agent                   string            `json:"agent,omitempty"`
	Model                   string            `json:"model,omitempty"`
	WorkflowID              string            `json:"workflowId,omitempty"`
	Snapshot                string            `json:"snapshot,omitempty"`
	Inputs                  map[string]string `json:"inputs,omitempty"`
	SourceTaskID            string            `json:"sourceTaskId,omitempty"`
	SupplementalInstruction string            `json:"supplementalInstruction,omitempty"`
	ProjectID               string            `json:"-"`
}

func decodeScheduleTarget(raw json.RawMessage) (ScheduleTarget, string) {
	var target ScheduleTarget
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&target) != nil {
		return target, "invalid_target"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return target, "invalid_target"
	}
	var required []string
	switch target.Kind {
	case "chat":
		required = []string{"kind", "prompt", "agent", "model"}
		if strings.TrimSpace(target.Prompt) == "" || len(target.Prompt) > 32000 {
			return target, "invalid_target"
		}
	case "workflow":
		required = []string{"kind", "workflowId", "snapshot", "inputs"}
		if !scheduleUUID.MatchString(target.WorkflowID) || !scheduleUUID.MatchString(target.Snapshot) {
			return target, "invalid_target"
		}
	case "task":
		required = []string{"kind", "sourceTaskId", "supplementalInstruction"}
		if !scheduleUUID.MatchString(target.SourceTaskID) || len(target.SupplementalInstruction) > 32000 {
			return target, "invalid_target"
		}
	default:
		return target, "invalid_target"
	}
	if len(fields) != len(required) {
		return target, "invalid_target"
	}
	for _, key := range required {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return target, "invalid_target"
		}
	}
	return target, ""
}

func (h *handler) resolveScheduleTarget(ctx context.Context, tenant, user string, in *store.ScheduleInput) error {
	target, code := decodeScheduleTarget(in.Target)
	if code != "" {
		return store.ErrNotFound
	}
	if target.Kind == "chat" {
		return nil
	}
	if h.opts.ScheduleTargets == nil {
		return ErrScheduleResourcesUnavailable
	}
	target.ProjectID = in.ProjectID
	project, err := h.opts.ScheduleTargets.ResolveScheduleTarget(ctx, tenant, user, target)
	if err != nil {
		return err
	}
	if !scheduleUUID.MatchString(project) {
		return ErrScheduleResourcesUnavailable
	}
	if target.Kind == "workflow" && project != in.ProjectID {
		return store.ErrNotFound
	}
	in.ProjectID = project
	return nil
}
