package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestWorkflowPluginImportsAndKeepsWorkflowsOnRemoval(t *testing.T) {
	f := newSquadFixture(t)
	owner := f.login(t, "owner", "owner")
	member := f.login(t, "member", "member")
	spaces, e := f.store.TenantSpaces(f.ctx, f.tenant, owner)
	if e != nil || len(spaces) != 1 {
		t.Fatalf("spaces = %v %v", spaces, e)
	}
	space := spaces[0].ID
	path := "/api/v1/tenants/" + f.tenant + "/spaces/" + space + "/plugins"
	entry := pluginmarket.Entry{
		Namespace: "official", Identifier: "ora-space.requirement-development",
		Title: "Requirement Development", Kind: "workflow", Version: "0.1.0",
		Description: "A workflow", URL: "https://example.org/workflow.orax",
		SHA256: strings.Repeat("a", 64), MarketplaceVisible: true,
		SourceURL: "https://github.com/ora-space/marketplace",
	}
	if e := f.store.ReplacePluginCatalog(f.ctx, []pluginmarket.Entry{entry}, time.Now()); e != nil {
		t.Fatal(e)
	}
	source, e := f.store.PluginPackageSource(f.ctx, f.tenant, space, owner, "official/ora-space.requirement-development")
	if e != nil || source.Entry.Kind != "workflow" || source.Locator != entry.URL || source.Digest != entry.SHA256 {
		t.Fatalf("package source = %+v %v", source, e)
	}
	if _, e := f.store.PluginPackageSource(f.ctx, f.tenant, space, member, "official/ora-space.requirement-development"); !errors.Is(e, store.ErrForbidden) {
		t.Fatalf("member package source = %v", e)
	}

	documents := []pluginmarket.WorkflowDocument{{
		Path:  "assets/workflows/requirement-development.json",
		Name:  "需求开发",
		Graph: `{"nodes":[{"id":"start","data":{"kind":"start"}},{"id":"output","data":{"kind":"output"}}],"edges":[{"source":"start","target":"output"}]}`,
	}}
	validate := func(graph string) error {
		if !strings.Contains(graph, `"start"`) {
			return errors.New("missing start")
		}
		return nil
	}
	image := store.PluginInstallInput{
		Identifier: "official/ora-space.requirement-development", PluginVersion: "0.1.0",
	}
	installed, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, space, owner, path, "install-workflow", image, documents, validate)
	if e != nil {
		t.Fatal(e)
	}
	if installed.Kind != "workflow" || installed.ObservedState != "installed" ||
		installed.ObservedVersion == nil || *installed.ObservedVersion != "0.1.0" {
		t.Fatalf("installed = %+v", installed)
	}
	workflows, e := f.store.Workflows(f.ctx, f.tenant)
	if e != nil || len(workflows) != 1 || workflows[0].Name != "需求开发" ||
		workflows[0].PublishedVersion == nil || *workflows[0].PublishedVersion != "v1" {
		t.Fatalf("workflows = %+v %v", workflows, e)
	}
	version, e := f.store.WorkflowVersion(f.ctx, f.tenant, workflows[0].ID, "v1")
	if e != nil || version.Graph != documents[0].Graph {
		t.Fatalf("published graph = %+v %v", version, e)
	}

	replayed, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, space, owner, path, "install-workflow", image, documents, validate)
	if e != nil || replayed.Version != installed.Version {
		t.Fatalf("replay = %+v %v", replayed, e)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 1 {
		t.Fatalf("replay duplicated workflows: %+v", all)
	}
	if _, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, space, owner, path, "reapply-workflow", store.PluginInstallInput{
		Identifier: image.Identifier, PluginVersion: "0.1.0", Version: installed.Version,
	}, documents, validate); e != nil {
		t.Fatal(e)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 1 {
		t.Fatalf("reapply duplicated workflows: %+v", all)
	}

	removed, e := f.store.RemoveSpacePlugin(f.ctx, f.tenant, space, owner, path, "remove-workflow", store.PluginRemoveInput{
		Identifier: image.Identifier, Version: installed.Version + 1,
	})
	if e != nil {
		t.Fatal(e)
	}
	if removed.DesiredState != "removed" || removed.ObservedState != "removed" {
		t.Fatalf("removed = %+v", removed)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 1 {
		t.Fatalf("removal deleted workflows: %+v", all)
	}

	reinstalled, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, space, owner, path, "reinstall-workflow", store.PluginInstallInput{
		Identifier: image.Identifier, PluginVersion: "0.1.0", Version: removed.Version,
	}, documents, validate)
	if e != nil || reinstalled.DesiredState != "installed" || reinstalled.ObservedState != "installed" {
		t.Fatalf("reinstall = %+v %v", reinstalled, e)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 1 {
		t.Fatalf("reinstall duplicated workflows: %+v", all)
	}
}

func TestWorkflowPluginRejectsInvalidDocument(t *testing.T) {
	f := newSquadFixture(t)
	owner := f.login(t, "owner", "owner")
	spaces, e := f.store.TenantSpaces(f.ctx, f.tenant, owner)
	if e != nil || len(spaces) != 1 {
		t.Fatalf("spaces = %v %v", spaces, e)
	}
	space := spaces[0].ID
	path := "/api/v1/tenants/" + f.tenant + "/spaces/" + space + "/plugins"
	if e := f.store.ReplacePluginCatalog(f.ctx, []pluginmarket.Entry{
		{Namespace: "official", Identifier: "bad.workflow", Title: "Bad", Kind: "workflow", Version: "1.0.0", URL: "https://example.org/bad.orax", SHA256: strings.Repeat("b", 64), MarketplaceVisible: true},
	}, time.Now()); e != nil {
		t.Fatal(e)
	}
	bad := []pluginmarket.WorkflowDocument{{Path: "assets/workflows/bad.json", Name: "坏图", Graph: "not-json"}}
	_, e = f.store.InstallWorkflowPlugin(f.ctx, f.tenant, space, owner, path, "bad-install", store.PluginInstallInput{
		Identifier: "official/bad.workflow", PluginVersion: "1.0.0",
	}, bad, func(string) error { return errors.New("invalid graph") })
	if !errors.Is(e, store.ErrPluginWorkflowInvalid) {
		t.Fatalf("invalid workflow = %v", e)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 0 {
		t.Fatalf("invalid install imported a workflow: %+v", all)
	}
}

func TestWorkflowPluginAutoNamesConflictsAndKeepsSpacesIndependent(t *testing.T) {
	f := newSquadFixture(t)
	owner := f.login(t, "owner", "owner")
	spaces, e := f.store.TenantSpaces(f.ctx, f.tenant, owner)
	if e != nil || len(spaces) != 1 {
		t.Fatalf("spaces = %v %v", spaces, e)
	}
	firstSpace := spaces[0].ID
	var secondSpace string
	if e := f.db.QueryRow(f.ctx, `
		INSERT INTO v2_spaces(tenant_id, name, slug, created_by)
		VALUES($1, 'Other', 'other', $2) RETURNING id::text`,
		f.tenant, owner).Scan(&secondSpace); e != nil {
		t.Fatal(e)
	}
	if e := f.store.ReplacePluginCatalog(f.ctx, []pluginmarket.Entry{{
		Namespace: "official", Identifier: "ora-space.requirement-development",
		Title: "Requirement Development", Kind: "workflow", Version: "0.1.0",
		URL: "https://example.org/workflow.orax", SHA256: strings.Repeat("d", 64),
		MarketplaceVisible: true,
	}}, time.Now()); e != nil {
		t.Fatal(e)
	}
	original, _, e := f.store.CreateWorkflow(f.ctx, f.tenant, "需求开发", `{"nodes":[],"edges":[]}`)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.store.CreateWorkflow(f.ctx, f.tenant, "需求开发（插件）", `{"nodes":[],"edges":[]}`); e != nil {
		t.Fatal(e)
	}
	documents := []pluginmarket.WorkflowDocument{{
		Path: "assets/workflows/requirement-development.json", Name: "需求开发",
		Graph: `{"nodes":[{"id":"start","data":{"kind":"start"}},{"id":"output","data":{"kind":"output"}}],"edges":[{"source":"start","target":"output"}]}`,
	}}
	validate := func(string) error { return nil }
	image := store.PluginInstallInput{
		Identifier: "official/ora-space.requirement-development", PluginVersion: "0.1.0",
	}
	first, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, firstSpace, owner,
		"/api/v1/tenants/"+f.tenant+"/spaces/"+firstSpace+"/plugins",
		"first-space", image, documents, validate)
	if e != nil || first.ObservedState != "installed" {
		t.Fatalf("first space install = %+v %v", first, e)
	}
	all, e := f.store.Workflows(f.ctx, f.tenant)
	if e != nil || len(all) != 3 {
		t.Fatalf("workflows after first space = %+v %v", all, e)
	}
	names := map[string]bool{}
	for _, workflow := range all {
		names[workflow.Name] = true
	}
	if !names["需求开发"] || !names["需求开发（插件）"] || !names["需求开发（插件 2）"] {
		t.Fatalf("auto names = %v", names)
	}
	if got, e := f.store.Workflow(f.ctx, f.tenant, original.ID); e != nil || got.Name != "需求开发" {
		t.Fatalf("original workflow changed = %+v %v", got, e)
	}

	second, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, secondSpace, owner,
		"/api/v1/tenants/"+f.tenant+"/spaces/"+secondSpace+"/plugins",
		"second-space", image, documents, validate)
	if e != nil || second.ObservedState != "installed" {
		t.Fatalf("second space install = %+v %v", second, e)
	}
	all, e = f.store.Workflows(f.ctx, f.tenant)
	if e != nil || len(all) != 4 {
		t.Fatalf("workflows after second space = %+v %v", all, e)
	}
	foundThird := false
	for _, workflow := range all {
		if workflow.Name == "需求开发（插件 3）" {
			foundThird = true
		}
	}
	if !foundThird {
		t.Fatalf("second space did not receive an independent name: %+v", all)
	}
	var independentMappings int
	if e := f.db.QueryRow(f.ctx, `
		SELECT count(DISTINCT i.workflow_id)
		FROM v2_workflow_plugin_imports i
		WHERE i.selection_id IN ($1, $2)`,
		first.ID, second.ID).Scan(&independentMappings); e != nil {
		t.Fatal(e)
	}
	if independentMappings != 2 {
		t.Fatalf("spaces share import mappings: %d", independentMappings)
	}
	if _, e := f.store.InstallWorkflowPlugin(f.ctx, f.tenant, secondSpace, owner,
		"/api/v1/tenants/"+f.tenant+"/spaces/"+secondSpace+"/plugins",
		"second-space-reapply", store.PluginInstallInput{
			Identifier: image.Identifier, PluginVersion: "0.1.0", Version: second.Version,
		}, documents, validate); e != nil {
		t.Fatal(e)
	}
	if all, _ := f.store.Workflows(f.ctx, f.tenant); len(all) != 4 {
		t.Fatalf("reapply duplicated a second-space workflow: %+v", all)
	}
}
