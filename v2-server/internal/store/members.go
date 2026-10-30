package store

import (
	"context"
	"fmt"
	"time"
)

// Member mirrors the contract's MemberListItem for a workspace roster entry.
//
// v2's memberships table has no membership id, version or join provenance: the row is keyed by
// (tenant, user) alone. ID therefore repeats the user id, Version stays 1, and the nullable
// provenance fields stay empty until Cloud's roster backs this route. Callers must not treat ID
// as a stable membership identity.
type Member struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenantId"`
	UserID           string     `json:"userId"`
	DisplayName      string     `json:"displayName"`
	Role             string     `json:"role"`
	Status           string     `json:"status"`
	Version          int64      `json:"version"`
	ActorDisplayName *string    `json:"actorDisplayName"`
	JoinMethod       *string    `json:"joinMethod"`
	JoinedAt         *time.Time `json:"joinedAt"`
}

// TenantMembers returns the roster of one workspace to a caller who belongs to it. The membership
// check runs first so a non-member cannot probe which users a workspace holds.
func (s *Store) TenantMembers(ctx context.Context, tenant, caller string) ([]Member, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT m.user_id, u.display_name, m.role, u.status
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1 AND u.deleted_at IS NULL
		ORDER BY u.display_name, m.user_id`, tenant)
	if e != nil {
		return nil, fmt.Errorf("list tenant members: %w", e)
	}
	defer rows.Close()
	items := make([]Member, 0)
	for rows.Next() {
		var m Member
		if e := rows.Scan(&m.UserID, &m.DisplayName, &m.Role, &m.Status); e != nil {
			return nil, fmt.Errorf("scan tenant member: %w", e)
		}
		m.ID, m.TenantID, m.Version = m.UserID, tenant, 1
		items = append(items, m)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate tenant members: %w", e)
	}
	return items, nil
}
