package core

func recordMemberJoin(t *transaction, tid, uid, method, actor, invitation, request string) {
	t.exec(`INSERT INTO membership_join_events(id,tenant_id,user_id,method,actor_user_id,invitation_id,request_id)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid)`, newID(), tid, uid, method, actor, invitation, request)
}

func activeMember(t *transaction, tid, uid string) bool {
	return t.one("SELECT user_id FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2 AND status='active'", tid, uid) != nil
}

func memberRoster(t *transaction, r *PublicRequest, uid string) Object {
	q := "SELECT m.user_id AS id,m.tenant_id,m.user_id,m.role,m.status,m.version,u.display_name FROM tenant_memberships m JOIN users u ON u.id=m.user_id WHERE m.tenant_id=$1"
	if membership(t, r.TenantID, uid, false).S("role") == "admin" {
		q = `SELECT m.user_id AS id,m.tenant_id,m.user_id,m.role,m.status,m.version,u.display_name,
 j.method AS join_method,j.joined_at,a.display_name AS actor_display_name
 FROM tenant_memberships m JOIN users u ON u.id=m.user_id
 LEFT JOIN LATERAL (SELECT * FROM membership_join_events e WHERE e.tenant_id=m.tenant_id AND e.user_id=m.user_id ORDER BY e.joined_at DESC,e.id DESC LIMIT 1) j ON true
 LEFT JOIN users a ON a.id=j.actor_user_id WHERE m.tenant_id=$1`
	}
	return page(t, q, []any{r.TenantID}, "m.user_id", r)
}

func memberJoinHistory(t *transaction, r *PublicRequest) Object {
	require(t.one("SELECT user_id FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2", r.TenantID, r.UserID) != nil, 404, "not_found")
	return page(t, `SELECT e.*,a.display_name AS actor_display_name,i.note AS invitation_note,l.note AS link_note
 FROM membership_join_events e JOIN users a ON a.id=e.actor_user_id
 LEFT JOIN tenant_invitations i ON i.id=e.invitation_id
 LEFT JOIN tenant_join_requests j ON j.id=e.request_id LEFT JOIN tenant_join_links l ON l.id=j.link_id
 WHERE e.tenant_id=$1 AND e.user_id=$2`, []any{r.TenantID, r.UserID}, "e.id", r)
}
