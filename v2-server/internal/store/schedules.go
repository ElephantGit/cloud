package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// ErrConflict indicates a stale resource version.
var ErrConflict = errors.New("version conflict")

// ScheduleConfig is a saved configuration; it does not imply execution is available.
type ScheduleConfig struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	ProjectID string          `json:"projectId"`
	Target    json.RawMessage `json:"target"`
	Rule      json.RawMessage `json:"rule"`
	Enabled   bool            `json:"enabled"`
	Version   int64           `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Runs      []any           `json:"runs"`
}

// ScheduleInput contains mutable configuration only.
type ScheduleInput struct {
	Name      string          `json:"name"`
	ProjectID string          `json:"projectId"`
	Target    json.RawMessage `json:"target"`
	Rule      json.RawMessage `json:"rule"`
	Version   int64           `json:"version"`
}

const scheduleColumns = `id,name,project_id,target,rule,enabled,version,created_at,updated_at`

func readSchedule(row pgx.Row) (ScheduleConfig, error) {
	var v ScheduleConfig
	e := row.Scan(&v.ID, &v.Name, &v.ProjectID, &v.Target, &v.Rule, &v.Enabled, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.Runs = []any{}
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, e
}

// ScheduleMember checks current active membership, including disabled users and tenants.
func (s *Store) ScheduleMember(ctx context.Context, tenant, user string) (bool, error) {
	var ok bool
	e := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id JOIN tenants t ON t.id=m.tenant_id WHERE m.tenant_id=$1 AND m.user_id=$2 AND u.status='active' AND u.deleted_at IS NULL AND t.status='active')`, tenant, user).Scan(&ok)
	return ok, e
}

// ScheduleConfigs returns only the caller's saved configurations in the tenant.
func (s *Store) ScheduleConfigs(ctx context.Context, tenant, user string) ([]ScheduleConfig, error) {
	rows, e := s.pool.Query(ctx, `SELECT `+scheduleColumns+` FROM v2_schedule_configs WHERE tenant_id=$1 AND user_id=$2 ORDER BY created_at DESC,id DESC`, tenant, user)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ScheduleConfig{}
	for rows.Next() {
		v, e := readSchedule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateScheduleConfig saves configuration without dispatching any execution.
func (s *Store) CreateScheduleConfig(ctx context.Context, tenant, user string, in ScheduleInput) (ScheduleConfig, error) {
	return readSchedule(s.pool.QueryRow(ctx, `INSERT INTO v2_schedule_configs(tenant_id,user_id,name,project_id,target,rule) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+scheduleColumns, tenant, user, in.Name, in.ProjectID, in.Target, in.Rule))
}

// UpdateScheduleConfig uses a row lock so ownership, version checks and writes are atomic.
// A nil enabled changes configuration; otherwise only the enabled flag changes.
func (s *Store) UpdateScheduleConfig(ctx context.Context, tenant, user, id string, in ScheduleInput, enabled *bool) (ScheduleConfig, error) {
	var result ScheduleConfig
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		old, e := readSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM v2_schedule_configs WHERE tenant_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, tenant, user, id))
		if e != nil {
			return e
		}
		if old.Version != in.Version {
			return ErrConflict
		}
		if enabled != nil {
			result, e = readSchedule(tx.QueryRow(ctx, `UPDATE v2_schedule_configs SET enabled=$2,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+scheduleColumns, id, *enabled))
		} else {
			result, e = readSchedule(tx.QueryRow(ctx, `UPDATE v2_schedule_configs SET name=$2,project_id=$3,target=$4,rule=$5,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+scheduleColumns, id, in.Name, in.ProjectID, in.Target, in.Rule))
		}
		return e
	})
	return result, err
}
