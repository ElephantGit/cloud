package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

const workflowRunColumns = `id,tenant_id,user_id,snapshot_id,definition,checkpoint,revision,created_at,updated_at,finished_at,cancel_requested_at`

func scanWorkflowRun(row pgx.Row) (workflow.Record, error) {
	var out workflow.Record
	err := row.Scan(&out.ID, &out.Scope.TenantID, &out.Scope.UserID, &out.SnapshotID, &out.Definition, &out.Checkpoint, &out.Revision, &out.CreatedAt, &out.UpdatedAt, &out.FinishedAt, &out.CancelRequestedAt)
	return out, workflowStoreError(err)
}

// CreateWorkflowRun freezes a published graph and the server-resolved bindings in
// the existing idempotency transaction. It does not dispatch execution. Replays
// return the original admission response; the host must claim the persisted row
// through BeginWorkflowRun, never launch from that possibly stale response.
func (s *Store) CreateWorkflowRun(ctx context.Context, scope execution.Scope, id, key string, in workflow.Admission) (workflow.Record, error) {
	return workflowMutation(ctx, s, scope, key, "POST /workflows/"+id+"/runs", in, func(tx pgx.Tx) (workflow.Record, error) {
		var out workflow.Record
		if _, err := readWorkflow(ctx, tx, scope.TenantID, id, true); err != nil {
			return out, err
		}
		snapshot, err := readWorkflowSnapshot(ctx, tx, scope.TenantID, id, in.SnapshotID)
		if err != nil {
			return out, err
		}
		var project string
		if err := tx.QueryRow(ctx, `SELECT id FROM v2_projects WHERE tenant_id=$1 AND id=$2`, scope.TenantID, in.ProjectID).Scan(&project); err != nil {
			return out, err
		}
		if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text,now()`).Scan(&out.ID, &out.CreatedAt); err != nil {
			return out, err
		}
		def := workflow.Definition{ID: out.ID, WorkflowID: id, ProjectID: project, CreatedAt: out.CreatedAt, Input: in.Input, Executors: in.Executors, RolePrompts: in.RolePrompts}
		if err := json.Unmarshal(snapshot.Graph, &def.Graph); err != nil {
			return out, err
		}
		if _, err := workflow.NewMachine(def); err != nil {
			return out, errors.Join(execution.ErrInvalidRequest, err)
		}
		checkpoint := workflow.Checkpoint{State: workflow.State{Status: "pending", Nodes: workflow.Object{}}}
		return scanWorkflowRun(tx.QueryRow(ctx, `INSERT INTO v2_workflow_runs
		 (id,tenant_id,user_id,workflow_id,snapshot_id,definition,checkpoint,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8) RETURNING `+workflowRunColumns,
			out.ID, scope.TenantID, scope.UserID, id, snapshot.ID, def, checkpoint, out.CreatedAt))
	})
}

// Runs contain prompts and output and are scoped to their initiator, matching the
// public execution service. A tenant-shared document does not grant another user
// access to its creator's execution content. Archiving never hides worker cleanup.
func readWorkflowRun(ctx context.Context, q executionQuerier, scope execution.Scope, workflowID, id string, lock bool) (workflow.Record, error) {
	if !validWorkflowID(workflowID) || !validWorkflowID(id) {
		return workflow.Record{}, ErrNotFound
	}
	sql := `SELECT ` + workflowRunColumns + ` FROM v2_workflow_runs
	 WHERE tenant_id=$1 AND user_id=$2 AND workflow_id=$3 AND id=$4`
	if lock {
		sql += ` FOR UPDATE`
	}
	return scanWorkflowRun(q.QueryRow(ctx, sql, scope.TenantID, scope.UserID, workflowID, id))
}

func (s *Store) GetWorkflowRun(ctx context.Context, scope execution.Scope, workflowID, id string) (workflow.Record, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return workflow.Record{}, err
	}
	return readWorkflowRun(ctx, s.pool, scope, workflowID, id, false)
}

func (s *Store) ListWorkflowRuns(ctx context.Context, scope execution.Scope, workflowID, after string, limit int) (workflow.Page[workflow.Record], error) {
	out := workflow.Page[workflow.Record]{Items: []workflow.Record{}}
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return out, err
	}
	if !validWorkflowID(workflowID) {
		return out, ErrNotFound
	}
	cursor, err := workflowPageArgs(after, limit)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+workflowRunColumns+` FROM v2_workflow_runs
	 WHERE tenant_id=$1 AND user_id=$2 AND workflow_id=$3 AND ($4::uuid IS NULL OR id>$4) ORDER BY id LIMIT $5`, scope.TenantID, scope.UserID, workflowID, cursor, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanWorkflowRun(rows)
		if err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = out.Items[limit-1].ID
			break
		}
		out.Items = append(out.Items, r)
	}
	return out, rows.Err()
}

// BeginWorkflowRun claims a never-started admission exactly once. A running row
// is not a resumable machine, even when no in-process worker currently owns it.
func (s *Store) BeginWorkflowRun(ctx context.Context, scope execution.Scope, workflowID, id string) (workflow.Record, error) {
	return s.workflowRunWrite(ctx, scope, workflowID, id, func(tx pgx.Tx, r workflow.Record) (workflow.Record, error) {
		if r.Checkpoint.State.Status != "pending" || r.CancelRequestedAt != nil {
			return r, ErrConflict
		}
		r.Checkpoint.State.Status = "running"
		return writeWorkflowCheckpoint(ctx, tx, r, r.Checkpoint)
	})
}

// CommitWorkflowRun is the runner's durable Commit adapter. The caller advances
// its expected revision only after a successful return. Unknown write outcomes
// stop dispatch; they must be read back, not repaired by starting another worker.
func (s *Store) CommitWorkflowRun(ctx context.Context, scope execution.Scope, workflowID, id string, revision int64, checkpoint workflow.Checkpoint) (workflow.Record, error) {
	return s.workflowRunWrite(ctx, scope, workflowID, id, func(tx pgx.Tx, r workflow.Record) (workflow.Record, error) {
		if r.Revision != revision || r.FinishedAt != nil || r.Checkpoint.State.Status == "pending" {
			return r, ErrConflict
		}
		if err := validWorkflowCheckpoint(r, checkpoint); err != nil {
			return r, err
		}
		previous := r.Checkpoint.Active
		if previous != nil && checkpoint.Active == nil {
			caller, _ := executionCaller(previous.Caller)
			if _, err := tx.Exec(ctx, `UPDATE v2_workflow_attempts SET settled_at=now() WHERE run_id=$1 AND caller=$2 AND settled_at IS NULL`, id, caller); err != nil {
				return r, err
			}
		}
		if checkpoint.Active != nil {
			caller, _ := executionCaller(checkpoint.Active.Caller)
			if previous == nil {
				_, err := tx.Exec(ctx, `INSERT INTO v2_workflow_attempts(run_id,sequence,caller,attempt,receipt)
				 SELECT $1,COALESCE(MAX(sequence),0)+1,$2,$3,$4 FROM v2_workflow_attempts WHERE run_id=$1`, id, caller, checkpoint.Active, checkpoint.Ref)
				if err != nil {
					return r, executionError(err)
				}
			} else if _, err := tx.Exec(ctx, `UPDATE v2_workflow_attempts SET receipt=$3 WHERE run_id=$1 AND caller=$2 AND settled_at IS NULL`, id, caller, checkpoint.Ref); err != nil {
				return r, err
			}
		}
		return writeWorkflowCheckpoint(ctx, tx, r, checkpoint)
	})
}

func validWorkflowCheckpoint(r workflow.Record, c workflow.Checkpoint) error {
	if c.State.Nodes == nil {
		return execution.ErrInvalidRequest
	}
	switch c.State.Status {
	case "running", "succeeded", "failed", "cancelled":
	default:
		return execution.ErrInvalidRequest
	}
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > workflow.MaxDocumentBytes {
		return execution.ErrInvalidRequest
	}
	if c.Active == nil {
		if c.Ref != (execution.Ref{}) {
			return execution.ErrInvalidRequest
		}
		return nil
	}
	a := c.Active
	if c.State.Status != "running" || a.Caller.Kind != execution.CallerWorkflow || a.Caller.ID != r.ID || a.Caller.NodeID == "" || a.ExecutorID == "" || a.ExecutorID != r.Definition.Executors[a.Caller.NodeID] || (c.Ref.SessionID == "") != (c.Ref.TurnID == "") {
		return execution.ErrInvalidRequest
	}
	if _, err := executionCaller(a.Caller); err != nil {
		return err
	}
	if old := r.Checkpoint.Active; old != nil {
		if !reflect.DeepEqual(old, a) || (r.Checkpoint.Ref != (execution.Ref{}) && r.Checkpoint.Ref != c.Ref) {
			return ErrConflict
		}
	} else if c.Ref != (execution.Ref{}) {
		// Start is forbidden until a receipt-free checkpoint has committed.
		return ErrConflict
	}
	return nil
}

func writeWorkflowCheckpoint(ctx context.Context, tx pgx.Tx, r workflow.Record, c workflow.Checkpoint) (workflow.Record, error) {
	return scanWorkflowRun(tx.QueryRow(ctx, `UPDATE v2_workflow_runs SET checkpoint=$2,revision=revision+1,updated_at=now(),
	 finished_at=CASE WHEN $3='running' THEN NULL ELSE now() END WHERE id=$1 RETURNING `+workflowRunColumns, r.ID, c, c.State.Status))
}

func (s *Store) workflowRunWrite(ctx context.Context, scope execution.Scope, workflowID, id string, write func(pgx.Tx, workflow.Record) (workflow.Record, error)) (workflow.Record, error) {
	var out workflow.Record
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := checkExecutionScope(ctx, tx, scope); err != nil {
			return err
		}
		r, err := readWorkflowRun(ctx, tx, scope, workflowID, id, true)
		if err != nil {
			return err
		}
		out, err = write(tx, r)
		return err
	})
	return out, workflowStoreError(err)
}

// RequestWorkflowRunCancel records intent without claiming execution has stopped.
// It does not advance the worker revision, so cancellation cannot invalidate the
// worker's in-flight receipt write. The host must signal/join its worker separately.
func (s *Store) RequestWorkflowRunCancel(ctx context.Context, scope execution.Scope, workflowID, id string) (workflow.Record, error) {
	return s.workflowRunWrite(ctx, scope, workflowID, id, func(tx pgx.Tx, r workflow.Record) (workflow.Record, error) {
		if r.CancelRequestedAt != nil {
			return r, nil
		}
		if r.FinishedAt != nil {
			return r, ErrConflict
		}
		return scanWorkflowRun(tx.QueryRow(ctx, `UPDATE v2_workflow_runs SET cancel_requested_at=now(),updated_at=now() WHERE id=$1 RETURNING `+workflowRunColumns, id))
	})
}

func (s *Store) ListWorkflowAttempts(ctx context.Context, scope execution.Scope, workflowID, id string, after int64, limit int) (workflow.Page[workflow.AttemptRecord], error) {
	out := workflow.Page[workflow.AttemptRecord]{Items: []workflow.AttemptRecord{}}
	if _, err := s.GetWorkflowRun(ctx, scope, workflowID, id); err != nil {
		return out, err
	}
	if after < 0 || limit < 1 || limit > 100 {
		return out, execution.ErrInvalidRequest
	}
	rows, err := s.pool.Query(ctx, `SELECT sequence,attempt,receipt,created_at,settled_at FROM v2_workflow_attempts WHERE run_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`, id, after, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a workflow.AttemptRecord
		if err := rows.Scan(&a.Sequence, &a.Attempt, &a.Ref, &a.CreatedAt, &a.SettledAt); err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = strconv.FormatInt(out.Items[limit-1].Sequence, 10)
			break
		}
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}

// InterruptWorkflowRun is used only after the caller has joined the old worker
// and confirmed shutdown through execution.Service. A pending admission is also
// interrupted, not replayed. A saved receipt remains in attempt history.
func (s *Store) InterruptWorkflowRun(ctx context.Context, scope execution.Scope, workflowID, id string, revision int64) (workflow.Record, error) {
	return s.workflowRunWrite(ctx, scope, workflowID, id, func(tx pgx.Tx, r workflow.Record) (workflow.Record, error) {
		if r.FinishedAt != nil {
			return r, nil
		}
		if r.Revision != revision {
			return r, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE v2_workflow_attempts SET settled_at=now() WHERE run_id=$1 AND settled_at IS NULL`, id); err != nil {
			return r, err
		}
		return writeWorkflowCheckpoint(ctx, tx, r, workflow.InterruptCheckpoint(r.Checkpoint, r.CancelRequestedAt != nil))
	})
}

// RecoverWorkflowRuns is startup maintenance, after the public execution runtime
// has stopped/reconciled ALL old sessions and before any workflow worker starts.
// It intentionally does not require a browser session or current membership:
// revoking an owner must not leave their old run eligible for replay. Missing
// receipts remain resolvable by their retained caller identity; no new Start is
// attempted. Do not invoke this while a workflow host is running.
func (s *Store) RecoverWorkflowRuns(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM v2_workflow_runs WHERE finished_at IS NULL ORDER BY id FOR UPDATE`)
		if err != nil {
			return err
		}
		// Keep only identities in memory; each frozen graph/checkpoint may be large.
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			r, err := scanWorkflowRun(tx.QueryRow(ctx, `SELECT `+workflowRunColumns+` FROM v2_workflow_runs WHERE id=$1`, id))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE v2_workflow_attempts SET settled_at=now() WHERE run_id=$1 AND settled_at IS NULL`, r.ID); err != nil {
				return err
			}
			if _, err := writeWorkflowCheckpoint(ctx, tx, r, workflow.InterruptCheckpoint(r.Checkpoint, r.CancelRequestedAt != nil)); err != nil {
				return err
			}
		}
		return nil
	})
}
