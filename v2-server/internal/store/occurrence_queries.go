package store

import "context"

// GetOccurrence reloads a single record through current membership and creator ownership.
func (s *Store) GetOccurrence(ctx context.Context, tenant, user, id string) (Occurrence, error) {
	ok, e := s.ScheduleMember(ctx, tenant, user)
	if e != nil {
		return Occurrence{}, e
	}
	if !ok {
		return Occurrence{}, ErrNotFound
	}
	return scanOccurrence(s.pool.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE id=$1 AND schedule_id IN (SELECT id FROM v2_schedule_configs WHERE tenant_id=$2 AND user_id=$3)`, id, tenant, user))
}

// FindManualOccurrence retrieves the immutable request timestamp before a retry creates one.
func (s *Store) FindManualOccurrence(ctx context.Context, tenant, user, scheduleID, key string) (Occurrence, error) {
	ok, e := s.ScheduleMember(ctx, tenant, user)
	if e != nil {
		return Occurrence{}, e
	}
	if !ok {
		return Occurrence{}, ErrNotFound
	}
	return scanOccurrence(s.pool.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 AND kind='manual' AND trigger_key=$2 AND schedule_id IN (SELECT id FROM v2_schedule_configs WHERE tenant_id=$3 AND user_id=$4)`, scheduleID, key, tenant, user))
}

// RecoverableOccurrences includes unsent records and all records holding an execution slot.
// It is an internal startup scan, not an unbounded public history endpoint.
func (s *Store) RecoverableOccurrences(ctx context.Context, tenant, user string) ([]Occurrence, error) {
	return s.pendingOccurrences(ctx, tenant, user, true)
}

// ActiveOccurrences includes terminal results awaiting resource cleanup as well as live work.
func (s *Store) ActiveOccurrences(ctx context.Context, tenant, user string) ([]Occurrence, error) {
	return s.pendingOccurrences(ctx, tenant, user, false)
}

func (s *Store) pendingOccurrences(ctx context.Context, tenant, user string, includeUnsent bool) ([]Occurrence, error) {
	ok, e := s.ScheduleMember(ctx, tenant, user)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrNotFound
	}
	rows, e := s.pool.Query(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE (NOT released OR ($3 AND status='recorded')) AND schedule_id IN (SELECT id FROM v2_schedule_configs WHERE tenant_id=$1 AND user_id=$2) ORDER BY recorded_at,id`, tenant, user, includeUnsent)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Occurrence{}
	for rows.Next() {
		o, e := scanOccurrence(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
