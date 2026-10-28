package integration

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wanglongan587/cloud/internal/core"
)

// TestInvitationNoOpPreservesFactsAndReplayCannotRestoreMembership covers
// specs/test-cases/cloud/tenancy/member-entry-provenance-and-link-preview.md#redemption-rechecks-state-and-no-op-never-consumes-an-invitation.
func TestInvitationNoOpPreservesFactsAndReplayCannotRestoreMembership(t *testing.T) {
	for _, role := range []string{"member", "admin"} {
		t.Run(role, func(t *testing.T) {
			f := setup(t)
			user, uid := f.addUser(t, "existing", "Existing")
			if role == "admin" {
				f.call("PUT", f.path("/members/"+uid), core.Object{"role": role, "status": "active", "version": 1}, "", 200)
			}
			token := joinToken('N')
			invite := f.call("POST", f.path("/invitations"), core.Object{"token": token}, "no-op-invite", 201)
			before := joinFacts(t, f)
			notices, cancel := f.store.Events.Subscribe(f.fixtureSpace().S("id"))
			t.Cleanup(cancel)
			var member core.Object
			for _, key := range []string{"no-op", "no-op", "fresh-no-op"} {
				member = joinCall(t, f, user, "POST", "/api/v1/join/invitations/redeem", key, core.Object{"token": token}, 200)
				if member.S("role") != role || member.S("status") != "active" || joinFacts(t, f) != before {
					t.Fatalf("no-op changed membership, token or provenance: %v", member)
				}
				select {
				case event := <-notices:
					t.Fatalf("no-op published a join notice: %v", event)
				default:
				}
			}
			f.call("PUT", f.path("/members/"+uid), core.Object{"role": role, "status": "disabled", "version": member.N("version")}, "", 200)
			joinCall(t, f, user, "POST", "/api/v1/join/invitations/redeem", "no-op", core.Object{"token": token}, 200)
			joinCall(t, f, user, "GET", f.path("/spaces"), "", nil, 403)
			if f.scalar("SELECT count(*) FROM tenant_invitations WHERE id=$1 AND consumed_at IS NOT NULL", invite.S("id")) != 0 {
				t.Fatal("replay consumed invitation after membership removal")
			}
			joinCall(t, f, joinUser("newcomer"), "POST", "/api/v1/join/invitations/redeem", "actual-join", core.Object{"token": token}, 200)
			joinCall(t, f, user, "POST", "/api/v1/join/invitations/redeem", "fresh-rejoin", core.Object{"token": token}, 404)
		})
	}
}

// joinFacts compares complete durable membership, link and provenance rows,
// including versions, rather than treating unchanged row counts as read-only.
func joinFacts(t *testing.T, f *fixture) string {
	t.Helper()
	var facts string
	must(t, f.store.Pool.QueryRow(`SELECT jsonb_build_object(
 'members', (SELECT jsonb_agg(to_jsonb(m) ORDER BY tenant_id,user_id) FROM tenant_memberships m),
 'invitations', (SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM tenant_invitations i),
 'links', (SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM tenant_join_links l),
 'requests', (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM tenant_join_requests r),
 'sources', (SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM membership_join_events e))::text`).Scan(&facts))
	return facts
}

// TestJoinPreviewPreservesCompleteBusinessFacts covers the read-only obligation
// in specs/test-cases/cloud/tenancy/member-entry-provenance-and-link-preview.md#preview-requires-the-exact-gateway-boundary-and-never-writes.
func TestJoinPreviewPreservesCompleteBusinessFacts(t *testing.T) {
	f := setup(t)
	gw := core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "gateway-a"}}
	for _, kind := range []string{"invite", "apply"} {
		token, path := joinToken(kind[0]), "/invitations"
		if kind == "apply" {
			path = "/join-links"
		}
		f.call("POST", f.path(path), core.Object{"token": token}, "read-only-"+kind, 201)
		before := joinFacts(t, f)
		users := f.scalar("SELECT count(*) FROM users")
		commands := f.scalar("SELECT count(*) FROM join_idempotency_records")
		for _, identity := range []*core.Claims{nil, &f.user} {
			for range 2 {
				out, status, err := f.client.Call(context.Background(), "POST", core.JoinLinkPreviewPath, "gateway", gw, identity, "", core.Object{"kind": kind, "token": token})
				must(t, err)
				if status != 200 || out.S("state") != "active" || (identity == nil && len(out) != 2) {
					t.Fatalf("unexpected preview: %d %v", status, out)
				}
			}
		}
		if joinFacts(t, f) != before || f.scalar("SELECT count(*) FROM users") != users || f.scalar("SELECT count(*) FROM join_idempotency_records") != commands {
			t.Fatal("preview changed durable business facts")
		}
	}
}

// TestJoinProvenanceFailureRollsBackAdmissionAndConsumption covers
// specs/test-cases/cloud/tenancy/member-entry-provenance-and-link-preview.md#join-provenance-commits-atomically-only-for-real-membership-changes.
func TestJoinProvenanceFailureRollsBackAdmissionAndConsumption(t *testing.T) {
	f := setup(t)
	user := joinUser("atomic-newcomer")
	identity := joinCall(t, f, user, "GET", "/api/v1/me", "", nil, 200)
	token := joinToken('F')
	f.call("POST", f.path("/invitations"), core.Object{"token": token}, "atomic-invite", 201)
	// Reject only new provenance writes, preserving existing fixture history.
	_, err := f.store.Pool.Exec("ALTER TABLE membership_join_events ADD CONSTRAINT test_reject_new_source CHECK (method <> 'invitation') NOT VALID")
	must(t, err)
	before := joinFacts(t, f)
	notices, cancel := f.store.Events.Subscribe(f.fixtureSpace().S("id"))
	t.Cleanup(cancel)
	joinCall(t, f, user, "POST", "/api/v1/join/invitations/redeem", "atomic-redeem", core.Object{"token": token}, 500)
	if joinFacts(t, f) != before || f.scalar("SELECT count(*) FROM join_idempotency_records WHERE user_id=$1", identity.S("id")) != 0 {
		t.Fatal("failed provenance left admission, consumption or replay facts")
	}
	select {
	case event := <-notices:
		t.Fatalf("rolled-back join published a notice: %v", event)
	default:
	}
	_, err = f.store.Pool.Exec("ALTER TABLE membership_join_events DROP CONSTRAINT test_reject_new_source")
	must(t, err)
	joinCall(t, f, user, "POST", "/api/v1/join/invitations/redeem", "atomic-redeem", core.Object{"token": token}, 200)
	if f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", identity.S("id")) != 1 {
		t.Fatal("retry did not atomically record exactly one join")
	}
}

// TestInvitationNoOpRacingAdmissionLeavesExactlyOneRealJoin verifies that a
// concurrent existing member never claims the single-use invitation.
func TestInvitationNoOpRacingAdmissionLeavesExactlyOneRealJoin(t *testing.T) {
	f := setup(t)
	existing, existingID := f.addUser(t, "existing-racer", "Existing")
	newcomer := joinUser("new-racer")
	newIdentity := joinCall(t, f, newcomer, "GET", "/api/v1/me", "", nil, 200)
	token := joinToken('R')
	invite := f.call("POST", f.path("/invitations"), core.Object{"token": token}, "race-invite", 201)
	before := f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", existingID)
	notices, cancel := f.store.Events.Subscribe(f.fixtureSpace().S("id"))
	t.Cleanup(cancel)
	start := make(chan struct{})
	type result struct {
		status int
		err    error
	}
	results := make(chan result, 2)
	for _, user := range []core.Claims{existing, newcomer} {
		go func() {
			<-start
			gw := core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "gateway-a"}}
			_, status, err := f.client.Call(context.Background(), "POST", "/api/v1/join/invitations/redeem", "gateway", gw, &user, "race-"+user.Subject, core.Object{"token": token})
			results <- result{status, err}
		}()
	}
	close(start)
	for range 2 {
		got := <-results
		must(t, got.err)
		if got.status != 200 {
			t.Fatalf("no-op blocked real admission: %d", got.status)
		}
	}
	if f.scalar("SELECT count(*) FROM tenant_invitations WHERE id=$1 AND consumed_by=$2", invite.S("id"), newIdentity.S("id")) != 1 ||
		f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", existingID) != before ||
		f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", newIdentity.S("id")) != 1 {
		t.Fatal("race did not preserve the single real admission and its provenance")
	}
	select {
	case event := <-notices:
		if event.Type != "space.member_updated" {
			t.Fatalf("wrong admission notice: %v", event)
		}
	default:
		t.Fatal("real admission did not publish its committed notice")
	}
	select {
	case event := <-notices:
		t.Fatalf("no-op published an extra notice: %v", event)
	default:
	}
}
