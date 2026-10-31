package store

import (
	"context"
	"github.com/jackc/pgx/v5"
)

const historyOwner = `SELECT EXISTS(SELECT 1 FROM v2_schedule_configs c JOIN memberships m ON m.tenant_id=c.tenant_id AND m.user_id=c.user_id JOIN users u ON u.id=c.user_id JOIN tenants t ON t.id=c.tenant_id WHERE c.id=$1 AND c.tenant_id=$2 AND c.user_id=$3 AND u.status='active' AND u.deleted_at IS NULL AND t.status='active')`

// ScheduleHistory returns a stable time-descending (recorded_at DESC, id DESC) page plus one
// lookahead row. Authorization and reads share a database snapshot. No status reconciliation
// or execution is performed.
func (s *Store) ScheduleHistory(ctx context.Context, tenant, user, scheduleID string, before OccurrenceCursor, limit int) ([]Occurrence, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrIneligible
	}
	cursor, ok := normalizeOccurrenceCursor(before)
	if !ok {
		return nil, ErrIneligible
	}
	out := []Occurrence{}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, historyOwner, scheduleID, tenant, user).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return ErrNotFound
		}
		var (
			rows pgx.Rows
			err  error
		)
		if cursor.ID == "" {
			rows, err = tx.Query(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 ORDER BY recorded_at DESC,id DESC LIMIT $2`, scheduleID, limit+1)
		} else {
			rows, err = tx.Query(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 AND (recorded_at,id)<($2::timestamptz,$3::uuid) ORDER BY recorded_at DESC,id DESC LIMIT $4`, scheduleID, cursor.RecordedAt, cursor.ID, limit+1)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scanOccurrence(rows)
			if err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// ScheduleHistoryItem hides absent, foreign and inaccessible records behind ErrNotFound.
func (s *Store) ScheduleHistoryItem(ctx context.Context, tenant, user, scheduleID, id string) (Occurrence, error) {
	return scanOccurrence(s.pool.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE id=$4 AND schedule_id=$1 AND (`+historyOwner+`)`, scheduleID, tenant, user, id))
}
