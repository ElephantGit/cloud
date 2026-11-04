package store_test

import (
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestWorkflowSkillRequiresMatchingProjectReceiptAndUser(t *testing.T) {
	f := newSquadFixture(t)
	owner := f.login(t, "owner", "owner")
	member := f.login(t, "member", "member")
	scope := execution.Scope{TenantID: f.tenant, UserID: owner}
	spaces, err := f.store.TenantSpaces(f.ctx, f.tenant, owner)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces[0].ID
	var other string
	if err := f.db.QueryRow(f.ctx, `INSERT INTO v2_spaces(tenant_id,name,slug,created_by) VALUES($1,'other','other',$2) RETURNING id`, f.tenant, owner).Scan(&other); err != nil {
		t.Fatal(err)
	}
	for id, spaceID := range map[string]string{"selected": space, "outside": other} {
		if _, err := f.db.Exec(f.ctx, `INSERT INTO v2_projects(id,tenant_id,data,plugin_space_id) VALUES($1,$2,'{}',$3)`, id, f.tenant, spaceID); err != nil {
			t.Fatal(err)
		}
	}
	entry := pluginmarket.Entry{Namespace: "official", Identifier: "review", Kind: "skill", Version: "1.0.0", MarketplaceVisible: true, URL: "https://example.org/skill", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := f.store.ReplacePluginCatalog(f.ctx, []pluginmarket.Entry{entry}, time.Now()); err != nil {
		t.Fatal(err)
	}
	selection, err := f.store.InstallSpacePlugin(f.ctx, f.tenant, space, owner, "plugins", "install", store.PluginInstallInput{Identifier: "official/review"})
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(project string) (*execution.Skills, error) {
		return f.store.ResolveWorkflowSkills(f.ctx, scope, project, []string{"official/review"})
	}
	if _, err := resolve("selected"); err == nil {
		t.Fatal("pending package accepted")
	}
	jobs, err := f.store.SkillJobs(f.ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if ok, err := f.store.StartSkillJob(f.ctx, jobs[0]); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := f.store.FinishSkillJob(f.ctx, jobs[0], store.SkillJobSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	got, err := resolve("selected")
	if err != nil || len(got.Plugins) != 1 || got.Plugins[0] != (execution.SkillPlugin{ID: "official/review", Version: "1.0.0"}) {
		t.Fatal(got, err)
	}
	if _, err := resolve("outside"); err == nil {
		t.Fatal("package from another space accepted")
	}
	if _, err := f.db.Exec(f.ctx, `UPDATE v2_space_agents SET version=version+1 WHERE id=$1`, selection.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve("selected"); err == nil {
		t.Fatal("stale project receipt accepted")
	}

	if _, err := f.store.CreateSkill(f.ctx, f.tenant, owner, store.SkillInput{Name: "private-review", Description: "Review", Content: "private"}); err != nil {
		t.Fatal(err)
	}
	got, err = f.store.ResolveWorkflowSkills(f.ctx, scope, "selected", []string{"private-review", "private-review"})
	if err != nil || len(got.Documents) != 1 || got.Documents[0].Content != "private" {
		t.Fatal(got, err)
	}
	if _, err := f.store.ResolveWorkflowSkills(f.ctx, execution.Scope{TenantID: f.tenant, UserID: member}, "selected", []string{"private-review"}); err == nil {
		t.Fatal("another user's skill accepted")
	}
}
