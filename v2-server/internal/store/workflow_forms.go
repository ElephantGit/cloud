package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

// WorkflowForm is a frozen review and its versioned, editable response.
type WorkflowForm struct {
	ExistingTasks []WorkflowFormExistingTask `json:"existingTasks,omitempty"`
	SubmittedAt   *time.Time                 `json:"submittedAt"`
	ID            string                     `json:"id"`
	RunID         string                     `json:"runId"`
	NodeID        string                     `json:"nodeId"`
	ReviewerID    string                     `json:"reviewerId"`
	Config        json.RawMessage            `json:"config"`
	Findings      json.RawMessage            `json:"findings"`
	Report        string                     `json:"report"`
	Answers       json.RawMessage            `json:"answers"`
	Status        string                     `json:"status"`
	Version       int64                      `json:"version"`
	SubmittedBy   string                     `json:"submittedBy"`
	SubmitKey     string                     `json:"-"`
	Results       json.RawMessage            `json:"results"`
	CanEdit       bool                       `json:"canEdit"`
}

// WorkflowFormExistingTask is the live assignment a repeated finding will reuse.
type WorkflowFormExistingTask struct {
	Key      string        `json:"key"`
	TaskID   string        `json:"taskId"`
	Status   string        `json:"status"`
	Assignee *TaskAssignee `json:"assignee"`
}

func existingFormTasks(ctx context.Context, q dbTX, formID string) ([]WorkflowFormExistingTask, error) {
	rows, err := q.Query(ctx, `SELECT ft.finding_key,t.id::text,t.status,COALESCE(t.assignee_type,''),COALESCE(t.assignee_id::text,'')
 FROM v2_workflow_forms f JOIN v2_workflow_runs r ON r.id=f.run_id
 CROSS JOIN LATERAL jsonb_array_elements(f.findings) finding
 JOIN v2_workflow_finding_tasks ft ON ft.tenant_id=f.tenant_id AND ft.project_id=r.project_id AND ft.finding_key=finding->>'key'
 JOIN v2_tasks t ON t.id=ft.task_id
 WHERE f.id=$1 AND t.status IN ('pending','running','review')`, formID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowFormExistingTask{}
	for rows.Next() {
		var row WorkflowFormExistingTask
		var kind, id string
		if err := rows.Scan(&row.Key, &row.TaskID, &row.Status, &kind, &id); err != nil {
			return nil, err
		}
		if kind != "" {
			row.Assignee = &TaskAssignee{Type: kind, ID: id}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

const formColumns = `id::text,run_id::text,node_id,reviewer_id::text,config,findings,report,answers,status,version,COALESCE(submitted_by::text,''),submit_key,results,submitted_at`

func scanForm(row pgx.Row) (WorkflowForm, error) {
	var f WorkflowForm
	err := row.Scan(&f.ID, &f.RunID, &f.NodeID, &f.ReviewerID, &f.Config, &f.Findings, &f.Report, &f.Answers, &f.Status, &f.Version, &f.SubmittedBy, &f.SubmitKey, &f.Results, &f.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return f, err
}

// SetWorkflowStarter freezes the actor before any node begins.
func (s *Store) SetWorkflowStarter(ctx context.Context, tenant, runID, user string) error {
	if _, err := tenantRole(ctx, s.pool, tenant, user); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE v2_workflow_runs SET started_by=$3::uuid WHERE tenant_id=$1 AND id=$2 AND status='pending'`, tenant, runID, user)
	return err
}

// OpenWorkflowForm freezes evidence and parks its node in one transaction.
func (s *Store) OpenWorkflowForm(ctx context.Context, runID, nodeRunID string, in WorkflowForm) (WorkflowForm, error) {
	var out WorkflowForm
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := lockEngineRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if r.status != WorkflowStatusRunning {
			return ErrWorkflowRunState
		}
		n, err := lockWorkflowNodeRun(ctx, tx, r.tenant, runID, nodeRunID)
		if err != nil {
			return err
		}
		if n.NodeType != "form" || n.ScopeID != r.root {
			return ErrWorkflowNodeRunState
		}
		if old, e := scanForm(tx.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE run_id=$1`, runID)); e == nil {
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		if n.Status != WorkflowStatusRunning {
			return ErrWorkflowNodeRunState
		}
		if in.ReviewerID == "" {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(started_by::text,'') FROM v2_workflow_runs WHERE id=$1`, runID).Scan(&in.ReviewerID); err != nil {
				return err
			}
		}
		if err := requireActiveUser(ctx, tx, r.tenant, in.ReviewerID); err != nil {
			return err
		}
		out, err = scanForm(tx.QueryRow(ctx, `INSERT INTO v2_workflow_forms(id,tenant_id,run_id,node_id,reviewer_id,config,findings,report) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+formColumns, nodeRunID, r.tenant, runID, n.NodeID, in.ReviewerID, in.Config, in.Findings, in.Report))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE v2_workflow_node_runs SET status='pending',updated_at=now() WHERE id=$1`, nodeRunID)
		return err
	})
	return out, err
}

// WorkflowForm reads evidence through current tenant membership.
func (s *Store) WorkflowForm(ctx context.Context, tenant, user, runID string) (WorkflowForm, error) {
	role, err := tenantRole(ctx, s.pool, tenant, user)
	if err != nil {
		return WorkflowForm{}, err
	}
	f, err := scanForm(s.pool.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE tenant_id=$1 AND run_id=$2`, tenant, runID))
	if err != nil {
		return f, err
	}
	var live bool
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status='running' AND deleted_at IS NULL,status FROM v2_workflow_runs WHERE id=$1`, runID).Scan(&live, &status)
	if status == "cancelled" {
		f.Status = "cancelled"
	}
	f.CanEdit = live && f.Status == "open" && (f.ReviewerID == user || isAdministrator(role))
	if err == nil {
		f.ExistingTasks, err = existingFormTasks(ctx, s.pool, f.ID)
	}
	return f, err
}

// SaveWorkflowForm uses a caller-supplied pure validator and completion fold under
// the run lock, so cancel, edits and final submission cannot overtake each other.
func (s *Store) SaveWorkflowForm(ctx context.Context, tenant, user, runID string, version int64, key string, answers json.RawMessage, final bool, validate func(WorkflowForm) error, fold func([]byte, WorkflowForm) ([]byte, error)) (WorkflowForm, error) {
	var out WorkflowForm
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		role, err := tenantRole(ctx, tx, tenant, user)
		if err != nil {
			return err
		}
		if err := lockWorkflowRun(ctx, tx, tenant, runID); err != nil {
			return err
		}
		r, err := lockEngineRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		f, err := scanForm(tx.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE tenant_id=$1 AND run_id=$2 FOR UPDATE`, tenant, runID))
		if err != nil {
			return err
		}
		if f.ReviewerID != user && !isAdministrator(role) {
			return ErrForbidden
		}
		if final && f.Status == "submitted" && f.SubmitKey == key && f.SubmittedBy == user {
			out = f
			return nil
		}
		if r.status != WorkflowStatusRunning || f.Status != "open" {
			return ErrWorkflowRunState
		}
		if f.Version != version {
			return ErrConflict
		}
		f.Answers = answers
		f.ExistingTasks, err = existingFormTasks(ctx, tx, f.ID)
		if err != nil {
			return err
		}
		if final {
			var values []map[string]json.RawMessage
			if err := json.Unmarshal(f.Answers, &values); err != nil {
				return err
			}
			byKey := map[string]*TaskAssignee{}
			for _, task := range f.ExistingTasks {
				byKey[task.Key] = task.Assignee
			}
			for _, row := range values {
				var key, decision string
				if err := json.Unmarshal(row["key"], &key); err != nil {
					return err
				}
				if err := json.Unmarshal(row["decision"], &decision); err != nil {
					return err
				}
				if decision == "confirmed" && (len(row["assignee"]) == 0 || string(row["assignee"]) == "null") && byKey[key] != nil {
					row["assignee"], _ = json.Marshal(map[string]string{"type": byKey[key].Type, "id": byKey[key].ID})
				}
			}
			var err error
			answers, err = json.Marshal(values)
			if err != nil {
				return err
			}
			f.Answers = answers
		}
		if err := validate(f); err != nil {
			return err
		}
		if final {
			var rows []struct {
				Key      string `json:"key"`
				Decision string `json:"decision"`
				Assignee *struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"assignee"`
			}
			if err := json.Unmarshal(answers, &rows); err != nil {
				return err
			}
			for _, row := range rows {
				if row.Decision == "confirmed" && row.Assignee != nil {
					if err := requireAssignee(ctx, tx, tenant, TaskAssigneeRef{Type: row.Assignee.Type, ID: row.Assignee.ID}); err != nil {
						return err
					}
				}
			}
			if key == "" || len(key) > 200 {
				return fmt.Errorf("invalid submission key")
			}
			f.Status = "submitted"
			f.SubmittedBy = user
			f.SubmitKey = key
			payload, err := fold(r.payload, f)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE v2_workflow_runs SET payload=$2 WHERE id=$1`, runID, payload); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE v2_workflow_node_runs SET status='succeeded',output=$2,finished_at=now(),updated_at=now() WHERE id=$1 AND status='pending'`, f.ID, string(answers)); err != nil {
				return err
			}
			if err := writeCurrentNodes(ctx, tx, runID, without(r.current, f.NodeID)); err != nil {
				return err
			}
		}
		out, err = scanForm(tx.QueryRow(ctx, `UPDATE v2_workflow_forms SET answers=$2,status=$3,version=version+1,submitted_by=NULLIF($4,'')::uuid,submit_key=$5,submitted_at=CASE WHEN $3='submitted' THEN now() ELSE NULL END WHERE id=$1 RETURNING `+formColumns, f.ID, f.Answers, f.Status, f.SubmittedBy, f.SubmitKey))
		return err
	})
	return out, err
}

// WorkflowStarter returns the durable execution identity used by recovery.
func (s *Store) WorkflowStarter(ctx context.Context, runID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(started_by::text,'') FROM v2_workflow_runs WHERE id=$1`, runID).Scan(&id)
	return id, err
}

// RetryWorkflowFormTasks reopens only a failed deterministic action. Completed
// tasks, cancelled runs and failures in evidence collection cannot be replayed.
func (s *Store) RetryWorkflowFormTasks(ctx context.Context, tenant, user, runID string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		role, err := tenantRole(ctx, tx, tenant, user)
		if err != nil {
			return err
		}
		if err := lockWorkflowRun(ctx, tx, tenant, runID); err != nil {
			return err
		}
		f, err := scanForm(tx.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE run_id=$1`, runID))
		if err != nil {
			return err
		}
		if user != f.ReviewerID && !isAdministrator(role) {
			return ErrForbidden
		}
		r, err := lockEngineRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		if r.status == WorkflowStatusSucceeded {
			return nil
		}
		if r.status != WorkflowStatusFailed || f.Status != "submitted" {
			return ErrWorkflowRunState
		}
		var nodeID string
		err = tx.QueryRow(ctx, `UPDATE v2_workflow_node_runs SET status='running',error=NULL,finished_at=NULL WHERE run_id=$1 AND node_type='createTasks' AND status='failed' AND deleted_at IS NULL RETURNING node_id`, runID).Scan(&nodeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkflowNodeRunState
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE v2_workflow_runs SET status='running',error=NULL,finished_at=NULL WHERE id=$1`, runID); err != nil {
			return err
		}
		return writeCurrentNodes(ctx, tx, runID, []string{nodeID})
	})
}

// RecoverableFormRuns returns deterministic review tails, never Agent work.
func (s *Store) RecoverableFormRuns(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text FROM v2_workflow_runs r JOIN v2_workflow_forms f ON f.run_id=r.id WHERE r.status='running' AND r.deleted_at IS NULL AND f.status='submitted'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// WorkflowFormForEngine reads the frozen review for an already-admitted run.
// Task writes independently authorize the durable human submitter.
func (s *Store) WorkflowFormForEngine(ctx context.Context, runID string) (WorkflowForm, error) {
	return scanForm(s.pool.QueryRow(ctx, `SELECT `+formColumns+` FROM v2_workflow_forms WHERE run_id=$1`, runID))
}
