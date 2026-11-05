package workflowhost

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
	"github.com/wanglongan587/cloud/v2-server/internal/workflowforms"
	"strings"
)

func (h *Host) dispatchReview(d workflow.AgentDispatch) {
	defer h.retireReview(d.RunID)
	var err error
	if d.Node.Kind == "form" {
		var c workflowforms.Config
		c, err = workflowforms.DecodeConfig(d.Node.FormConfig())
		if err == nil {
			var value any
			var assigned bool
			value, assigned, err = d.Pool.Resolve(c.ItemsSelector)
			if err == nil && !assigned {
				err = fmt.Errorf("form items not assigned")
			}
			var findings []workflowforms.Finding
			if err == nil {
				findings, err = workflowforms.DecodeFindings(value)
			}
			var report any
			if err == nil {
				report, assigned, err = d.Pool.Resolve(c.ReportSelector)
				if err == nil && !assigned {
					err = fmt.Errorf("form report not assigned")
				}
			}
			if err == nil {
				text, ok := report.(string)
				if !ok {
					err = fmt.Errorf("form report must be text")
				} else {
					raw, _ := json.Marshal(findings)
					_, err = h.store.OpenWorkflowForm(h.ctx, d.RunID, d.NodeRunID, store.WorkflowForm{ReviewerID: c.ReviewerID, Config: canonicalFormConfig(c), Findings: raw, Report: text})
				}
			}
		}
	} else {
		err = h.createFormTasks(h.ctx, d.RunID, d.NodeRunID)
	}
	if err != nil {
		h.fail(d, workflow.Failure{Kind: workflow.FailureSession, Message: "表单处理失败，请核对数据与负责人后重试"})
		return
	}
	h.hub.Publish(d.RunID)
	if d.Node.Kind == "createTasks" {
		if err := h.engine.Resume(h.ctx, d.RunID); err != nil {
			h.log.Error("resume form tasks", "error", err)
		}
	}
}

func (h *Host) createFormTasks(ctx context.Context, runID, nodeRunID string) error {
	f, err := h.store.WorkflowFormForEngine(ctx, runID)
	if err != nil {
		return err
	}
	var answers []workflowforms.Answer
	var findings []workflowforms.Finding
	if err := json.Unmarshal(f.Answers, &answers); err != nil {
		return err
	}
	if err := json.Unmarshal(f.Findings, &findings); err != nil {
		return err
	}
	byKey := map[string]workflowforms.Finding{}
	for _, v := range findings {
		byKey[v.Key] = v
	}
	items := []store.FormTaskInput{}
	for _, a := range answers {
		if a.Decision != "confirmed" {
			continue
		}
		v, ok := byKey[a.Key]
		if !ok {
			return fmt.Errorf("unknown finding")
		}
		description := v.Description + "\n\n" + a.Values["notes"]
		chars := []rune(description)
		if len(chars) > 3500 {
			description = string(chars[:3500])
		}
		description += "\n\n来源巡检：/chat?run=" + runID + "\n问题标识：" + v.Key
		var assignee *store.TaskAssigneeRef
		if a.Assignee != nil {
			assignee = &store.TaskAssigneeRef{Type: a.Assignee.Type, ID: a.Assignee.ID}
		}
		items = append(items, store.FormTaskInput{Key: v.Key, Title: v.Title, Description: description, Assignee: assignee})
	}
	return h.store.CreateWorkflowFormTasks(ctx, runID, nodeRunID, items, func(raw []byte, node store.WorkflowNodeRun, results []store.FormTaskResult) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		output, _ := json.Marshal(results)
		text := string(output)
		structured := map[string]any{}
		encoded, _ := json.Marshal(map[string]any{"tasks": results})
		_ = json.Unmarshal(encoded, &structured)
		if err := p.Complete(node.NodeID, "createTasks", nil, &text, structured); err != nil {
			return nil, err
		}
		return p.Encode()
	})
}

// ReadForm serves the durable form rather than deriving fresh evidence from the model.
func (h *Host) ReadForm(ctx context.Context, s server.WorkflowRunScope, id string) (store.WorkflowForm, error) {
	return h.store.WorkflowForm(ctx, s.TenantID, s.UserID, id)
}

// SaveForm persists a draft or completes the form; only subsequent nodes create tasks.
func (h *Host) SaveForm(ctx context.Context, s server.WorkflowRunScope, id string, version int64, key string, answers json.RawMessage, final bool) (store.WorkflowForm, error) {
	var parsed []workflowforms.Answer
	decoder := json.NewDecoder(strings.NewReader(string(answers)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return store.WorkflowForm{}, &workflowforms.FieldError{Field: "answers", Reason: "invalid answer shape"}
	}
	for i := range parsed {
		if parsed[i].Values == nil {
			parsed[i].Values = map[string]string{}
		}
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return store.WorkflowForm{}, err
	}
	answers = normalized
	f, err := h.store.SaveWorkflowForm(ctx, s.TenantID, s.UserID, id, version, key, answers, final, func(f store.WorkflowForm) error {
		if err := json.Unmarshal(f.Answers, &parsed); err != nil {
			return err
		}
		config, err := workflowforms.DecodeConfig(f.Config)
		if err != nil {
			return err
		}
		var findings []workflowforms.Finding
		if err := json.Unmarshal(f.Findings, &findings); err != nil {
			return err
		}
		return workflowforms.ValidateAnswers(config.Fields, findings, parsed, final, nil)
	}, func(raw []byte, f store.WorkflowForm) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		var values any
		if err := json.Unmarshal(f.Answers, &values); err != nil {
			return nil, err
		}
		output := string(f.Answers)
		if err := p.Complete(f.NodeID, "form", nil, &output, map[string]any{"answers": values}); err != nil {
			return nil, err
		}
		return p.Encode()
	})
	if err != nil {
		return f, err
	}
	h.hub.Publish(id)
	if final {
		if err := h.resumeReviewRun(context.WithoutCancel(ctx), id); err != nil {
			return f, err
		}
	}
	return f, nil
}

// resumeReviewRun replays only committed, deterministic review work.
func (h *Host) resumeReviewRun(ctx context.Context, id string) error {
	defer h.retireReview(id)
	nodes, err := h.store.WorkflowEngineNodeRuns(ctx, id)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.NodeType == "createTasks" && n.Status == store.WorkflowStatusRunning {
			if err := h.createFormTasks(ctx, id, n.ID); err != nil {
				if recordErr := h.engine.FailNode(ctx, id, n.ID, workflow.Failure{Kind: workflow.FailureSession, Message: "任务创建失败，请核对负责人后重试"}); recordErr != nil {
					return recordErr
				}
				return err
			}
		}
	}
	return h.engine.Resume(ctx, id)
}

// RetryForm retries task creation without collecting or confirming evidence again.
func (h *Host) RetryForm(ctx context.Context, s server.WorkflowRunScope, id string) (store.WorkflowForm, error) {
	if err := h.store.RetryWorkflowFormTasks(ctx, s.TenantID, s.UserID, id); err != nil {
		return store.WorkflowForm{}, err
	}
	if err := h.resumeReviewRun(ctx, id); err != nil {
		return store.WorkflowForm{}, err
	}
	h.hub.Publish(id)
	return h.ReadForm(ctx, s, id)
}

func (h *Host) retireReview(id string) {
	if state := h.state(id); state != nil {
		h.forgetFinished(id, state)
	}
}

func canonicalFormConfig(c workflowforms.Config) json.RawMessage {
	raw, _ := json.Marshal(c)
	return raw
}
