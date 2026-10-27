package integration

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wanglongan587/cloud/internal/core"
)

func TestJoinPreviewIsAnonymousReadOnlyAndCallerScoped(t *testing.T) {
	f := setup(t)
	token := joinToken('p')
	invite := f.call("POST", f.path("/invitations"), core.Object{"token": token, "note": "New teammate"}, "preview-invite", 201)
	users := f.scalar("SELECT count(*) FROM users")
	gw := core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "gateway-a"}}
	for _, identity := range []*core.Claims{nil, func() *core.Claims { u := joinUser("not-provisioned"); return &u }()} {
		out, status, err := f.client.Call(context.Background(), "POST", core.JoinLinkPreviewPath, "gateway", gw, identity, "", core.Object{"kind": "invite", "token": token})
		must(t, err)
		if status != 200 || out.S("state") != "active" || len(out) != 2 {
			t.Fatalf("unprovisioned preview must be minimal: %d %v", status, out)
		}
	}
	if got := f.scalar("SELECT count(*) FROM users"); got != users {
		t.Fatal("preview provisioned an identity")
	}
	if f.scalar("SELECT count(*) FROM tenant_invitations WHERE id=$1 AND consumed_at IS NOT NULL", invite.S("id")) != 0 {
		t.Fatal("preview consumed invitation")
	}
	preview := joinCall(t, f, f.user, "POST", core.JoinLinkPreviewPath, "", core.Object{"kind": "invite", "token": token}, 200)
	if !preview.B("alreadyMember") || preview.S("slug") == "" {
		t.Fatalf("existing member should enter directly: %v", preview)
	}
	unknown := joinCall(t, f, f.user, "POST", core.JoinLinkPreviewPath, "", core.Object{"kind": "invite", "token": joinToken('z')}, 200)
	if len(unknown) != 2 || unknown.S("state") != "unavailable" {
		t.Fatalf("unknown token leaked metadata: %v", unknown)
	}
	_, status, err := f.client.Call(context.Background(), "POST", core.JoinLinkPreviewPath, "controller", gw, nil, "", core.Object{"kind": "invite", "token": token})
	must(t, err)
	if status != 403 {
		t.Fatalf("non-gateway preview: %d", status)
	}
}

func TestJoinProvenanceIsAtomicPrivateAndRecordsReentry(t *testing.T) {
	f := setup(t)
	bob := joinUser("provenance-bob")
	token := joinToken('q')
	f.call("POST", f.path("/invitations"), core.Object{"token": token}, "provenance-invite", 201)
	member := joinCall(t, f, bob, "POST", "/api/v1/join/invitations/redeem", "provenance-redeem", core.Object{"token": token}, 200)
	uid := member.S("userId")
	historyPath := f.path("/members/" + uid + "/join-history")
	history := f.call("GET", historyPath, nil, "", 200)
	if len(history["items"].([]any)) != 1 || core.Object(history["items"].([]any)[0].(map[string]any)).S("method") != "invitation" {
		t.Fatalf("missing provenance: %v", history)
	}
	joinCall(t, f, bob, "GET", historyPath, "", nil, 403)
	joinCall(t, f, bob, "POST", "/api/v1/join/invitations/redeem", "provenance-redeem", core.Object{"token": token}, 200)
	if f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", uid) != 1 {
		t.Fatal("replay duplicated provenance")
	}
	f.call("PUT", f.path("/members/"+uid), core.Object{"role": "member", "status": "disabled", "version": member.N("version")}, "", 200)
	linkToken := joinToken('r')
	f.call("POST", f.path("/join-links"), core.Object{"token": linkToken, "note": "Community"}, "provenance-link", 201)
	request := joinCall(t, f, bob, "POST", "/api/v1/join/requests", "provenance-apply", core.Object{"token": linkToken}, 201)
	preview := joinCall(t, f, bob, "POST", core.JoinLinkPreviewPath, "", core.Object{"kind": "apply", "token": linkToken}, 200)
	if preview.S("applicationStatus") != "pending" {
		t.Fatalf("pending request missing from preview: %v", preview)
	}
	f.call("POST", f.path("/join-requests/"+request.S("id")+"/approve"), core.Object{"version": request.N("version")}, "provenance-approve", 200)
	if f.scalar("SELECT count(*) FROM membership_join_events WHERE user_id=$1", uid) != 2 {
		t.Fatal("reentry lost history")
	}
	roster := joinCall(t, f, bob, "GET", f.path("/members"), "", nil, 200)
	for _, item := range roster["items"].([]any) {
		if _, present := item.(map[string]any)["joinMethod"]; present {
			t.Fatal("ordinary roster disclosed provenance")
		}
	}
}

func TestAnonymousJoinPreviewTerminalStatesDoNotExposePeople(t *testing.T) {
	f := setup(t)
	gw := core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "gateway-a"}}
	for _, state := range []string{"expired", "revoked", "used"} {
		token := joinToken(state[0])
		invite := f.call("POST", f.path("/invitations"), core.Object{"token": token}, "terminal-"+state, 201)
		switch state {
		case "expired":
			_, err := f.store.Pool.Exec("UPDATE tenant_invitations SET created_at=now()-interval '8 days',expires_at=now()-interval '7 days' WHERE id=$1", invite.S("id"))
			must(t, err)
		case "revoked":
			f.call("DELETE", f.path("/invitations/"+invite.S("id")), core.Object{"version": 1}, "terminal-revoke", 200)
		case "used":
			joinCall(t, f, joinUser("terminal-new-member"), "POST", "/api/v1/join/invitations/redeem", "terminal-use", core.Object{"token": token}, 200)
		}
		out, status, err := f.client.Call(context.Background(), "POST", core.JoinLinkPreviewPath, "gateway", gw, nil, "", core.Object{"kind": "invite", "token": token})
		must(t, err)
		if status != 200 || out.S("state") != state || len(out) != 2 {
			t.Fatalf("anonymous %s leaked detail: %d %v", state, status, out)
		}
		failure := joinCall(t, f, joinUser("terminal-outsider"), "POST", "/api/v1/join/invitations/redeem", "fail-"+state, core.Object{"token": token}, 404)
		if failure.O("params").S("reason") != state {
			t.Fatalf("submission lost precise reason: %v", failure)
		}
	}
}
