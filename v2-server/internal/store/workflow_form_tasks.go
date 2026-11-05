package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sort"
	"unicode/utf8"
)

// FormTaskInput is a validated confirmed row, derived by the host from frozen evidence.
type FormTaskInput struct {
	Key, Title, Description string
	Assignee                *TaskAssigneeRef
}

// FormTaskResult identifies the actual task created or reused for one finding.
type FormTaskResult struct {
	Key    string `json:"key"`
	TaskID string `json:"taskId"`
	Action string `json:"action"`
}

// CreateWorkflowFormTasks creates a complete batch, its source links and node
// completion in one transaction. Locks are ordered across runs for cross-day reuse.
func (s *Store) CreateWorkflowFormTasks(ctx context.Context, runID, nodeRunID string, items []FormTaskInput, fold func([]byte, WorkflowNodeRun, []FormTaskResult) ([]byte, error)) error {
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := lockEngineRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if r.status != WorkflowStatusRunning {
			return ErrWorkflowRunState
		}
		node, err := lockWorkflowNodeRun(ctx, tx, r.tenant, runID, nodeRunID)
		if err != nil {
			return err
		}
		if node.Status == WorkflowStatusSucceeded {
			return nil
		}
		if node.Status != WorkflowStatusRunning || node.NodeType != "createTasks" {
			return ErrWorkflowNodeRunState
		}
		f, err := scanForm(tx.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE run_id=$1`, runID))
		if err != nil {
			return err
		}
		if f.Status != "submitted" {
			return ErrWorkflowNodeRunState
		}
		if _, err := tenantRole(ctx, tx, r.tenant, f.SubmittedBy); err != nil {
			return err
		}
		var project string
		if err := tx.QueryRow(ctx, `SELECT project_id FROM v2_workflow_runs WHERE id=$1`, runID).Scan(&project); err != nil {
			return err
		}
		if err := requireProject(ctx, tx, r.tenant, project); err != nil {
			return err
		}
		results := []FormTaskResult{}
		for _, item := range items {
			if utf8.RuneCountInString(item.Title) > 200 || item.Title == "" || utf8.RuneCountInString(item.Description) > 4000 {
				return fmt.Errorf("invalid task text")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO v2_workflow_finding_tasks(tenant_id,project_id,finding_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, r.tenant, project, item.Key); err != nil {
				return err
			}
			var id *string
			if err := tx.QueryRow(ctx, `SELECT task_id::text FROM v2_workflow_finding_tasks WHERE tenant_id=$1 AND project_id=$2 AND finding_key=$3 FOR UPDATE`, r.tenant, project, item.Key).Scan(&id); err != nil {
				return err
			}
			action := "reused"
			if id != nil {
				var status string
				if err := tx.QueryRow(ctx, `SELECT status FROM v2_tasks WHERE id=$1 FOR UPDATE`, *id).Scan(&status); err != nil {
					return err
				}
				if status == TaskDone || status == TaskCancelled {
					id = nil
				}
			}
			if id == nil {
				if item.Assignee == nil {
					return ErrInvalidAssignee
				}
				if err := requireAssignee(ctx, tx, r.tenant, *item.Assignee); err != nil {
					return err
				}
				var created string
				if err := tx.QueryRow(ctx, `INSERT INTO v2_tasks(tenant_id,title,description,project_id,assignee_type,assignee_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, r.tenant, item.Title, item.Description, project, item.Assignee.Type, item.Assignee.ID, f.SubmittedBy).Scan(&created); err != nil {
					return err
				}
				id = &created
				action = "created"
				if _, err := tx.Exec(ctx, `UPDATE v2_workflow_finding_tasks SET task_id=$4 WHERE tenant_id=$1 AND project_id=$2 AND finding_key=$3`, r.tenant, project, item.Key, *id); err != nil {
					return err
				}
			}
			results = append(results, FormTaskResult{Key: item.Key, TaskID: *id, Action: action})
		}
		payload, err := fold(r.payload, node, results)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(results)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE v2_workflow_forms SET results=$2 WHERE id=$1`, f.ID, encoded); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE v2_workflow_node_runs SET status='succeeded',output=$2,finished_at=now(),updated_at=now() WHERE id=$1`, node.ID, string(encoded)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE v2_workflow_runs SET payload=$2 WHERE id=$1`, runID, payload); err != nil {
			return err
		}
		return writeCurrentNodes(ctx, tx, runID, without(r.current, node.NodeID))
	})
}
