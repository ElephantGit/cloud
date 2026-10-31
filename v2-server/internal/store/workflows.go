package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

// SQL adapts internal/core/workflow.go's document/snapshot semantics to v2's
// existing store. Every public entry checks active membership at this boundary.
var ErrWorkflowNameConflict = errors.New("workflow name already exists")

const workflowColumns = `id, tenant_id, name, description, graph, version, created_at, updated_at, deleted_at`
const snapshotColumns = `id, tenant_id, workflow_id, version, name, graph, created_at`

func scanWorkflow(row pgx.Row) (workflow.Document, error) {
	var d workflow.Document
	err := row.Scan(&d.ID, &d.TenantID, &d.Name, &d.Description, &d.Graph, &d.Version, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
	return d, workflowStoreError(err)
}

func scanWorkflowSnapshot(row pgx.Row) (workflow.Snapshot, error) {
	var s workflow.Snapshot
	err := row.Scan(&s.ID, &s.TenantID, &s.WorkflowID, &s.Version, &s.Name, &s.Graph, &s.CreatedAt)
	return s, workflowStoreError(err)
}

func workflowStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" && p.ConstraintName == "v2_workflows_live_name" {
			return ErrWorkflowNameConflict
		}
		// JSON permits values PostgreSQL cannot store (for example U+0000 or a
		// numeric literal outside jsonb's range). Treat these as invalid input.
		switch p.Code {
		case "22P02", "22P05", "22003", "22021":
			return execution.ErrInvalidRequest
		}
	}
	return err
}

func validWorkflowID(id string) bool {
	var u pgtype.UUID
	return len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' && u.Scan(id) == nil && u.Valid
}

func workflowPageArgs(after string, limit int) (any, error) {
	if limit < 1 || limit > 100 || (after != "" && !validWorkflowID(after)) {
		return nil, execution.ErrInvalidRequest
	}
	if after == "" {
		return nil, nil
	}
	return after, nil
}

func (s *Store) GetWorkflow(ctx context.Context, scope execution.Scope, id string) (workflow.Document, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return workflow.Document{}, err
	}
	return readWorkflow(ctx, s.pool, scope.TenantID, id, false)
}

func readWorkflow(ctx context.Context, q executionQuerier, tenant, id string, lock bool) (workflow.Document, error) {
	if !validWorkflowID(id) {
		return workflow.Document{}, ErrNotFound
	}
	sql := `SELECT ` + workflowColumns + ` FROM v2_workflows WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`
	if lock {
		sql += ` FOR UPDATE`
	}
	return scanWorkflow(q.QueryRow(ctx, sql, tenant, id))
}

func (s *Store) ListWorkflows(ctx context.Context, scope execution.Scope, after string, limit int) (workflow.Page[workflow.Document], error) {
	out := workflow.Page[workflow.Document]{Items: []workflow.Document{}}
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return out, err
	}
	cursor, err := workflowPageArgs(after, limit)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+workflowColumns+` FROM v2_workflows
	 WHERE tenant_id=$1 AND deleted_at IS NULL AND ($2::uuid IS NULL OR id>$2) ORDER BY id LIMIT $3`, scope.TenantID, cursor, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanWorkflow(rows)
		if err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = out.Items[limit-1].ID
			break
		}
		out.Items = append(out.Items, d)
	}
	return out, rows.Err()
}

func (s *Store) GetWorkflowSnapshot(ctx context.Context, scope execution.Scope, id, snapshot string) (workflow.Snapshot, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return workflow.Snapshot{}, err
	}
	if _, err := readWorkflow(ctx, s.pool, scope.TenantID, id, false); err != nil {
		return workflow.Snapshot{}, err
	}
	return readWorkflowSnapshot(ctx, s.pool, scope.TenantID, id, snapshot)
}

func readWorkflowSnapshot(ctx context.Context, q executionQuerier, tenant, id, snapshot string) (workflow.Snapshot, error) {
	if !validWorkflowID(snapshot) {
		return workflow.Snapshot{}, ErrNotFound
	}
	return scanWorkflowSnapshot(q.QueryRow(ctx, `SELECT `+snapshotColumns+` FROM v2_workflow_snapshots
	 WHERE tenant_id=$1 AND workflow_id=$2 AND id=$3`, tenant, id, snapshot))
}

func (s *Store) ListWorkflowSnapshots(ctx context.Context, scope execution.Scope, id, after string, limit int) (workflow.Page[workflow.Snapshot], error) {
	out := workflow.Page[workflow.Snapshot]{Items: []workflow.Snapshot{}}
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return out, err
	}
	if _, err := readWorkflow(ctx, s.pool, scope.TenantID, id, false); err != nil {
		return out, err
	}
	cursor, err := workflowPageArgs(after, limit)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+snapshotColumns+` FROM v2_workflow_snapshots
	 WHERE tenant_id=$1 AND workflow_id=$2 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $4`, scope.TenantID, id, cursor, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanWorkflowSnapshot(rows)
		if err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = out.Items[limit-1].ID
			break
		}
		out.Items = append(out.Items, d)
	}
	return out, rows.Err()
}

func workflowName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return "", execution.ErrInvalidRequest
	}
	return name, nil
}

func workflowDraftGraph(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]json.RawMessage
	if len(raw) > workflow.MaxDocumentBytes || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, execution.ErrInvalidRequest
	}
	return raw, nil
}

func workflowVersion(d workflow.Document, version *int64) error {
	if version == nil {
		return workflow.ErrVersionRequired
	}
	if *version < 0 {
		return execution.ErrInvalidRequest
	}
	if *version != d.Version {
		return ErrConflict
	}
	return nil
}

func (s *Store) CreateWorkflow(ctx context.Context, scope execution.Scope, key string, in workflow.CreateDocument) (workflow.Document, error) {
	return workflowMutation(ctx, s, scope, key, "POST /workflows", in, func(tx pgx.Tx) (workflow.Document, error) {
		name, err := workflowName(in.Name)
		if err != nil {
			return workflow.Document{}, err
		}
		graph, err := workflowDraftGraph(in.Graph)
		if err != nil {
			return workflow.Document{}, err
		}
		description := ""
		if in.Description != nil {
			description = *in.Description
		}
		return scanWorkflow(tx.QueryRow(ctx, `INSERT INTO v2_workflows(tenant_id,name,description,graph)
		 VALUES($1,$2,$3,$4) RETURNING `+workflowColumns, scope.TenantID, name, description, graph))
	})
}

func (s *Store) UpdateWorkflow(ctx context.Context, scope execution.Scope, id string, in workflow.UpdateDocument) (workflow.Document, error) {
	return workflowMutation(ctx, s, scope, "", "PUT /workflows/"+id, in, func(tx pgx.Tx) (workflow.Document, error) {
		d, err := readWorkflow(ctx, tx, scope.TenantID, id, true)
		if err != nil {
			return d, err
		}
		if err := workflowVersion(d, in.Version); err != nil {
			return d, err
		}
		if d.Name, err = workflowName(in.Name); err != nil {
			return d, err
		}
		if in.Description != nil {
			d.Description = *in.Description
		}
		if len(in.Graph) != 0 {
			if d.Graph, err = workflowDraftGraph(in.Graph); err != nil {
				return d, err
			}
		}
		return scanWorkflow(tx.QueryRow(ctx, `UPDATE v2_workflows SET name=$3, description=$4, graph=$5,
		 version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, scope.TenantID, id, d.Name, d.Description, d.Graph))
	})
}

func (s *Store) DeleteWorkflow(ctx context.Context, scope execution.Scope, id, key string, in workflow.DocumentVersion) (workflow.Document, error) {
	return workflowMutation(ctx, s, scope, key, "DELETE /workflows/"+id, in, func(tx pgx.Tx) (workflow.Document, error) {
		d, err := readWorkflow(ctx, tx, scope.TenantID, id, true)
		if err != nil {
			return d, err
		}
		if err := workflowVersion(d, in.Version); err != nil {
			return d, err
		}
		return scanWorkflow(tx.QueryRow(ctx, `UPDATE v2_workflows SET deleted_at=now(), updated_at=now(), version=version+1
		 WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, scope.TenantID, id))
	})
}

func (s *Store) PublishWorkflow(ctx context.Context, scope execution.Scope, id, key string, in workflow.PublishDocument) (workflow.Snapshot, error) {
	return workflowMutation(ctx, s, scope, key, "POST /workflows/"+id+"/publish", in, func(tx pgx.Tx) (workflow.Snapshot, error) {
		d, err := readWorkflow(ctx, tx, scope.TenantID, id, true)
		if err != nil {
			return workflow.Snapshot{}, err
		}
		graph, err := workflow.FreezeGraph(d.Graph)
		if err != nil {
			return workflow.Snapshot{}, err
		}
		name := d.Name
		if in.Name != nil && *in.Name != "" {
			if name, err = workflowName(*in.Name); err != nil {
				return workflow.Snapshot{}, err
			}
		}
		return scanWorkflowSnapshot(tx.QueryRow(ctx, `INSERT INTO v2_workflow_snapshots(tenant_id,workflow_id,version,name,graph)
		 SELECT $1,$2,COALESCE(MAX(version),0)+1,$3,$4 FROM v2_workflow_snapshots WHERE tenant_id=$1 AND workflow_id=$2
		 RETURNING `+snapshotColumns, scope.TenantID, id, name, graph))
	})
}

func (s *Store) RestoreWorkflowSnapshot(ctx context.Context, scope execution.Scope, id, snapshot string, in workflow.DocumentVersion) (workflow.Document, error) {
	return workflowMutation(ctx, s, scope, "", "PUT /workflows/"+id+"/snapshots/"+snapshot+"/restore", in, func(tx pgx.Tx) (workflow.Document, error) {
		d, err := readWorkflow(ctx, tx, scope.TenantID, id, true)
		if err != nil {
			return d, err
		}
		if err := workflowVersion(d, in.Version); err != nil {
			return d, err
		}
		frozen, err := readWorkflowSnapshot(ctx, tx, scope.TenantID, id, snapshot)
		if err != nil {
			return d, err
		}
		return scanWorkflow(tx.QueryRow(ctx, `UPDATE v2_workflows SET graph=$3, version=version+1, updated_at=now()
		 WHERE tenant_id=$1 AND id=$2 RETURNING `+workflowColumns, scope.TenantID, id, frozen.Graph))
	})
}

// Admission and its response commit in one transaction. The unique key arbitrates
// concurrent retries, including retries after a lost response or process restart.
// Replay happens after current authorization, but before document/version lookup.
func workflowMutation[T any](ctx context.Context, s *Store, scope execution.Scope, key, action string, input any, write func(pgx.Tx) (T, error)) (T, error) {
	var out T
	keyRequired := strings.HasPrefix(action, "POST ") || strings.HasPrefix(action, "DELETE ")
	if keyRequired && (key == "" || len(key) > 200 || strings.TrimSpace(key) == "") {
		return out, execution.ErrInvalidRequest
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > workflow.MaxDocumentBytes {
		return out, execution.ErrInvalidRequest
	}
	// Decode RawMessage graphs too: reordering JSON object keys is the same request.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return out, execution.ErrInvalidRequest
	}
	raw, err = json.Marshal(canonical)
	if err != nil {
		return out, execution.ErrInvalidRequest
	}
	fingerprint := sha256.Sum256(append([]byte(action+"\n"), raw...))
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := checkExecutionScope(ctx, tx, scope); err != nil {
			return err
		}
		if keyRequired {
			tag, err := tx.Exec(ctx, `INSERT INTO v2_workflow_requests(tenant_id,user_id,key,fingerprint)
			 VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, scope.TenantID, scope.UserID, key, fingerprint[:])
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				var previous, response []byte
				if err := tx.QueryRow(ctx, `SELECT fingerprint,response FROM v2_workflow_requests
				 WHERE tenant_id=$1 AND user_id=$2 AND key=$3`, scope.TenantID, scope.UserID, key).Scan(&previous, &response); err != nil {
					return err
				}
				if !bytes.Equal(previous, fingerprint[:]) {
					return ErrKeyReused
				}
				return json.Unmarshal(response, &out)
			}
		}
		var err error
		if out, err = write(tx); err != nil {
			return err
		}
		if keyRequired {
			response, err := json.Marshal(out)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE v2_workflow_requests SET response=$4 WHERE tenant_id=$1 AND user_id=$2 AND key=$3`, scope.TenantID, scope.UserID, key, response)
			return err
		}
		return nil
	})
	return out, workflowStoreError(err)
}
