package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// ExecutionEvidence is supplied only by the trusted execution adapter, never an HTTP body.
// NotAccepted follows a completed Start or the public startup recovery barrier.
// Released confirms session End, not directory deletion. Ref is an opaque stable public ref.
type ExecutionEvidence struct{ Event, Ref, Reason string }

func terminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "cancelled" || status == "interrupted"
}

// ApplyOccurrenceEvidence serializes transitions with configuration and trigger creation.
// Begin reserves a task once; only its successful caller may request external admission,
// using the immutable occurrence ID. All retries reconcile that identity, never dispatch anew.
// expectedVersion fences stale callbacks. Public execution calls never occur in this transaction.
func (s *Store) ApplyOccurrenceEvidence(ctx context.Context, tenant, user, id string, expectedVersion int64, proof ExecutionEvidence) (Occurrence, error) {
	var out Occurrence
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var scheduleID string
		e := tx.QueryRow(ctx, `SELECT c.id FROM v2_schedule_configs c JOIN v2_schedule_occurrences o ON o.schedule_id=c.id JOIN memberships m ON m.tenant_id=c.tenant_id AND m.user_id=c.user_id JOIN users u ON u.id=c.user_id JOIN tenants t ON t.id=c.tenant_id WHERE o.id=$1 AND c.tenant_id=$2 AND c.user_id=$3 AND u.status='active' AND u.deleted_at IS NULL AND t.status='active' FOR UPDATE OF c`, id, tenant, user).Scan(&scheduleID)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		o, e := scanOccurrence(tx.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE id=$1 FOR UPDATE`, id))
		if e != nil {
			return e
		}
		if o.Version != expectedVersion {
			return ErrConflict
		}
		if len(proof.Ref) > 512 || len(proof.Reason) > 256 {
			return ErrIneligible
		}
		switch proof.Event {
		case "begin":
			if o.Status != "recorded" {
				return ErrIneligible
			}
			var active bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v2_schedule_occurrences WHERE schedule_id=$1 AND NOT released)`, scheduleID).Scan(&active); e != nil {
				return e
			}
			if active {
				o.Status, o.Reason = "skipped", "previous_unfinished"
			} else {
				var enabled bool
				var version int64
				if e = tx.QueryRow(ctx, `SELECT enabled,version FROM v2_schedule_configs WHERE id=$1`, scheduleID).Scan(&enabled, &version); e != nil {
					return e
				}
				if version != o.ConfigVersion {
					o.Status, o.Reason = "skipped", "configuration_changed"
				} else if o.Kind == "scheduled" && !enabled {
					o.Status, o.Reason = "skipped", "paused"
				} else {
					o.Status, o.Released = "starting", false
				}
			}
		case "rejected":
			// Only a definitive admission refusal is evidence. Conflicts and ambiguous
			// outcomes may conceal an existing execution and must retain their slot.
			if o.Status != "starting" || proof.Reason == "" {
				return ErrIneligible
			}
			o.Status, o.Reason, o.Released = "skipped", proof.Reason, true
		case "reconcile":
			if o.Status != "starting" {
				return ErrIneligible
			}
			o.Status, o.Reason = "reconciling", "lookup_required"
		case "accepted":
			if o.Status != "starting" && o.Status != "reconciling" {
				return ErrIneligible
			}
			if proof.Ref == "" || (o.ExecutionRef != "" && o.ExecutionRef != proof.Ref) {
				return ErrConflict
			}
			o.ExecutionRef = proof.Ref
			o.Status, o.Reason = "running", ""
		case "not_accepted":
			if (o.Status != "starting" && o.Status != "reconciling") || o.ExecutionRef != "" {
				return ErrIneligible
			}
			o.Status, o.Reason, o.Released = "skipped", "execution_not_started", true
		case "cancel_requested":
			if o.Status != "running" {
				return ErrIneligible
			}
			o.Status = "cancelling"
		case "completed", "failed", "cancelled", "interrupted":
			if o.Status != "running" && o.Status != "cancelling" && o.Status != "reconciling" {
				return ErrIneligible
			}
			if o.ExecutionRef == "" || proof.Ref != o.ExecutionRef {
				return ErrConflict
			}
			o.Status = proof.Event
			if proof.Event == "completed" {
				o.Status = "succeeded"
			}
			o.Reason = proof.Reason
			// A result does not prove that the dedicated session has ended.
		case "released":
			if !terminal(o.Status) || o.Released || proof.Ref != o.ExecutionRef {
				return ErrIneligible
			}
			o.Released = true
		case "recover":
			switch o.Status {
			case "recorded":
				o.Status, o.Reason = "skipped", "server_restarted"
			case "starting", "running", "cancelling":
				o.Status, o.Reason = "reconciling", "server_restarted"
			default:
				out = o
				return nil
			}
		default:
			return ErrIneligible
		}
		out, e = scanOccurrence(tx.QueryRow(ctx, `UPDATE v2_schedule_occurrences SET status=$2,reason=$3,execution_ref=$4,released=$5,version=version+1 WHERE id=$1 RETURNING `+occurrenceColumns, id, o.Status, o.Reason, o.ExecutionRef, o.Released))
		return e
	})
	return out, err
}
