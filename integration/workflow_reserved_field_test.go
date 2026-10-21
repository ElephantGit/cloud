package integration

import (
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
	"github.com/wanglongan587/cloud/internal/workflowasset"
)

// reservedVariables is the author declaration the publish gate refuses: a Start variable under the
// name the platform owns in the `@` form.
func reservedVariables() []any {
	return []any{
		core.Object{"name": "snapshotId", "displayName": "Release version", "fieldType": "text-input", "valueType": "string"},
	}
}

// TestWorkflowReservedFieldIsRefusedOnPublishAndImport covers the two freezing entries
// (20261021-snapshot-id-is-a-reserved-field.md D1): publishing and workflow asset import share the
// graph gate, so the same declaration is refused with the same code and the same location, and
// neither leaves a snapshot behind. Saving the draft, and a `launchFields` withdrawal that mentions
// the name without declaring it, are not refusals.
func TestWorkflowReservedFieldIsRefusedOnPublishAndImport(t *testing.T) {
	f := setup(t)

	wid := createWorkflow(t, f, "Reserved Start", "reserved-start", reservedVariables())
	refused := f.call("POST", f.path("/workflows/"+wid+"/publish"), core.Object{}, "reserved-start-pub", 400)
	if got := problems(t, refused); len(got) != 1 || got[0].S("code") != "reserved_field_name" || got[0].S("nodeId") != "start-1" || got[0].S("field") != "snapshotId" {
		t.Fatalf("the Start declaration was not refused by name: %v", got)
	}

	globalGraph := startInputGraph(nil)
	globalGraph["globalVariables"] = []any{core.Object{"name": "snapshotId", "valueType": "string", "value": "v1"}}
	global := f.call("POST", f.path("/workflows"), core.Object{
		"name": "Reserved Global", "graph": globalGraph,
	}, "reserved-global-save", 200).O("resource")
	globalRefused := f.call("POST", f.path("/workflows/"+global.S("id")+"/publish"), core.Object{}, "reserved-global-pub", 400)
	if got := problems(t, globalRefused); len(got) != 1 || got[0].S("code") != "reserved_field_name" || got[0].S("name") != "snapshotId" {
		t.Fatalf("the global declaration was not refused by name: %v", got)
	}

	// Withdrawing the platform's selector is a request about the platform's own field, not a claim on
	// the name, so it publishes like any graph that declares nothing.
	withdrawn := createLaunchWorkflow(t, f, "Withdrawn", "reserved-withdrawn", nil, []any{
		core.Object{"key": "snapshotId", "enabled": false},
	})
	f.call("POST", f.path("/workflows/"+withdrawn+"/publish"), core.Object{}, "reserved-withdrawn-pub", 200)

	// The import entry freezes documents through the same gate. A package with one good document and
	// the two declarations above reports each failed path on its own and still imports the good one.
	repo := workflowMarketplace(t, f.root)
	document := func(name, node, extra string) string {
		if extra != "" {
			extra = "," + extra
		}
		return `{"name":"` + name + `",` + node + extra + `}`
	}
	pkg := workflowPackage(t, map[string]string{
		"start.json": document("Reserved Start Kit",
			`"nodes":[{"id":"start","data":{"kind":"start","inputVariables":[{"name":"snapshotId","valueType":"string"}]}}],"edges":[]`, ""),
		"global.json": document("Reserved Global Kit",
			`"nodes":[{"id":"start","data":{"kind":"start"}}],"edges":[]`,
			`"globalVariables":[{"name":"snapshotId","valueType":"string","value":"v1"}]`),
		"fine.json": executableDocument("Fine Kit", ""),
	})
	packages := newWorkflowPackages(t, map[string][]byte{"/packages/reserved-1.0.0.orax": pkg})
	publishWorkflowRelease(t, repo, "reserved-kit", "1.0.0", packages.server.URL+"/packages/reserved-1.0.0.orax", pkg)
	f.syncMarketplace(t, repo)
	f.store.WorkflowAssets = workflowasset.NewFetcher(nil, packages.server.Client().Transport)
	sid := f.defaultSpaceID()
	f.installWorkflowPlugin(t, sid, "official/reserved-kit", "")
	must(t, f.importWorkflowAssets(t))

	documents := assetDocuments(f.assetImport(t, sid, "official/reserved-kit"))
	if got := documents["assets/workflows/fine.json"].S("outcome"); got != "imported" {
		t.Fatalf("a legal document was not imported: %v", documents["assets/workflows/fine.json"])
	}
	for path, want := range map[string]core.Object{
		"assets/workflows/start.json":  {"code": "reserved_field_name", "nodeId": "start", "field": "snapshotId"},
		"assets/workflows/global.json": {"code": "reserved_field_name", "name": "snapshotId"},
	} {
		imported := documents[path]
		if imported.S("outcome") != "failed" || imported.S("failureCode") != "document_not_executable_graph" {
			t.Fatalf("%s = %v, want document_not_executable_graph", path, imported)
		}
		raw, _ := imported.O("failureDetail")["problems"].([]any)
		if len(raw) != 1 {
			t.Fatalf("%s problems = %v, want the one reserved declaration", path, raw)
		}
		problem, _ := raw[0].(map[string]any)
		if got := core.Object(problem); got.S("code") != want.S("code") || got.S("nodeId") != want.S("nodeId") || got.S("name") != want.S("name") || got.S("field") != want.S("field") {
			t.Fatalf("%s problem = %v, want %v", path, got, want)
		}
	}

	// Neither entry froze a refused declaration, and the two successful paths each froze exactly one.
	if n := f.scalar("SELECT count(*) FROM workflow_snapshots WHERE workflow_id=$1", wid); n != 0 {
		t.Fatalf("a refused publish left %d snapshots", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_snapshots WHERE workflow_id=$1", global.S("id")); n != 0 {
		t.Fatalf("a refused global left %d snapshots", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_snapshots WHERE workflow_id IN (SELECT id FROM workflows WHERE name IN ('Reserved Start Kit','Reserved Global Kit'))"); n != 0 {
		t.Fatalf("refused imported documents left %d snapshots", n)
	}
	if n := f.scalar("SELECT count(*) FROM workflow_snapshots"); n != 2 {
		t.Fatalf("snapshots = %d, want the withdrawn-selector version and the imported one", n)
	}
}

// TestWorkflowReservedFieldWithholdsStockVersionsAndRefusesThem covers the read side of the reserved
// field (20261021-snapshot-id-is-a-reserved-field.md D2/D3): a version published before the gate that
// declares `snapshotId` is never offered by the selector, the selector explains why, and a confirm
// that resolves to it — by cursor or by falling back to the newest version — is refused instead of
// executing something the user did not choose or silently substituting another version.
func TestWorkflowReservedFieldWithholdsStockVersionsAndRefusesThem(t *testing.T) {
	f := setup(t)
	// The live draft leaves `snapshotId` to the platform, which is what makes a submitted value under
	// that key a version cursor rather than an author variable.
	wid := createWorkflow(t, f, "Stock Versions", "reserved-stock", notesVariables())
	selectable := publishWorkflow(t, f, wid, "reserved-stock-pub-1")
	// The stock version is newer, so it is what "no cursor" resolves to.
	stock := seedLegacySnapshot(t, f, wid, 2, startInputGraph(append(reservedVariables(), notesVariables()...)))

	project := f.create("reserved-stock-project").O("resource")
	iss := f.call("POST", f.path("/issues"), core.Object{
		"title": "Run a stock version", "projectRef": project.S("id"),
	}, "reserved-stock-issue", 200).O("resource")

	fields := formFields(t, f.call("GET", f.path("/collaboration/forms/"+wid+"?issueId="+iss.S("id")), nil, "", 200))
	version := fields["snapshotId"]
	if version.S("type") != "select" || version["defaultValue"] != selectable.S("id") {
		t.Fatalf("the selector did not open on the newest selectable version: %v", version)
	}
	options, _ := version["options"].([]any)
	if len(options) != 1 || core.Object(options[0].(map[string]any)).S("value") != selectable.S("id") {
		t.Fatalf("the withheld version was offered: %v", options)
	}
	if version.S("description") == "" {
		t.Fatalf("the selector did not explain the withheld version: %v", version)
	}

	// The snapshot's own form is not the selector's: it projects the author's declaration, as it did
	// before the gate. The selector above is the platform's offer, and it no longer includes it.
	own := formFields(t, f.call("GET", f.path("/workflows/"+wid+"/snapshots/"+stock.S("id")+"/form"), nil, "", 200))
	if got := own["snapshotId"]; got.S("type") != "text" || got.S("label") != "Release version" {
		t.Fatalf("the stock version's author declaration was not projected: %v", got)
	}

	it := workflowComment(t, f, wid, iss.S("id"), "reserved-stock-1", "run it")
	base := core.Object{"repository": project.S("repositoryUrl"), "branch": "main"}
	refusal := func(key string, values core.Object) core.Object {
		t.Helper()
		refused := f.call("POST", confirmPath(f, iss.S("id"), it.S("id")), core.Object{"values": values}, key, 409)
		if refused.S("code") != "workflow_version_reserved_field" || refused.O("params").S("snapshotId") != stock.S("id") || refused.O("params").S("field") != "snapshotId" {
			t.Fatalf("refusal = %v, want workflow_version_reserved_field for %s", refused, stock.S("id"))
		}
		return refused
	}
	// No cursor: the newest published version is the stock one, and it is refused rather than executed.
	refusal("reserved-stock-c1", core.Object{"repository": base.S("repository"), "branch": "main"})
	// An explicit cursor to it is the same refusal, not a silent fallback to another version.
	refusal("reserved-stock-c2", core.Object{"repository": base.S("repository"), "branch": "main", "snapshotId": stock.S("id")})
	if n := f.scalar("SELECT count(*) FROM issue_runs WHERE issue_id=$1", iss.S("id")); n != 0 {
		t.Fatalf("a refused confirm created %d runs", n)
	}
	if n := f.scalar("SELECT count(*) FROM issue_interactions WHERE id=$1 AND run_id IS NOT NULL", it.S("id")); n != 0 {
		t.Fatal("a refused confirm claimed the interaction")
	}

	// The selectable version is the one the form offers, and choosing it runs exactly it.
	created := f.call("POST", confirmPath(f, iss.S("id"), it.S("id")), core.Object{"values": core.Object{
		"repository": base.S("repository"), "branch": "main", "snapshotId": selectable.S("id"),
	}}, "reserved-stock-c3", 200).O("resource")
	run := f.call("GET", f.path("/issues/"+iss.S("id")+"/runs/"+created.S("id")), nil, "", 200)
	if got := run.O("input").S("workflowSnapshotId"); got != selectable.S("id") {
		t.Fatalf("run snapshot = %q, want the chosen selectable version %q", got, selectable.S("id"))
	}
}

// TestWorkflowReservedFieldBelongsToTheAuthorWhenTheDraftDeclaresIt pins the other half of D3: the
// Confirm refusal protects a key the *platform* owns, which is why it applies only while the live draft
// leaves `snapshotId` alone. A draft that declares the name itself owns it; a submitted value under it
// is the author's variable, not a version cursor, so a frozen version that also declares it is
// executed as chosen rather than refused.
func TestWorkflowReservedFieldBelongsToTheAuthorWhenTheDraftDeclaresIt(t *testing.T) {
	f := setup(t)
	// The live draft is the legacy shape the publish gate no longer creates, saved directly.
	wid := createWorkflow(t, f, "Author Owns It", "reserved-owned", reservedVariables())
	stock := seedLegacySnapshot(t, f, wid, 1, startInputGraph(reservedVariables()))

	project := f.create("reserved-owned-project").O("resource")
	iss := f.call("POST", f.path("/issues"), core.Object{
		"title": "Own the key", "projectRef": project.S("id"),
	}, "reserved-owned-issue", 200).O("resource")
	it := workflowComment(t, f, wid, iss.S("id"), "reserved-owned-1", "run it")
	created := f.call("POST", confirmPath(f, iss.S("id"), it.S("id")), core.Object{"values": core.Object{
		"repository": project.S("repositoryUrl"), "branch": "main", "snapshotId": "release-7",
	}}, "reserved-owned-c1", 200).O("resource")

	run := f.call("GET", f.path("/issues/"+iss.S("id")+"/runs/"+created.S("id")), nil, "", 200)
	input := run.O("input")
	if got := input.S("workflowSnapshotId"); got != stock.S("id") {
		t.Fatalf("run snapshot = %q, want the newest published version %q", got, stock.S("id"))
	}
	if got := input.O("interactionValues").S("snapshotId"); got != "release-7" {
		t.Fatalf("the author's value did not reach the workflow: %v", input.O("interactionValues"))
	}
}
