package core

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/wanglongan587/cloud/internal/objectstore"
)

// Reading one registered scheduled artifact (ADR D4).
//
// The read is two steps for the same reason the Revision history read is: authorisation and the
// registered row belong to one short transaction, and the object store must never be called inside
// it. The row decides which object is served - a caller names a run and an index, never a key - and
// the persisted key is re-checked against the layout Cloud generates, so a stored value cannot turn
// into an arbitrary object read.

// SessionRunArtifact returns a short-lived download URL for one artifact of a run.
//
// Visibility follows the issue: the caller must be an effective tenant member and the run must live
// in the named issue. An artifact that was never registered - because it was refused, never
// uploaded, or pruned by retention - is a 404 rather than an empty URL, since a reader that is told
// "not there" can act on it while a broken link cannot.
func (s *Store) SessionRunArtifact(ctx context.Context, r *PublicRequest) (Object, int, error) {
	artifact, err := s.transact(ctx, func(t *transaction) Object {
		return scheduledArtifactRegistration(t, r)
	})
	if err != nil {
		return nil, 0, err
	}
	if s.ObjectStore == nil {
		return nil, 0, &Fault{Status: 503, Code: "object_store_unavailable", Params: Object{}}
	}
	grant, err := objectstore.PresignGET(s.ObjectStore, artifact.S("objectKey"), time.Now())
	if err != nil {
		return nil, 0, &Fault{Status: 503, Code: "object_store_unavailable", Params: Object{}}
	}
	out := Object{
		"name": artifact.S("displayName"), "kind": artifact.S("kind"),
		"sizeBytes": artifact.N("sizeBytes"), "sha256": artifact.S("sha256"),
		"url": grant.URL, "expiresAt": grant.Expires.UTC(),
	}
	return out, 200, nil
}

// scheduledArtifactRegistration authorises the read and returns the registered artifact row.
func scheduledArtifactRegistration(t *transaction, r *PublicRequest) Object {
	require(r != nil && r.Method == "GET", 405, "method_not_allowed")
	uid := sessionReadIdentity(t, r.Identity)
	membership(t, r.TenantID, uid, false)
	issue(t, r.TenantID, r.IssueID)
	require(validID(r.RunID), 404, "not_found")
	run := t.one(`SELECT id FROM issue_runs
		WHERE id=$1 AND tenant_id=$2 AND issue_id=$3 AND deleted_at IS NULL`, r.RunID, r.TenantID, r.IssueID)
	require(run != nil, 404, "not_found")
	index, err := strconv.Atoi(r.ArtifactIndex)
	require(err == nil && index >= 0 && index < maxScheduledArtifactItems, 404, "artifact_unavailable")
	artifact := t.one(`SELECT object_key, display_name, kind, size_bytes, sha256
		FROM scheduled_run_artifacts WHERE issue_run_id=$1 AND artifact_index=$2 AND tenant_id=$3`,
		r.RunID, index, r.TenantID)
	require(artifact != nil, 404, "artifact_unavailable")
	// The layout is Cloud's own (`scheduledArtifactKey`), so a persisted key still cannot cross scope,
	// become an arbitrary read, or name another run's index.
	parts := strings.Split(artifact.S("objectKey"), "/")
	require(len(parts) == 4 && parts[0] == "scheduled-runs" && parts[1] == r.TenantID &&
		parts[2] == r.RunID && parts[3] == strconv.Itoa(index), 502, "artifact_invalid")
	return artifact
}
