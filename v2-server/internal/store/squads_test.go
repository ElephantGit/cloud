package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type squadFixture struct {
	ctx    context.Context
	store  *store.Store
	db     *pgx.Conn
	tenant string
}

// newSquadFixture opens an isolated schema and a maintenance connection that can seed users,
// memberships and Space Agents the squad API does not own.
func newSquadFixture(t *testing.T) *squadFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	var b [8]byte
	_, _ = rand.Read(b[:])
	schema := "squad_" + hex.EncodeToString(b[:])
	if _, e := db.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	if _, e := db.Exec(ctx, "SET search_path TO "+schema+",public"); e != nil {
		t.Fatal(e)
	}
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return &squadFixture{ctx: ctx, store: s, db: db}
}

// login returns the user id and makes sure the fixture tenant is known.
func (f *squadFixture) login(t *testing.T, subject, role string) string {
	t.Helper()
	token, e := f.store.Login(f.ctx, store.Identity{Source: "test", Subject: subject, DisplayName: subject}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	u, e := f.store.SessionUser(f.ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenants, e := f.store.Tenants(f.ctx, u.ID)
	if e != nil || len(tenants) != 1 {
		t.Fatalf("tenants = %v %v", tenants, e)
	}
	f.tenant = tenants[0].ID
	if _, e := f.db.Exec(f.ctx, `UPDATE memberships SET role = $1 WHERE tenant_id = $2 AND user_id = $3`, role, f.tenant, u.ID); e != nil {
		t.Fatal(e)
	}
	return u.ID
}

func (f *squadFixture) agent(t *testing.T, name, status string) string {
	t.Helper()
	var id string
	e := f.db.QueryRow(f.ctx, `INSERT INTO v2_space_agents (tenant_id, name, status) VALUES ($1, $2, $3) RETURNING id`, f.tenant, name, status).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	return id
}

func liveMembers(d store.SquadDetail) []store.SquadMember {
	var out []store.SquadMember
	for _, m := range d.Members {
		if m.EndedAt == nil {
			out = append(out, m)
		}
	}
	return out
}

func TestSquadLifecycle(t *testing.T) {
	f := newSquadFixture(t)
	ctx := f.ctx
	owner := f.login(t, "owner", "owner")
	member := f.login(t, "member", "member")
	agent := f.agent(t, "研究员", "active")

	created, e := f.store.CreateSquad(ctx, f.tenant, owner, store.SquadInput{Name: "产品小队", Description: "一起做产品", Charter: "上午评审"})
	if e != nil {
		t.Fatal(e)
	}
	if created.Version != 1 || created.HumanCount != 1 || created.MaintainerCount != 1 || created.AgentCount != 0 {
		t.Fatalf("created = %+v", created)
	}
	creator := liveMembers(created)
	if len(creator) != 1 || creator[0].Type != store.MemberHuman || creator[0].UserID == nil || *creator[0].UserID != owner || !creator[0].Maintainer {
		t.Fatalf("creator membership = %+v", created.Members)
	}

	// A plain workspace member can read the squad but not edit it.
	if out, e := f.store.Squad(ctx, f.tenant, created.ID); e != nil || out.Name != "产品小队" {
		t.Fatalf("read as member = %+v %v", out, e)
	}
	if _, e := f.store.UpdateSquad(ctx, f.tenant, member, created.ID, store.SquadInput{Name: "改名", Version: 1}); !errors.Is(e, store.ErrForbidden) {
		t.Fatalf("non-maintainer edit = %v", e)
	}

	// The maintainer adds the other human and the agent; counts split by type.
	added, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberHuman, UserID: member, Role: "评审", Version: 1})
	if e != nil {
		t.Fatal(e)
	}
	if added.Version != 2 || added.HumanCount != 2 || added.AgentCount != 0 {
		t.Fatalf("after human add = %+v", added)
	}
	if _, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberHuman, UserID: member, Version: 2}); !errors.Is(e, store.ErrMemberExists) {
		t.Fatalf("duplicate add = %v", e)
	}
	withAgent, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberAgent, SpaceAgentID: agent, Role: "调研", Version: 2})
	if e != nil {
		t.Fatal(e)
	}
	if withAgent.AgentCount != 1 || withAgent.HumanCount != 2 {
		t.Fatalf("after agent add = %+v", withAgent)
	}

	// Agents cannot be maintainers, and retired or not-installed agents cannot join.
	if _, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberAgent, SpaceAgentID: f.agent(t, "退役", "retired"), Version: 3}); !errors.Is(e, store.ErrAgentUnavailable) {
		t.Fatalf("retired agent = %v", e)
	}
	var failed string
	if e := f.db.QueryRow(ctx, `INSERT INTO v2_space_agents (tenant_id, name, observed_state) VALUES ($1, '安装失败', 'failed') RETURNING id`, f.tenant).Scan(&failed); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberAgent, SpaceAgentID: failed, Version: 3}); !errors.Is(e, store.ErrAgentUnavailable) {
		t.Fatalf("agent not installed = %v", e)
	}

	detail, e := f.store.Squad(ctx, f.tenant, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	var memberID string
	for _, m := range detail.Members {
		if m.UserID != nil && *m.UserID == member && m.EndedAt == nil {
			memberID = m.ID
		}
	}
	if memberID == "" || detail.MaintainerCount != 1 || detail.HumanCount != 2 || detail.AgentCount != 1 {
		t.Fatalf("added detail = %+v", detail)
	}
	if _, e := f.store.UpdateSquadMember(ctx, f.tenant, owner, created.ID, memberID, store.SquadMemberUpdate{Role: "评审", Maintainer: true, Version: detail.Version}); e != nil {
		t.Fatalf("promote maintainer = %v", e)
	}

	// The stale version fails before any write; the current one is required.
	if _, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberAgent, SpaceAgentID: agent, Version: 2}); !errors.Is(e, store.ErrConflict) {
		t.Fatalf("stale version = %v", e)
	}

	detail, e = f.store.Squad(ctx, f.tenant, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if detail.MaintainerCount != 2 {
		t.Fatalf("promoted detail = %+v", detail)
	}

	// Removing ends the grant; rejoining is a new grant and keeps the old row as history.
	removed, e := f.store.RemoveSquadMember(ctx, f.tenant, member, created.ID, memberID, detail.Version)
	if e != nil {
		t.Fatal(e)
	}
	if removed.HumanCount != 1 || removed.MaintainerCount != 1 {
		t.Fatalf("after remove = %+v", removed)
	}
	rejoined, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberHuman, UserID: member, Role: "重新加入", Version: removed.Version})
	if e != nil {
		t.Fatal(e)
	}
	ended := 0
	for _, m := range rejoined.Members {
		if m.UserID != nil && *m.UserID == member && m.EndedAt != nil {
			ended++
		}
	}
	if ended != 1 || rejoined.HumanCount != 2 {
		t.Fatalf("rejoin history = %+v", rejoined)
	}

	// The leader must be an active agent member; setting one adds the membership in the same write.
	leader, e := f.store.UpdateSquad(ctx, f.tenant, owner, created.ID, store.SquadInput{Name: "产品小队", Description: "一起做产品", Charter: "上午评审", LeaderAgentID: &agent, Version: rejoined.Version})
	if e != nil {
		t.Fatal(e)
	}
	if leader.LeaderAgentID == nil || *leader.LeaderAgentID != agent || leader.AgentCount != 1 {
		t.Fatalf("leader = %+v", leader)
	}
	leaderMember := ""
	for _, m := range leader.Members {
		if m.SpaceAgentID != nil && *m.SpaceAgentID == agent && m.EndedAt == nil {
			leaderMember = m.ID
		}
	}
	if leaderMember == "" {
		t.Fatalf("leader membership missing: %+v", leader.Members)
	}

	// Removing the leader clears the leader in the same transaction.
	noLeader, e := f.store.RemoveSquadMember(ctx, f.tenant, owner, created.ID, leaderMember, leader.Version)
	if e != nil {
		t.Fatal(e)
	}
	if noLeader.LeaderAgentID != nil || noLeader.AgentCount != 0 {
		t.Fatalf("remove leader = %+v", noLeader)
	}

	// Archiving ends every live membership, keeps the history and blocks further edits.
	archived, e := f.store.ArchiveSquad(ctx, f.tenant, owner, created.ID, noLeader.Version)
	if e != nil {
		t.Fatal(e)
	}
	if archived.ArchivedAt == nil || archived.HumanCount != 0 || archived.AgentCount != 0 || len(liveMembers(archived)) != 0 {
		t.Fatalf("archived = %+v", archived)
	}
	if len(archived.Members) < 3 {
		t.Fatalf("archive lost history: %+v", archived.Members)
	}
	if _, e := f.store.AddSquadMember(ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberHuman, UserID: member, Version: archived.Version}); !errors.Is(e, store.ErrArchived) {
		t.Fatalf("add after archive = %v", e)
	}
	if _, e := f.store.ArchiveSquad(ctx, f.tenant, owner, created.ID, archived.Version); !errors.Is(e, store.ErrArchived) {
		t.Fatalf("second archive = %v", e)
	}
	list, e := f.store.Squads(ctx, f.tenant)
	if e != nil || len(list) != 0 {
		t.Fatalf("archived listed = %+v %v", list, e)
	}
	if _, e := f.store.Squad(ctx, "00000000-0000-0000-0000-000000000001", created.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("cross-tenant read = %v", e)
	}
}

func TestSquadConcurrentMemberAdd(t *testing.T) {
	f := newSquadFixture(t)
	owner := f.login(t, "owner", "owner")
	first := f.login(t, "first", "member")
	second := f.login(t, "second", "member")
	created, e := f.store.CreateSquad(f.ctx, f.tenant, owner, store.SquadInput{Name: "并发小队"})
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, user := range []string{first, second} {
		go func(user string) {
			<-start
			_, e := f.store.AddSquadMember(f.ctx, f.tenant, owner, created.ID, store.SquadMemberInput{Type: store.MemberHuman, UserID: user, Version: 1})
			results <- e
		}(user)
	}
	close(start)
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		switch e := <-results; {
		case e == nil:
			wins++
		case errors.Is(e, store.ErrConflict):
			conflicts++
		default:
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins %d conflicts %d", wins, conflicts)
	}
	detail, e := f.store.Squad(f.ctx, f.tenant, created.ID)
	if e != nil || detail.Version != 2 || detail.HumanCount != 2 {
		t.Fatalf("after race = %+v %v", detail, e)
	}
}
