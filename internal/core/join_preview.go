package core

import "context"

// JoinLinkPreviewPath is the sole API route permitting a gateway without a user.
const JoinLinkPreviewPath = "/api/v1/join/links/preview"

// JoinLinkPreview reads only existing identities and link metadata. The router
// authenticates the gateway and any optional caller-bound user credential.
// Anonymous inspection deliberately never enters Public's identity provisioning.
func (s *Store) JoinLinkPreview(ctx context.Context, kind, token string, claims *Claims) (Object, error) {
	out, _, err := s.transactLocked(ctx, lockControlPlaneShared, nil, func(t *transaction) Object {
		require(kind == "invite" || kind == "apply", 400, "invalid_join_kind")
		row := lookupJoinLink(t, kind, token)
		state := "unavailable"
		if row != nil {
			state = row.S("state")
		}
		result := Object{"kind": kind, "state": state}
		if claims == nil || row == nil || !row.B("workspaceActive") {
			return result
		}
		u := t.one(`SELECT u.id,u.status,u.deleted_at FROM users u JOIN user_identities i ON i.user_id=u.id WHERE i.source=$1 AND i.subject=$2`, claims.Source, claims.Subject)
		if u == nil {
			return result
		}
		require(u.S("status") == "active" && u["deletedAt"] == nil, 403, "user_disabled")
		if activeMember(t, row.S("tenantId"), u.S("id")) {
			result["alreadyMember"] = true
		}
		if kind == "apply" {
			if pending := t.one("SELECT id FROM tenant_join_requests WHERE tenant_id=$1 AND user_id=$2 AND status='pending'", row.S("tenantId"), u.S("id")); pending != nil {
				result["applicationStatus"] = "pending"
			}
		}
		if state != "active" && !result.B("alreadyMember") && result.S("applicationStatus") == "" {
			return Object{"kind": kind, "state": state}
		}
		result["name"], result["slug"] = row.S("name"), row.S("slug")
		return result
	})
	return out, err
}

func lookupJoinLink(t *transaction, kind, token string) Object {
	table, used := "tenant_join_links", "false"
	if kind == "invite" {
		table, used = "tenant_invitations", "i.consumed_at IS NOT NULL"
	}
	// Only the fixed table/column fragments above enter SQL. The bearer token is
	// validated then bound as a digest; raw tokens never enter a query or log.
	return t.one(`SELECT i.*,tn.name,w.slug,
 (tn.status='active' AND tn.deleted_at IS NULL AND w.archived_at IS NULL) AS workspace_active,
 CASE WHEN tn.status<>'active' OR tn.deleted_at IS NOT NULL OR w.archived_at IS NOT NULL THEN 'unavailable'
 WHEN `+used+` THEN 'used' WHEN i.revoked_at IS NOT NULL THEN 'revoked'
 WHEN i.expires_at<=clock_timestamp() THEN 'expired' ELSE 'active' END AS state
 FROM `+table+` i JOIN tenants tn ON tn.id=i.tenant_id JOIN collab_workspaces w ON w.tenant_id=i.tenant_id WHERE i.token_hash=$1`, joinTokenDigest(token))
}

func requireActiveJoinLink(row Object) {
	state := "unavailable"
	if row != nil {
		state = row.S("state")
	}
	if state != "active" {
		rejectWith(404, "join_link_unavailable", Object{"reason": state})
	}
}
