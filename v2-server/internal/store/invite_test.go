package store_test

import (
	"errors"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestSeedUsersIsIdempotentAndKeepsInvitedAccounts(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	preset := store.Preset{Users: []store.PresetUser{
		{Email: "owner@demo.test", DisplayName: "Owner", Role: store.RoleOwner},
		{Email: "alice@demo.test", DisplayName: "Alice", Role: store.RoleMember},
	}}
	if e := f.store.SeedUsers(ctx, preset); e != nil {
		t.Fatal(e)
	}
	owner, e := f.store.UserByEmail(ctx, "owner@demo.test")
	if e != nil {
		t.Fatal(e)
	}
	invited, e := f.store.InviteMember(ctx, tenantOf(t, f, owner.ID), owner.ID, store.Invite{Email: "bob@demo.test"})
	if e != nil {
		t.Fatal(e)
	}

	// A restart with a renamed account refreshes it and leaves the invited account alone.
	preset.Users[1].DisplayName = "Alice Chen"
	if e := f.store.SeedUsers(ctx, preset); e != nil {
		t.Fatal(e)
	}
	alice, e := f.store.UserByEmail(ctx, "alice@demo.test")
	if e != nil || alice.DisplayName != "Alice Chen" || alice.ID == "" {
		t.Fatalf("alice = %+v %v", alice, e)
	}
	if _, e := f.store.UserByEmail(ctx, "bob@demo.test"); e != nil {
		t.Fatalf("invited account gone after reseed: %v", e)
	}
	if invited.Role != store.RoleMember {
		t.Fatalf("invited role = %q", invited.Role)
	}
	if _, e := f.store.UserByEmail(ctx, "nobody@demo.test"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("unknown address = %v", e)
	}
}

func TestRetirePresetUsersHidesThemAndKeepsInvitedMembers(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	preset := store.Preset{Users: []store.PresetUser{
		{Email: "owner@demo.test", DisplayName: "Owner", Role: store.RoleOwner},
		{Email: "alice@demo.test", DisplayName: "Alice", Role: store.RoleMember},
		{Email: "bob@demo.test", DisplayName: "Bob", Role: store.RoleMember},
	}}
	if e := f.store.SeedUsers(ctx, preset); e != nil {
		t.Fatal(e)
	}
	owner, e := f.store.UserByEmail(ctx, "owner@demo.test")
	if e != nil {
		t.Fatal(e)
	}
	tenant := tenantOf(t, f, owner.ID)
	if _, e := f.store.InviteMember(ctx, tenant, owner.ID, store.Invite{Email: "carol@demo.test", DisplayName: "Carol"}); e != nil {
		t.Fatal(e)
	}

	if e := f.store.RetireUsers(ctx, []store.RetiredUser{{Email: "alice@demo.test"}, {Email: "bob@demo.test"}}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.UserByEmail(ctx, "alice@demo.test"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("retired user still signs in: %v", e)
	}
	members, e := f.store.TenantMembers(ctx, tenant, owner.ID)
	if e != nil {
		t.Fatal(e)
	}
	if len(members) != 2 || members[0].DisplayName != "Carol" || members[1].DisplayName != "Owner" {
		t.Fatalf("roster after retirement = %+v", members)
	}

	// The cleanup is one-time: re-enabling the address later (for example after a fresh invite)
	// and restarting the server must not retire it again.
	if _, e := f.db.Exec(ctx, `
		UPDATE users SET status = 'active', deleted_at = NULL, version = version + 1
		WHERE source = 'email' AND subject = 'alice@demo.test'`); e != nil {
		t.Fatal(e)
	}
	if e := f.store.RetireUsers(ctx, []store.RetiredUser{{Email: "alice@demo.test"}}); e != nil {
		t.Fatal(e)
	}
	var active bool
	if e := f.db.QueryRow(ctx, `
		SELECT status = 'active' AND deleted_at IS NULL
		FROM users WHERE source = 'email' AND subject = 'alice@demo.test'`).Scan(&active); e != nil {
		t.Fatal(e)
	}
	if !active {
		t.Fatal("one-time retirement ran again")
	}
}

func TestRetiredAccountIsNeitherInvitedBackNorGivenStarterSkills(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", store.RoleOwner)
	// An account from an older preset: it was created before the starter catalog existed, so it
	// owns neither a skill nor a seed marker of its own.
	var legacy string
	if e := f.db.QueryRow(ctx, `
		INSERT INTO users (source, subject, display_name) VALUES ('email', 'alice@demo.test', 'Alice')
		RETURNING id::text`).Scan(&legacy); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(ctx, `
		INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, 'member')`,
		f.tenant, legacy); e != nil {
		t.Fatal(e)
	}
	if e := f.store.RetireUsers(ctx, []store.RetiredUser{{Email: "alice@demo.test"}}); e != nil {
		t.Fatal(e)
	}

	// The invitation path refuses the address instead of resurrecting it with a fresh catalog.
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "alice@demo.test"}); !errors.Is(e, store.ErrUserUnavailable) {
		t.Fatalf("inviting a retired address = %v", e)
	}

	// Listing it in the startup preset again refreshes the name and nothing else: the account stays
	// deleted, and it gets no starter catalog even though it never had one.
	if e := f.store.SeedUsers(ctx, store.Preset{Users: []store.PresetUser{
		{Email: "owner@demo.test", DisplayName: "owner", Role: store.RoleOwner},
		{Email: "alice@demo.test", DisplayName: "Alice Again", Role: store.RoleMember},
	}}); e != nil {
		t.Fatal(e)
	}
	var status string
	var deleted bool
	if e := f.db.QueryRow(ctx, `
		SELECT status, deleted_at IS NOT NULL FROM users WHERE id = $1`, legacy).Scan(&status, &deleted); e != nil {
		t.Fatal(e)
	}
	if status != "disabled" || !deleted {
		t.Fatalf("reseed resurrected the retired account: status=%q deleted=%v", status, deleted)
	}
	var skills, seeds int
	if e := f.db.QueryRow(ctx, `SELECT count(*) FROM v2_skills WHERE user_id = $1`, legacy).Scan(&skills); e != nil {
		t.Fatal(e)
	}
	if e := f.db.QueryRow(ctx, `SELECT count(*) FROM v2_skill_seeds WHERE user_id = $1`, legacy).Scan(&seeds); e != nil {
		t.Fatal(e)
	}
	if skills != 0 || seeds != 0 {
		t.Fatalf("retired account regained its starter catalog: skills=%d seeds=%d", skills, seeds)
	}
}

func TestRetireLegacyDemoUserKeepsItsHistory(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", store.RoleOwner)
	var legacy string
	if e := f.db.QueryRow(ctx, `
		INSERT INTO users (source, subject, display_name) VALUES ('dev', 'demo', '演示用户')
		RETURNING id::text`).Scan(&legacy); e != nil {
		t.Fatal(e)
	}
	if _, e := f.db.Exec(ctx, `
		INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)`,
		f.tenant, legacy, store.RoleMember); e != nil {
		t.Fatal(e)
	}

	if e := f.store.RetireLegacyDemoUser(ctx); e != nil {
		t.Fatal(e)
	}
	members, e := f.store.TenantMembers(ctx, f.tenant, owner)
	if e != nil {
		t.Fatal(e)
	}
	if len(members) != 1 || members[0].UserID != owner {
		t.Fatalf("roster after legacy retirement = %+v", members)
	}
	var disabled bool
	if e := f.db.QueryRow(ctx, `
		SELECT status = 'disabled' AND deleted_at IS NOT NULL
		FROM users WHERE id = $1`, legacy).Scan(&disabled); e != nil {
		t.Fatal(e)
	}
	if !disabled {
		t.Fatal("legacy demo user was not retired")
	}
	if e := f.store.RetireLegacyDemoUser(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestInviteMemberRequiresOwnerAndCreatesTheAccount(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", "owner")
	member := f.login(t, "member", "member")

	if _, e := f.store.InviteMember(ctx, f.tenant, member, store.Invite{Email: "new@demo.test"}); !errors.Is(e, store.ErrForbidden) {
		t.Fatalf("member invite = %v", e)
	}
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "new@demo.test"}); e != nil {
		t.Fatal(e)
	}
	// The display name defaults to the local part, and the invitee can now sign in.
	u, e := f.store.UserByEmail(ctx, "new@demo.test")
	if e != nil || u.DisplayName != "new" {
		t.Fatalf("invitee = %+v %v", u, e)
	}
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "NEW@demo.test"}); !errors.Is(e, store.ErrAlreadyMember) {
		t.Fatalf("duplicate invite = %v", e)
	}
	// An invalid address never creates an account.
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "not-an-address"}); e == nil {
		t.Fatal("invited an invalid address")
	}
}

func TestInviteMemberJoinsOneLiveSquad(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", "owner")
	squad, e := f.store.CreateSquad(ctx, f.tenant, owner, store.SquadInput{Name: "产品小队"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "new@demo.test", SquadID: squad.ID}); e != nil {
		t.Fatal(e)
	}
	detail, e := f.store.Squad(ctx, f.tenant, squad.ID)
	if e != nil {
		t.Fatal(e)
	}
	joined := false
	for _, m := range liveMembers(detail) {
		if m.Type == store.MemberHuman && m.DisplayName == "new" {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("invitee not in squad: %+v", detail.Members)
	}
	// The squad's version moves with the new member, so an open edit cannot overwrite it unseen.
	if detail.Version != squad.Version+1 {
		t.Fatalf("squad version after invite = %d, want %d", detail.Version, squad.Version+1)
	}

	// An archived or foreign squad refuses the invitation without creating the account.
	if _, e := f.store.ArchiveSquad(ctx, f.tenant, owner, squad.ID, detail.Version); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.InviteMember(ctx, f.tenant, owner, store.Invite{Email: "late@demo.test", SquadID: squad.ID}); !errors.Is(e, store.ErrSquadNotFound) {
		t.Fatalf("archived squad invite = %v", e)
	}
	if _, e := f.store.UserByEmail(ctx, "late@demo.test"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("failed invite left an account: %v", e)
	}
}

// tenantOf reads the demo tenant behind an account; v2 has exactly one.
func tenantOf(t *testing.T, f *squadFixture, user string) string {
	t.Helper()
	tenants, e := f.store.Tenants(f.ctx, user)
	if e != nil || len(tenants) != 1 {
		t.Fatalf("tenants = %v %v", tenants, e)
	}
	f.tenant = tenants[0].ID
	return tenants[0].ID
}
