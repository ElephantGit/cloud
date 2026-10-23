package core

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// scheduledArtifactReadScene registers one artifact for a scheduled run and returns the request that
// reads it, with the claims of a member of the run's own issue.
func scheduledArtifactReadScene(t *testing.T, store *Store, withStorage bool) (*PublicRequest, string) {
	t.Helper()
	config, _ := cleanupObjectStore(t)
	if withStorage {
		store.ObjectStore = config
	}
	scene := seedTakeoverScene(t, store)
	stageScheduledOccurrence(t, store, scene.run)
	var tenantID, issueID, ownerID, taskID string
	if err := store.Pool.QueryRow(`
		SELECT r.tenant_id, r.issue_id, i.creator_user_id, o.task_id
		FROM issue_runs r
		JOIN issues i ON i.id = r.issue_id
		JOIN scheduled_task_occurrences o ON o.run_id = r.id
		WHERE r.id=$1`, scene.run).Scan(&tenantID, &issueID, &ownerID, &taskID); err != nil {
		t.Fatalf("read the scheduled run: %v", err)
	}
	claims := &Claims{
		Source: "test-artifact-reader", Caller: "gateway",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "artifact-reader"},
	}
	historyExec(t, store, "INSERT INTO user_identities(user_id,source,subject) VALUES($1,$2,$3)", ownerID, claims.Source, claims.Subject)
	key := scheduledArtifactKey(tenantID, scene.run, 0)
	if _, err := store.Pool.Exec(`
		INSERT INTO scheduled_run_artifacts
			(tenant_id, issue_run_id, task_id, scheduled_at, artifact_index, object_key, display_name, kind, size_bytes, sha256)
		VALUES($1,$2,$3, now(), 0, $4, 'reports/review.md', 'markdown', 5, $5)`,
		tenantID, scene.run, taskID, key, strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("register the artifact: %v", err)
	}
	return &PublicRequest{
		Method: "GET", Path: "/api/v1/tenants/" + tenantID + "/issues/" + issueID + "/runs/" + scene.run + "/artifacts/0",
		TenantID: tenantID, IssueID: issueID, RunID: scene.run, ArtifactIndex: "0", Identity: claims,
	}, key
}

// A registered artifact is readable by a member of its issue, and only when it is registered.
func TestSessionRunArtifactServesOnlyRegisteredArtifacts(t *testing.T) {
	store := dispatcherDB(t)
	request, key := scheduledArtifactReadScene(t, store, true)
	read, status, err := store.SessionRunArtifact(context.Background(), request)
	if err != nil || status != 200 {
		t.Fatalf("read = %d %v, want 200", status, err)
	}
	if read.S("name") != "reports/review.md" || read.S("kind") != "markdown" || read.N("sizeBytes") != 5 ||
		read.S("sha256") != strings.Repeat("ab", 32) {
		t.Fatalf("read = %v, want the registered artifact's own facts", read)
	}
	// The URL must be a capability for exactly the key Cloud registered, not for a caller-chosen one.
	signed, err := url.Parse(read.S("url"))
	if err != nil || !strings.HasSuffix(signed.Path, key) || signed.Query().Get("X-Amz-Signature") == "" || read["expiresAt"] == nil {
		t.Fatalf("url = %q expiresAt = %v, want a signed read of %s", read.S("url"), read["expiresAt"], key)
	}
	// An index the run never registered - refused, never uploaded, or pruned - is not there.
	outOfRange := *request
	outOfRange.ArtifactIndex = "1"
	var notFound *Fault
	if _, _, err := store.SessionRunArtifact(context.Background(), &outOfRange); !errors.As(err, &notFound) || notFound.Code != "artifact_unavailable" {
		t.Fatalf("unregistered index err = %v, want artifact_unavailable", err)
	}
}

// A deployment without object storage cannot hand out a URL, and says so instead of returning one.
func TestSessionRunArtifactRefusesWithoutObjectStorage(t *testing.T) {
	store := dispatcherDB(t)
	request, _ := scheduledArtifactReadScene(t, store, false)
	store.ObjectStore = nil
	var unavailable *Fault
	if _, _, err := store.SessionRunArtifact(context.Background(), request); !errors.As(err, &unavailable) || unavailable.Status != 503 || unavailable.Code != "object_store_unavailable" {
		t.Fatalf("err = %v, want 503 object_store_unavailable", err)
	}
}

// The list read tells a reader what a run registered without minting a single capability.
func TestSessionRunArtifactsListsWhatWasRegistered(t *testing.T) {
	store := dispatcherDB(t)
	request, key := scheduledArtifactReadScene(t, store, true)
	list := *request
	list.ArtifactIndex = ""
	read, status, err := store.SessionRunArtifacts(context.Background(), &list)
	if err != nil || status != 200 {
		t.Fatalf("list = %d %v, want 200", status, err)
	}
	items, ok := read["artifacts"].([]Object)
	if !ok || len(items) != 1 {
		t.Fatalf("artifacts = %v, want exactly the registered row", read["artifacts"])
	}
	item := items[0]
	if item.N("index") != 0 || item.S("name") != "reports/review.md" || item.S("kind") != "markdown" ||
		item.N("sizeBytes") != 5 || item.S("sha256") != strings.Repeat("ab", 32) {
		t.Fatalf("item = %v, want the registered artifact's own facts", item)
	}
	// Listing must never leak a capability: the key and a signed URL belong to the download read.
	if _, present := item["objectKey"]; present {
		t.Fatalf("item = %v, must not expose the object key", item)
	}
	if _, present := item["url"]; present {
		t.Fatalf("item = %v, must not mint a URL", item)
	}
	if !strings.HasSuffix(key, "/0") {
		t.Fatalf("key = %q, want the run's index 0", key)
	}
}

// Both reads must enforce the same live identity, tenant, issue and run scope before returning facts.
func TestSessionRunArtifactReadsRejectWrongScope(t *testing.T) {
	store := dispatcherDB(t)
	request, _ := scheduledArtifactReadScene(t, store, true)
	for _, tc := range []struct {
		name   string
		status int
		change func(*PublicRequest)
	}{
		{"no identity", 401, func(r *PublicRequest) { r.Identity = nil }},
		{"other tenant", 404, func(r *PublicRequest) { r.TenantID = "00000000-0000-4000-8000-000000000001" }},
		{"other issue", 404, func(r *PublicRequest) { r.IssueID = "00000000-0000-4000-8000-000000000002" }},
		{"other run", 404, func(r *PublicRequest) { r.RunID = "00000000-0000-4000-8000-000000000003" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scoped := *request
			tc.change(&scoped)
			for _, read := range []func(context.Context, *PublicRequest) (Object, int, error){store.SessionRunArtifacts, store.SessionRunArtifact} {
				object, _, err := read(context.Background(), &scoped)
				var fault *Fault
				if !errors.As(err, &fault) || fault.Status != tc.status || object != nil {
					t.Fatalf("wrong-scope read returned %v, %v; want no facts and a public fault", object, err)
				}
			}
		})
	}
}

// A compromised registration must not become a capability for another object.
func TestSessionRunArtifactRefusesAnOutOfScopeObjectKey(t *testing.T) {
	store := dispatcherDB(t)
	request, _ := scheduledArtifactReadScene(t, store, true)
	historyExec(t, store, "UPDATE scheduled_run_artifacts SET object_key=$1 WHERE issue_run_id=$2",
		"scheduled-runs/another-tenant/another-run/0", request.RunID)
	object, _, err := store.SessionRunArtifact(context.Background(), request)
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "artifact_invalid" || object != nil {
		t.Fatalf("tampered-key read returned %v, %v; want artifact_invalid without a grant", object, err)
	}
}
