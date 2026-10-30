package store

import (
	"context"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// ScheduleScopes discovers persisted owners without depending on browser sessions.
// Paused configurations are included so already accepted work can still settle.
func (s *Store) ScheduleScopes(ctx context.Context) ([]execution.Scope, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT c.tenant_id,c.user_id FROM v2_schedule_configs c
		JOIN memberships m ON m.tenant_id=c.tenant_id AND m.user_id=c.user_id
		JOIN users u ON u.id=c.user_id JOIN tenants t ON t.id=c.tenant_id
		WHERE u.status='active' AND u.deleted_at IS NULL AND t.status='active'
		ORDER BY c.tenant_id,c.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []execution.Scope{}
	for rows.Next() {
		var item execution.Scope
		if err := rows.Scan(&item.TenantID, &item.UserID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetScheduleConfig checks current membership and creator ownership before an operation.
func (s *Store) GetScheduleConfig(ctx context.Context, tenant, user, id string) (ScheduleConfig, error) {
	ok, err := s.ScheduleMember(ctx, tenant, user)
	if err != nil {
		return ScheduleConfig{}, err
	}
	if !ok {
		return ScheduleConfig{}, ErrNotFound
	}
	return readSchedule(s.pool.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM v2_schedule_configs WHERE tenant_id=$1 AND user_id=$2 AND id=$3`, tenant, user, id))
}
