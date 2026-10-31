package store_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

const publishedGraph = `{"nodes":[{"id":"s","type":"workflow","data":{"kind":"start"}},{"id":"a","type":"workflow","data":{"kind":"agent","agentConfig":{"schemaVersion":3,"prompt":"review"}}}],"edges":[{"id":"s-a","source":"s","target":"a"}],"editor":{"revision":9007199254740993},"globalVariables":[{"name":"env.file","valueType":"file","value":"docs/input.md"}]}`

func workflowPointer[T any](v T) *T { return &v }

func createWorkflow(t *testing.T, f *executionFixture, name string) workflow.Document {
	t.Helper()
	d, err := f.s.CreateWorkflow(t.Context(), f.scope, name, workflow.CreateDocument{Name: name, Graph: json.RawMessage(publishedGraph)})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWorkflowDraftPublishRestoreAndArchive(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	d, err := f.s.CreateWorkflow(ctx, f.scope, "create", workflow.CreateDocument{Name: " Draft ", Graph: json.RawMessage(`{"editor":{"unfinished":true}}`)})
	if err != nil || d.Name != "Draft" || d.Version != 1 || d.DeletedAt != nil {
		t.Fatalf("draft=%+v err=%v", d, err)
	}
	var invalid *workflow.GraphError
	if _, err := f.s.PublishWorkflow(ctx, f.scope, d.ID, "publish", workflow.PublishDocument{}); !errors.As(err, &invalid) {
		t.Fatalf("unfinished draft published: %v", err)
	}
	if _, err := f.s.UpdateWorkflow(ctx, f.scope, d.ID, workflow.UpdateDocument{Name: d.Name}); !errors.Is(err, workflow.ErrVersionRequired) {
		t.Fatalf("missing version: %v", err)
	}
	d, err = f.s.UpdateWorkflow(ctx, f.scope, d.ID, workflow.UpdateDocument{Name: d.Name, Version: &d.Version, Graph: json.RawMessage(publishedGraph)})
	if err != nil || d.Version != 2 {
		t.Fatalf("save: %+v %v", d, err)
	}
	// Failed publication rolled back its key; now the same request can succeed.
	snapshot, err := f.s.PublishWorkflow(ctx, f.scope, d.ID, "publish", workflow.PublishDocument{})
	if err != nil || snapshot.Version != 1 || !bytes.Contains(snapshot.Graph, []byte(`workspace_file`)) || !bytes.Contains(snapshot.Graph, []byte(`9007199254740993`)) {
		t.Fatalf("publish=%+v err=%v", snapshot, err)
	}
	if current, err := f.s.GetWorkflow(ctx, f.scope, d.ID); err != nil || current.Version != 2 || bytes.Contains(current.Graph, []byte(`workspace_file`)) {
		t.Fatalf("publish changed draft: %+v %v", current, err)
	}
	d, err = f.s.UpdateWorkflow(ctx, f.scope, d.ID, workflow.UpdateDocument{Name: "Renamed", Version: &d.Version, Graph: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.RestoreWorkflowSnapshot(ctx, f.scope, d.ID, snapshot.ID, workflow.DocumentVersion{Version: workflowPointer(int64(2))}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale restore: %v", err)
	}
	d, err = f.s.RestoreWorkflowSnapshot(ctx, f.scope, d.ID, snapshot.ID, workflow.DocumentVersion{Version: &d.Version})
	if err != nil || d.Version != 4 || d.Name != "Renamed" || !bytes.Equal(d.Graph, snapshot.Graph) {
		t.Fatalf("restore=%+v err=%v", d, err)
	}
	if _, err := f.s.CreateWorkflow(ctx, f.scope, "duplicate", workflow.CreateDocument{Name: d.Name}); !errors.Is(err, store.ErrWorkflowNameConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	version := d.Version
	d, err = f.s.DeleteWorkflow(ctx, f.scope, d.ID, "delete", workflow.DocumentVersion{Version: &version})
	if err != nil || d.Version != 5 || d.DeletedAt == nil {
		t.Fatalf("archive=%+v err=%v", d, err)
	}
	if _, err := f.s.GetWorkflowSnapshot(ctx, f.scope, d.ID, snapshot.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("archived history visible: %v", err)
	}
	if _, err := f.s.CreateWorkflow(ctx, f.scope, "duplicate", workflow.CreateDocument{Name: d.Name}); err != nil {
		t.Fatalf("archive did not free name: %v", err)
	}
	// Reopen both the pool and schema; receipt replay precedes deleted/version checks.
	f.s.Close()
	f.s, err = store.OpenConfig(ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.DeleteWorkflow(ctx, f.scope, d.ID, "delete", workflow.DocumentVersion{Version: &version})
	if err != nil || replay.ID != d.ID || replay.Version != d.Version || replay.DeletedAt == nil || !replay.DeletedAt.Equal(*d.DeletedAt) {
		t.Fatalf("reopened delete replay=%+v err=%v", replay, err)
	}
	if _, err := f.s.DeleteWorkflow(ctx, f.scope, d.ID, "delete", workflow.DocumentVersion{Version: &d.Version}); !errors.Is(err, store.ErrKeyReused) {
		t.Fatalf("changed replay: %v", err)
	}
}

func TestWorkflowConcurrentAdmissionPublicationAndVersionConflict(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	const count = 8
	docs := make([]workflow.Document, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			docs[i], errs[i] = f.s.CreateWorkflow(ctx, f.scope, "same", workflow.CreateDocument{Name: "Concurrent", Graph: json.RawMessage(publishedGraph)})
		})
	}
	wg.Wait()
	for i := range count {
		if errs[i] != nil || docs[i].ID != docs[0].ID {
			t.Fatalf("duplicate admission %d: %+v %v", i, docs[i], errs[i])
		}
	}
	id := docs[0].ID
	snaps := make([]workflow.Snapshot, count)
	for i := range count {
		wg.Go(func() {
			snaps[i], errs[i] = f.s.PublishWorkflow(ctx, f.scope, id, "same-publish", workflow.PublishDocument{})
		})
	}
	wg.Wait()
	for i := range count {
		if errs[i] != nil || snaps[i].ID != snaps[0].ID || snaps[i].Version != 1 {
			t.Fatalf("duplicate publish %d: %+v %v", i, snaps[i], errs[i])
		}
	}
	for i := range count {
		wg.Go(func() {
			snaps[i], errs[i] = f.s.PublishWorkflow(ctx, f.scope, id, fmt.Sprintf("publish-%d", i), workflow.PublishDocument{})
		})
	}
	wg.Wait()
	versions := map[int64]bool{}
	for i := range count {
		if errs[i] != nil || versions[snaps[i].Version] || snaps[i].Version < 2 || snaps[i].Version > count+1 {
			t.Fatalf("publication version %d: %+v %v", i, snaps[i], errs[i])
		}
		versions[snaps[i].Version] = true
	}
	for i := range count {
		wg.Go(func() {
			_, errs[i] = f.s.UpdateWorkflow(ctx, f.scope, id, workflow.UpdateDocument{Name: "Concurrent", Version: workflowPointer(int64(1)), Description: workflowPointer(fmt.Sprintf("edit-%d", i))})
		})
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent writes accepted %d stale versions", wins)
	}
}

func TestWorkflowIdempotencyIncludesActionPathBodyAndOwner(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	input := workflow.CreateDocument{Name: "Ordered", Graph: json.RawMessage(`{"a":1,"b":{"x":2,"y":3}}`)}
	d, err := f.s.CreateWorkflow(ctx, f.scope, "key", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Graph = json.RawMessage(`{"b":{"y":3,"x":2},"a":1}`)
	if replay, err := f.s.CreateWorkflow(ctx, f.scope, "key", input); err != nil || replay.ID != d.ID {
		t.Fatalf("canonical replay: %+v %v", replay, err)
	}
	input.Graph = json.RawMessage(`{"a":2}`)
	if _, err := f.s.CreateWorkflow(ctx, f.scope, "key", input); !errors.Is(err, store.ErrKeyReused) {
		t.Fatalf("changed body reused key: %v", err)
	}
	if _, err := f.s.PublishWorkflow(ctx, f.scope, d.ID, "key", workflow.PublishDocument{}); !errors.Is(err, store.ErrKeyReused) {
		t.Fatalf("changed path reused key: %v", err)
	}
	other := executionMember(t, f.s, "editor")
	if _, err := f.s.CreateWorkflow(ctx, other, "key", workflow.CreateDocument{Name: "Other owner key"}); err != nil {
		t.Fatalf("key leaked between users: %v", err)
	}
}

func TestWorkflowTenantSnapshotAndRevocationBoundaries(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	d := createWorkflow(t, f, "Visible to members")
	otherDoc := createWorkflow(t, f, "Other workflow")
	snapshot, err := f.s.PublishWorkflow(ctx, f.scope, d.ID, "publish", workflow.PublishDocument{})
	if err != nil {
		t.Fatal(err)
	}
	member := executionMember(t, f.s, "member")
	if got, err := f.s.GetWorkflow(ctx, member, d.ID); err != nil || got.ID != d.ID {
		t.Fatalf("tenant document not shared: %+v %v", got, err)
	}
	if _, err := f.s.GetWorkflowSnapshot(ctx, f.scope, otherDoc.ID, snapshot.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign workflow snapshot read: %v", err)
	}
	if _, err := f.s.RestoreWorkflowSnapshot(ctx, f.scope, otherDoc.ID, snapshot.ID, workflow.DocumentVersion{Version: &otherDoc.Version}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign workflow snapshot restored: %v", err)
	}
	var tenant string
	if err := f.admin.QueryRow(ctx, `INSERT INTO tenants(name) VALUES('other tenant') RETURNING id`).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `INSERT INTO memberships(tenant_id,user_id,role) VALUES($1,$2,'owner')`, tenant, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	foreign := execution.Scope{TenantID: tenant, UserID: f.scope.UserID}
	if _, err := f.s.GetWorkflow(ctx, foreign, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross tenant read: %v", err)
	}
	if _, err := f.s.PublishWorkflow(ctx, foreign, d.ID, "cross", workflow.PublishDocument{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross tenant publish: %v", err)
	}
	if _, err := f.admin.Exec(ctx, `DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2`, f.scope.TenantID, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"get":      func() error { _, e := f.s.GetWorkflow(ctx, f.scope, d.ID); return e },
		"list":     func() error { _, e := f.s.ListWorkflows(ctx, f.scope, "", 50); return e },
		"snapshot": func() error { _, e := f.s.GetWorkflowSnapshot(ctx, f.scope, d.ID, snapshot.ID); return e },
		"history":  func() error { _, e := f.s.ListWorkflowSnapshots(ctx, f.scope, d.ID, "", 50); return e },
		"update": func() error {
			_, e := f.s.UpdateWorkflow(ctx, f.scope, d.ID, workflow.UpdateDocument{Name: "Changed", Version: &d.Version})
			return e
		},
		"restore": func() error {
			_, e := f.s.RestoreWorkflowSnapshot(ctx, f.scope, d.ID, snapshot.ID, workflow.DocumentVersion{Version: &d.Version})
			return e
		},
		"delete": func() error {
			_, e := f.s.DeleteWorkflow(ctx, f.scope, d.ID, "delete", workflow.DocumentVersion{Version: &d.Version})
			return e
		},
		"create replay": func() error {
			_, e := f.s.CreateWorkflow(ctx, f.scope, d.Name, workflow.CreateDocument{Name: d.Name, Graph: json.RawMessage(publishedGraph)})
			return e
		},
		"publish replay": func() error {
			_, e := f.s.PublishWorkflow(ctx, f.scope, d.ID, "publish", workflow.PublishDocument{})
			return e
		},
	} {
		if err := call(); !errors.Is(err, execution.ErrNotFound) {
			t.Fatalf("revoked access accepted %s: %v", name, err)
		}
	}
}

func TestWorkflowPaginationAndInvalidStoreInputs(t *testing.T) {
	f, ctx := executionDatabase(t), t.Context()
	for i := range 5 {
		createWorkflow(t, f, fmt.Sprintf("doc-%d", i))
	}
	var ids []string
	after := ""
	for {
		page, err := f.s.ListWorkflows(ctx, f.scope, after, 2)
		if err != nil || page.Items == nil {
			t.Fatalf("list: %+v %v", page, err)
		}
		for _, d := range page.Items {
			if len(ids) > 0 && ids[len(ids)-1] >= d.ID {
				t.Fatal("pagination repeated or reordered a document")
			}
			ids = append(ids, d.ID)
		}
		if after = page.NextCursor; after == "" {
			break
		}
	}
	if len(ids) != 5 {
		t.Fatal(ids)
	}
	for _, graph := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`"text"`), json.RawMessage(`{"text":"\u0000"}`)} {
		if _, err := f.s.CreateWorkflow(ctx, f.scope, "invalid", workflow.CreateDocument{Name: "Invalid", Graph: graph}); !errors.Is(err, execution.ErrInvalidRequest) {
			t.Fatalf("accepted graph %s: %v", graph, err)
		}
	}
	for _, limit := range []int{0, -1, 101} {
		if _, err := f.s.ListWorkflows(ctx, f.scope, "", limit); !errors.Is(err, execution.ErrInvalidRequest) {
			t.Fatalf("accepted limit %d: %v", limit, err)
		}
	}
	for _, id := range []string{"invalid", "00000000_0000_4000_8000_000000000001"} {
		if _, err := f.s.GetWorkflow(ctx, f.scope, id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("malformed id: %v", err)
		}
	}
	d := createWorkflow(t, f, "Empty snapshots")
	page, err := f.s.ListWorkflowSnapshots(ctx, f.scope, d.ID, "", 50)
	if err != nil || !reflect.DeepEqual(page.Items, []workflow.Snapshot{}) || page.NextCursor != "" {
		t.Fatalf("empty history: %+v %v", page, err)
	}
}
