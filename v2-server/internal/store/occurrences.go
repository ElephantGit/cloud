package store

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/schedule"
)

// ErrIneligible means a new automatic trigger is paused, future, or outside its saved rule.
var ErrIneligible = errors.New("trigger ineligible")

var occurrenceUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Trigger identifies a scheduled instant or an explicitly requested manual occurrence.
// Key is a caller-generated stable manual request ID; automatic keys are derived internally.
type Trigger struct {
	Kind, Key string
	At        time.Time
	Version   int64
}

// OccurrenceCursor identifies the last seen row in descending (recorded_at, id) order.
// A zero value starts from the newest occurrence.
type OccurrenceCursor struct {
	RecordedAt time.Time
	ID         string
}

func normalizeOccurrenceCursor(before OccurrenceCursor) (OccurrenceCursor, bool) {
	if before.RecordedAt.IsZero() && before.ID == "" {
		return OccurrenceCursor{}, true
	}
	if before.RecordedAt.IsZero() || !occurrenceUUID.MatchString(before.ID) {
		return OccurrenceCursor{}, false
	}
	utc := before.RecordedAt.UTC()
	if !utc.Equal(utc.Truncate(time.Microsecond)) {
		return OccurrenceCursor{}, false
	}
	return OccurrenceCursor{RecordedAt: utc, ID: strings.ToLower(before.ID)}, true
}

// Occurrence is a frozen domain record, not proof that any execution started.
// Public execution sessions and events remain owned by the shared execution module.
type Occurrence struct {
	ID            string          `json:"id"`
	ScheduleID    string          `json:"scheduleId"`
	Kind          string          `json:"kind"`
	Key           string          `json:"key"`
	ScheduledAt   time.Time       `json:"scheduledAt"`
	ConfigVersion int64           `json:"configVersion"`
	Snapshot      json.RawMessage `json:"snapshot"`
	Status        string          `json:"status"`
	RecordedAt    time.Time       `json:"recordedAt"`
	Reason        string          `json:"reason"`
	ExecutionRef  string          `json:"executionRef"`
	Released      bool            `json:"released"`
	Version       int64           `json:"version"`
}

const occurrenceColumns = `id,schedule_id,kind,trigger_key,scheduled_at,config_version,snapshot,status,recorded_at,reason,execution_ref,released,version`

func scanOccurrence(row pgx.Row) (Occurrence, error) {
	var o Occurrence
	e := row.Scan(&o.ID, &o.ScheduleID, &o.Kind, &o.Key, &o.ScheduledAt, &o.ConfigVersion, &o.Snapshot, &o.Status, &o.RecordedAt, &o.Reason, &o.ExecutionRef, &o.Released, &o.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return o, e
}

// RecordOccurrence atomically freezes configuration and deduplicates a trigger.
// now is supplied by the service clock; no polling loop or executor calls are installed here.
func (s *Store) RecordOccurrence(ctx context.Context, tenant, user, id string, t Trigger, now time.Time) (Occurrence, error) {
	var out Occurrence
	t.At = t.At.UTC().Truncate(time.Microsecond)
	now = now.UTC().Truncate(time.Microsecond)
	if t.At.IsZero() || now.IsZero() || t.Version < 1 || t.At.After(now) {
		return out, ErrIneligible
	}
	switch t.Kind {
	case "scheduled":
		if t.Key != "" {
			return out, ErrIneligible
		}
		t.Key = t.At.Format(time.RFC3339Nano)
	case "manual":
		if t.Key == "" || len(t.Key) > 200 {
			return out, ErrIneligible
		}
	default:
		return out, ErrIneligible
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var active bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m JOIN users u ON u.id=m.user_id JOIN tenants t ON t.id=m.tenant_id WHERE m.tenant_id=$1 AND m.user_id=$2 AND u.status='active' AND u.deleted_at IS NULL AND t.status='active')`, tenant, user).Scan(&active)
		if e != nil {
			return e
		}
		if !active {
			return ErrNotFound
		}
		config, e := readSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM v2_schedule_configs WHERE tenant_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, tenant, user, id))
		if e != nil {
			return e
		}
		existing, e := scanOccurrence(tx.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 AND kind=$2 AND trigger_key=$3`, id, t.Kind, t.Key))
		if e == nil {
			if existing.ConfigVersion != t.Version || !existing.ScheduledAt.Equal(t.At) {
				return ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(e, ErrNotFound) {
			return e
		}
		if config.Version != t.Version {
			return ErrConflict
		}
		if t.At.Before(config.CreatedAt) {
			return ErrIneligible
		}
		if t.Kind == "scheduled" {
			if !config.Enabled {
				return ErrIneligible
			}
			var rule schedule.Rule
			if e = json.Unmarshal(config.Rule, &rule); e != nil {
				return ErrIneligible
			}
			next, e := schedule.Next(rule, config.CreatedAt, t.At.Add(-time.Nanosecond), 1)
			if e != nil {
				return ErrIneligible
			}
			if !next[0].Equal(t.At) {
				return ErrIneligible
			}
		}
		snapshot, e := json.Marshal(config)
		if e != nil {
			return e
		}
		var activeRun bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v2_schedule_occurrences WHERE schedule_id=$1 AND NOT released)`, id).Scan(&activeRun); e != nil {
			return e
		}
		status, reason := "recorded", ""
		if activeRun {
			status, reason = "skipped", "previous_unfinished"
		}
		out, e = scanOccurrence(tx.QueryRow(ctx, `INSERT INTO v2_schedule_occurrences(schedule_id,kind,trigger_key,scheduled_at,config_version,snapshot,status,reason,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+occurrenceColumns, id, t.Kind, t.Key, t.At, t.Version, snapshot, status, reason, now))
		return e
	})
	return out, err
}

// Occurrences reads a bounded page in descending (recorded_at, id) order within caller ownership.
func (s *Store) Occurrences(ctx context.Context, tenant, user, id string, before OccurrenceCursor, limit int) ([]Occurrence, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrIneligible
	}
	cursor, ok := normalizeOccurrenceCursor(before)
	if !ok {
		return nil, ErrIneligible
	}
	ok, e := s.ScheduleMember(ctx, tenant, user)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrNotFound
	}
	var found bool
	e = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v2_schedule_configs WHERE id=$1 AND tenant_id=$2 AND user_id=$3)`, id, tenant, user).Scan(&found)
	if e != nil {
		return nil, e
	}
	if !found {
		return nil, ErrNotFound
	}
	var rows pgx.Rows
	if cursor.ID == "" {
		rows, e = s.pool.Query(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 ORDER BY recorded_at DESC,id DESC LIMIT $2`, id, limit)
	} else {
		rows, e = s.pool.Query(ctx, `SELECT `+occurrenceColumns+` FROM v2_schedule_occurrences WHERE schedule_id=$1 AND (recorded_at,id)<($2::timestamptz,$3::uuid) ORDER BY recorded_at DESC,id DESC LIMIT $4`, id, cursor.RecordedAt, cursor.ID, limit)
	}
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
