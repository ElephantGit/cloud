package core

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// Join tokens are generated with 32 random browser bytes, sent only in a
// request body, and stored solely as SHA-256 digests. The creator builds the
// share URL locally, so idempotency records never contain the raw token.
func joinTokenDigest(token string) []byte {
	require(len(token) == 43, 400, "invalid_join_token")
	raw, err := base64.RawURLEncoding.DecodeString(token)
	require(err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == token, 400, "invalid_join_token")
	sum := sha256.Sum256(raw)
	return sum[:]
}

// joinBeforeMembership distinguishes a fresh redemption from an idempotent
// replay so only the former invalidates online members after commit.
func joinBeforeMembership(t *transaction, r *PublicRequest, uid string) (out Object, status int, publishMemberEvent bool) {
	if r.Path == "/api/v1/me/join-requests" {
		return page(t, `SELECT j.id,j.tenant_id,j.user_id,j.link_id,j.status,j.created_at,j.decided_at,j.decided_by,j.version,tn.name
FROM tenant_join_requests j JOIN tenants tn ON tn.id=j.tenant_id WHERE j.user_id=$1`, []any{uid}, "j.id", r), 200, false
	}
	require(r.Key != "" && len(r.Key) <= 200, 400, "idempotency_key_required")
	hash := requestHash(r.Method, r.Path, r.Body)
	if old := t.one("SELECT * FROM join_idempotency_records WHERE user_id=$1 AND key=$2", uid, r.Key); old != nil {
		require(old.S("requestHash") == hash, 409, "idempotency_conflict")
		return old.O("response"), int(old.N("status")), false
	}
	switch r.Path {
	case "/api/v1/join/invitations/redeem":
		out, publishMemberEvent = redeemInvitation(t, r.Body.S("token"), uid)
		status = 200
	case "/api/v1/join/requests":
		out, status = requestJoin(t, r.Body.S("token"), uid), 201
	default:
		reject(404, "not_found")
	}
	t.exec("INSERT INTO join_idempotency_records(user_id,key,request_hash,response,status) VALUES($1,$2,$3,$4,$5)", uid, r.Key, hash, jsonText(out), status)
	return out, status, publishMemberEvent
}

func redeemInvitation(t *transaction, token, uid string) (Object, bool) {
	invite := lookupJoinLink(t, "invite", token)
	if invite != nil && invite.B("workspaceActive") && activeMember(t, invite.S("tenantId"), uid) {
		return joinedMembership(t, invite.S("tenantId"), uid), false
	}
	requireActiveJoinLink(invite)
	addMembership(t, invite.S("tenantId"), uid, "member")
	recordMemberJoin(t, invite.S("tenantId"), uid, "invitation", invite.S("createdBy"), invite.S("id"), "")
	t.exec("UPDATE tenant_invitations SET consumed_by=$2,consumed_at=clock_timestamp(),version=version+1 WHERE id=$1", invite.S("id"), uid)
	return joinedMembership(t, invite.S("tenantId"), uid), true
}

func joinedMembership(t *transaction, tid, uid string) Object {
	return t.one("SELECT m.tenant_id,m.user_id,m.role,m.status,m.version,tn.name FROM tenant_memberships m JOIN tenants tn ON tn.id=m.tenant_id WHERE m.tenant_id=$1 AND m.user_id=$2", tid, uid)
}

func requestJoin(t *transaction, token, uid string) Object {
	link := lookupJoinLink(t, "apply", token)
	requireActiveJoinLink(link)
	require(t.one("SELECT user_id FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2 AND status='active'", link.S("tenantId"), uid) == nil, 409, "already_member")
	if old := t.one("SELECT * FROM tenant_join_requests WHERE tenant_id=$1 AND user_id=$2 AND status='pending'", link.S("tenantId"), uid); old != nil {
		return old
	}
	id := newID()
	t.exec("INSERT INTO tenant_join_requests(id,tenant_id,user_id,link_id,status) VALUES($1,$2,$3,$4,'pending')", id, link.S("tenantId"), uid, link.S("id"))
	return t.one("SELECT * FROM tenant_join_requests WHERE id=$1", id)
}

// addMembership preserves durable references by reactivating an existing row
// instead of deleting or replacing it. Caller authorization is checked above.
func addMembership(t *transaction, tid, uid, role string) Object {
	old := t.one("SELECT * FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2", tid, uid)
	if old == nil {
		t.exec("INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES($1,$2,$3,'active')", tid, uid, role)
	} else if old.S("status") == "disabled" {
		t.exec("UPDATE tenant_memberships SET role=$3,status='active',version=version+1 WHERE tenant_id=$1 AND user_id=$2", tid, uid, role)
	}
	return t.one("SELECT * FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2", tid, uid)
}

func adminJoinRead(t *transaction, r *PublicRequest) Object {
	switch {
	case strings.HasSuffix(r.Path, "/invitations"):
		return page(t, `SELECT i.id,i.tenant_id,i.note,i.created_by,i.created_at,i.expires_at,i.revoked_at,i.consumed_by,i.consumed_at,i.version,
 c.display_name AS created_by_name,u.display_name AS consumed_by_name FROM tenant_invitations i JOIN users c ON c.id=i.created_by LEFT JOIN users u ON u.id=i.consumed_by WHERE i.tenant_id=$1`, []any{r.TenantID}, "i.id", r)
	case strings.HasSuffix(r.Path, "/join-links"):
		return page(t, `SELECT l.id,l.tenant_id,l.note,l.created_by,l.created_at,l.expires_at,l.revoked_at,l.version,c.display_name AS created_by_name,
 (SELECT count(*) FROM tenant_join_requests j WHERE j.link_id=l.id AND j.status='pending') AS pending_count,
 (SELECT count(*) FROM tenant_join_requests j WHERE j.link_id=l.id AND j.status='approved') AS approved_count
 FROM tenant_join_links l JOIN users c ON c.id=l.created_by WHERE l.tenant_id=$1`, []any{r.TenantID}, "l.id", r)
	default:
		return page(t, `SELECT j.id,j.tenant_id,j.user_id,j.link_id,j.status,j.created_at,j.decided_at,j.decided_by,j.version,u.display_name,
 l.note AS link_note,d.display_name AS decided_by_name FROM tenant_join_requests j JOIN users u ON u.id=j.user_id
 JOIN tenant_join_links l ON l.id=j.link_id LEFT JOIN users d ON d.id=j.decided_by WHERE j.tenant_id=$1`, []any{r.TenantID}, "j.id", r)
	}
}

func adminJoinWrite(t *transaction, r *PublicRequest, uid string) (out Object, status int) {
	switch {
	case r.Method == "POST" && strings.HasSuffix(r.Path, "/invitations"):
		digest := joinTokenDigest(r.Body.S("token"))
		// Token digests are globally unique per table. A deliberately reused
		// token is a client conflict, never an internal constraint failure.
		require(t.one("SELECT id FROM tenant_invitations WHERE token_hash=$1", digest) == nil, 409, "join_token_conflict")
		id := newID()
		t.exec("INSERT INTO tenant_invitations(id,tenant_id,token_hash,created_by,expires_at,note) VALUES($1,$2,$3,$4,now()+interval '7 days',$5)", id, r.TenantID, digest, uid, joinNote(r))
		return t.one("SELECT id,tenant_id,note,created_by,created_at,expires_at,version FROM tenant_invitations WHERE id=$1", id), 201
	case r.Method == "POST" && strings.HasSuffix(r.Path, "/join-links"):
		digest := joinTokenDigest(r.Body.S("token"))
		require(t.one("SELECT id FROM tenant_join_links WHERE token_hash=$1", digest) == nil, 409, "join_token_conflict")
		id := newID()
		t.exec("INSERT INTO tenant_join_links(id,tenant_id,token_hash,created_by,expires_at,note) VALUES($1,$2,$3,$4,now()+interval '30 days',$5)", id, r.TenantID, digest, uid, joinNote(r))
		return t.one("SELECT id,tenant_id,note,created_by,created_at,expires_at,version FROM tenant_join_links WHERE id=$1", id), 201
	case r.Method == "DELETE" && r.InvitationID != "":
		row := t.one("SELECT * FROM tenant_invitations WHERE id=$1 AND tenant_id=$2", r.InvitationID, r.TenantID)
		require(row != nil, 404, "not_found")
		version(row, r.Body.N("version"))
		t.exec("UPDATE tenant_invitations SET revoked_at=clock_timestamp(),version=version+1 WHERE id=$1 AND revoked_at IS NULL", r.InvitationID)
		return t.one("SELECT id,tenant_id,created_by,created_at,expires_at,revoked_at,consumed_by,consumed_at,version FROM tenant_invitations WHERE id=$1", r.InvitationID), 200
	case r.Method == "DELETE" && r.JoinLinkID != "":
		row := t.one("SELECT * FROM tenant_join_links WHERE id=$1 AND tenant_id=$2", r.JoinLinkID, r.TenantID)
		require(row != nil, 404, "not_found")
		version(row, r.Body.N("version"))
		t.exec("UPDATE tenant_join_links SET revoked_at=clock_timestamp(),version=version+1 WHERE id=$1 AND revoked_at IS NULL", r.JoinLinkID)
		return t.one("SELECT id,tenant_id,created_by,created_at,expires_at,revoked_at,version FROM tenant_join_links WHERE id=$1", r.JoinLinkID), 200
	case r.Method == "POST" && r.JoinRequestID != "":
		row := t.one("SELECT * FROM tenant_join_requests WHERE id=$1 AND tenant_id=$2", r.JoinRequestID, r.TenantID)
		require(row != nil, 404, "not_found")
		version(row, r.Body.N("version"))
		require(row.S("status") == "pending", 409, "request_already_decided")
		decision := "rejected"
		if strings.HasSuffix(r.Path, "/approve") {
			changed := !activeMember(t, r.TenantID, row.S("userId"))
			addMembership(t, r.TenantID, row.S("userId"), "member")
			if changed {
				recordMemberJoin(t, r.TenantID, row.S("userId"), "application", uid, "", row.S("id"))
			}
			decision = "approved"
		}
		t.exec("UPDATE tenant_join_requests SET status=$2,decided_at=clock_timestamp(),decided_by=$3,version=version+1 WHERE id=$1", r.JoinRequestID, decision, uid)
		return t.one("SELECT * FROM tenant_join_requests WHERE id=$1", r.JoinRequestID), 200
	}
	reject(404, "not_found")
	return nil, 0
}

func joinNote(r *PublicRequest) string {
	note := strings.TrimSpace(r.Body.S("note"))
	require(len([]rune(note)) <= 200, 400, "invalid_note")
	return note
}
