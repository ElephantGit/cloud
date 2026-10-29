package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrVersionConflict prevents a stale editor from replacing newer work.
var ErrVersionConflict = errors.New("version conflict")

// ErrWorkflowConflict reports a duplicate live name within a tenant.
var ErrWorkflowConflict = errors.New("workflow conflict")

// Workflow is the authored document, including unfinished graphs.
type Workflow struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenantId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Graph       json.RawMessage `json:"graph"`
	Version     int64           `json:"version"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	DeletedAt   *time.Time      `json:"deletedAt"`
}

// WorkflowSnapshot freezes a document independently of subsequent draft edits.
type WorkflowSnapshot struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenantId"`
	WorkflowID string          `json:"workflowId"`
	Version    int64           `json:"version"`
	Name       string          `json:"name"`
	Graph      json.RawMessage `json:"graph"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// WorkflowEdit distinguishes omitted optional fields from explicitly empty values.
type WorkflowEdit struct {
	Name        string
	Description *string
	Graph       json.RawMessage
	Version     int64
}

const workflowColumns = "id, tenant_id, name, description, graph, version, created_at, updated_at, deleted_at"
const snapshotColumns = "id, tenant_id, workflow_id, version, name, graph, created_at"

func scanWorkflow(row pgx.Row) (Workflow, error) {
	var w Workflow
	err := row.Scan(&w.ID, &w.TenantID, &w.Name, &w.Description, &w.Graph, &w.Version, &w.CreatedAt, &w.UpdatedAt, &w.DeletedAt)
	return w, workflowError(err)
}

func scanSnapshot(row pgx.Row) (WorkflowSnapshot, error) {
	var s WorkflowSnapshot
	err := row.Scan(&s.ID, &s.TenantID, &s.WorkflowID, &s.Version, &s.Name, &s.Graph, &s.CreatedAt)
	return s, workflowError(err)
}

func workflowError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "workflows_live_name" {
		return ErrWorkflowConflict
	}
	if err != nil {
		return fmt.Errorf("workflow storage: %w", err)
	}
	return nil
}

// Workflows returns a stable UUID-ordered page, with one extra row for the next cursor.
func (s *Store) Workflows(ctx context.Context, tenant, after string, limit int) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+workflowColumns+`
        FROM workflows WHERE tenant_id=$1 AND deleted_at IS NULL
        AND ($2::uuid IS NULL OR id>$2::uuid) ORDER BY id LIMIT $3`, tenant, cursorID(after), limit+1)
	if err != nil {
		return nil, workflowError(err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Workflow])
}

func cursorID(after string) any {
	if after == "" {
		return nil
	}
	return after
}

// Workflow reads only a live workflow in the authorized tenant.
func (s *Store) Workflow(ctx context.Context, tenant, id string) (Workflow, error) {
	return scanWorkflow(s.pool.QueryRow(ctx, "SELECT "+workflowColumns+
		" FROM workflows WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL", tenant, id))
}

// CreateWorkflow stores a whole draft; absence of graph means a blank document.
func (s *Store) CreateWorkflow(ctx context.Context, tenant string, edit WorkflowEdit) (Workflow, error) {
	graph := edit.Graph
	if graph == nil {
		graph = json.RawMessage("{}")
	}
	description := ""
	if edit.Description != nil {
		description = *edit.Description
	}
	return scanWorkflow(s.pool.QueryRow(ctx, `
        INSERT INTO workflows(tenant_id,name,description,graph) VALUES($1,$2,$3,$4)
        RETURNING `+workflowColumns, tenant, edit.Name, description, graph))
}

// UpdateWorkflow serializes edits and archive operations, checking the version under the lock.
func (s *Store) UpdateWorkflow(ctx context.Context, tenant, id string, edit WorkflowEdit) (Workflow, error) {
	var out Workflow
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := lockedWorkflow(ctx, tx, tenant, id, edit.Version)
		if err != nil {
			return err
		}
		description, graph := current.Description, current.Graph
		if edit.Description != nil {
			description = *edit.Description
		}
		if edit.Graph != nil {
			graph = edit.Graph
		}
		out, err = scanWorkflow(tx.QueryRow(ctx, `
            UPDATE workflows SET name=$3,description=$4,graph=$5,version=version+1,updated_at=now()
            WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, tenant, id, edit.Name, description, graph))
		return err
	})
	return out, err
}

func lockedWorkflow(ctx context.Context, tx pgx.Tx, tenant, id string, version int64) (Workflow, error) {
	w, err := scanWorkflow(tx.QueryRow(ctx, "SELECT "+workflowColumns+
		" FROM workflows WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE", tenant, id))
	if err != nil {
		return w, err
	}
	if w.Version != version {
		return w, ErrVersionConflict
	}
	return w, nil
}

// ArchiveWorkflow hides the document and its snapshots while retaining their data.
func (s *Store) ArchiveWorkflow(ctx context.Context, tenant, id string, version int64) (Workflow, error) {
	var out Workflow
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockedWorkflow(ctx, tx, tenant, id, version); err != nil {
			return err
		}
		var err error
		out, err = scanWorkflow(tx.QueryRow(ctx, `
            UPDATE workflows SET deleted_at=now(),updated_at=now(),version=version+1
            WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, tenant, id))
		return err
	})
	return out, err
}

// PublishWorkflow freezes exactly the validated draft version. Concurrent edits cannot slip
// unvalidated content into the snapshot; concurrent publishes receive distinct version numbers.
func (s *Store) PublishWorkflow(ctx context.Context, tenant, id, name string, version int64) (WorkflowSnapshot, error) {
	var out WorkflowSnapshot
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		w, err := lockedWorkflow(ctx, tx, tenant, id, version)
		if err != nil {
			return err
		}
		if name == "" {
			name = w.Name
		}
		out, err = scanSnapshot(tx.QueryRow(ctx, `
            INSERT INTO workflow_snapshots(tenant_id,workflow_id,version,name,graph)
            SELECT $1,$2,COALESCE(MAX(version),0)+1,$3,$4 FROM workflow_snapshots WHERE workflow_id=$2
            RETURNING `+snapshotColumns, tenant, id, name, w.Graph))
		return err
	})
	return out, err
}

// WorkflowSnapshots lists immutable versions of a live workflow.
func (s *Store) WorkflowSnapshots(ctx context.Context, tenant, id, after string, limit int) ([]WorkflowSnapshot, error) {
	if _, err := s.Workflow(ctx, tenant, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+snapshotColumns+` FROM workflow_snapshots
        WHERE tenant_id=$1 AND workflow_id=$2 AND ($3::uuid IS NULL OR id>$3::uuid)
        ORDER BY id LIMIT $4`, tenant, id, cursorID(after), limit+1)
	if err != nil {
		return nil, workflowError(err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WorkflowSnapshot])
}

// WorkflowSnapshot rejects foreign workflow and tenant identifiers.
func (s *Store) WorkflowSnapshot(ctx context.Context, tenant, id, snapshot string) (WorkflowSnapshot, error) {
	if _, err := s.Workflow(ctx, tenant, id); err != nil {
		return WorkflowSnapshot{}, err
	}
	return scanSnapshot(s.pool.QueryRow(ctx, "SELECT "+snapshotColumns+
		" FROM workflow_snapshots WHERE tenant_id=$1 AND workflow_id=$2 AND id=$3", tenant, id, snapshot))
}

// RestoreWorkflowSnapshot copies only the frozen graph, preserving the live name and description.
// The same row lock and version fence used by edits prevent a restore from overwriting newer work.
func (s *Store) RestoreWorkflowSnapshot(ctx context.Context, tenant, id, snapshot string, version int64) (Workflow, error) {
	var out Workflow
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := lockedWorkflow(ctx, tx, tenant, id, version); err != nil {
			return err
		}
		frozen, err := scanSnapshot(tx.QueryRow(ctx, "SELECT "+snapshotColumns+
			" FROM workflow_snapshots WHERE tenant_id=$1 AND workflow_id=$2 AND id=$3", tenant, id, snapshot))
		if err != nil {
			return err
		}
		out, err = scanWorkflow(tx.QueryRow(ctx, `UPDATE workflows SET graph=$3,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, tenant, id, frozen.Graph))
		return err
	})
	return out, err
}
