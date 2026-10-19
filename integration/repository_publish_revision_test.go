package integration

import (
	"testing"

	"github.com/wanglongan587/cloud/internal/cloneauth"
	"github.com/wanglongan587/cloud/internal/core"
)

// Publishing a run's Revision publishes what Cloud verified when it registered the Revision: its
// delivery ref on the Git Server and its final commit (publish ADR, revisionId; revision ADR D4).
// The Revision's revisionRef is only the Node-local refs/ora/revisions/<run> and never exists on
// the Git Server, so it must never become a publish source.
func TestPublishingARevisionUsesItsVerifiedDeliveryRef(t *testing.T) {
	f := setup(t)
	f.useObjectStore()
	scene := deliveringScene(t, f)
	execution := f.deliverRevision(t, scene)
	in := f.deliveryInputOf(execution)
	f.push(in, revisionFinalCommit)
	if _, e := f.deliveryTerminal(scene, execution, "publish-revision-saved", 1, deliveredResult(scene, in, revisionFinalCommit), "revision_delivered"); e != nil {
		t.Fatalf("a verified delivery must be taken over: %v", e)
	}
	row := f.revisionRow(scene.runID)
	if row == nil || row.DeliveryRef == "" || row.DeliveryRef == row.RevisionRef {
		t.Fatalf("the run must have registered a Git Revision with its own delivery ref: %+v", row)
	}

	// The scene lives in its own tenant, where the fixture's user is an administrator.
	f.tid = scene.tenantID
	projectID, repositoryURL := f.runPlacement(scene.runID)
	scope, err := cloneauth.NormalizeRepository(repositoryURL)
	must(t, err)
	credential := f.storedGitCredential(f.uid, "team", scope)
	must(t, firstErr(f.store.Pool.Exec(`UPDATE credential_refs SET capabilities='["publish"]' WHERE id=$1`, credential)))
	f.store.GitRefs = publishRefs{}

	path := f.path("/projects/" + projectID + "/repository/publishes")
	body := core.Object{"revisionId": row.ID, "targetBranch": "main", "expectedOldOid": revisionBaseCommitOf(t, f, row.ID), "credentialRefId": credential}
	op := f.gitCall("POST", path, body, "", 201).O("resource")
	if op.S("sourceRef") != row.DeliveryRef || op.S("sourceCommit") != revisionFinalCommit || op.S("revisionId") != row.ID {
		t.Fatalf("a Revision publish must use the verified delivery ref %s and final commit %s, got %v", row.DeliveryRef, revisionFinalCommit, op)
	}
}

// revisionBaseCommitOf is the commit the Revision started from, a target branch value that differs
// from its final commit so the publish has something to fast-forward.
func revisionBaseCommitOf(t *testing.T, f *fixture, revisionID string) string {
	t.Helper()
	var base string
	must(t, f.store.Pool.QueryRow(`SELECT base_commit FROM revisions WHERE id=$1`, revisionID).Scan(&base))
	return base
}
