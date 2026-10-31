package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"os"
	"testing"
	"time"
)

func TestOccurrenceDeduplicationAndSnapshots(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	var b [8]byte
	_, _ = rand.Read(b[:])
	schema := "occurrence_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	token, e := s.Login(ctx, store.Identity{Source: "test", Subject: "owner", DisplayName: "Owner"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	u, e := s.SessionUser(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenants, e := s.Tenants(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	tid := tenants[0].ID
	in := store.ScheduleInput{Name: "original", Target: json.RawMessage(`{"kind":"chat","prompt":"original prompt","agent":"","model":""}`), Rule: json.RawMessage(`{"kind":"hours","time":"09:00","days":[],"hours":1,"timezone":"UTC"}`)}
	cfg, e := s.CreateScheduleConfig(ctx, tid, u.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	at := cfg.CreatedAt.Add(time.Hour)
	trigger := store.Trigger{Kind: "scheduled", At: at, Version: 1}
	start := make(chan struct{})
	results := make(chan store.Occurrence, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			<-start
			o, e := s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, at)
			results <- o
			errs <- e
		}()
	}
	close(start)
	first := ""
	for i := 0; i < 8; i++ {
		o := <-results
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
		if first == "" {
			first = o.ID
		}
		if o.ID != first || o.Status != "recorded" {
			t.Fatal("duplicate or fabricated execution")
		}
	}
	in.Name = "edited"
	in.Version = 1
	_, e = s.UpdateScheduleConfig(ctx, tid, u.ID, cfg.ID, in, nil)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, at)
	if e != nil || retry.ID != first {
		t.Fatalf("retry %v %v", retry, e)
	}
	var snapshot store.ScheduleConfig
	if e = json.Unmarshal(retry.Snapshot, &snapshot); e != nil || snapshot.Name != "original" {
		t.Fatalf("snapshot %v %v", snapshot, e)
	}
	trigger.Version = 2
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, at); !errors.Is(e, store.ErrConflict) {
		t.Fatalf("different version %v", e)
	}
	trigger.At = at.Add(time.Minute)
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, trigger.At); !errors.Is(e, store.ErrIneligible) {
		t.Fatalf("off grid %v", e)
	}
	trigger.At = at.Add(time.Hour)
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, trigger.At); e != nil {
		t.Fatal(e)
	}
	paused := false
	_, e = s.UpdateScheduleConfig(ctx, tid, u.ID, cfg.ID, store.ScheduleInput{Version: 2}, &paused)
	if e != nil {
		t.Fatal(e)
	}
	trigger.Version = 3
	trigger.At = at.Add(2 * time.Hour)
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, trigger, trigger.At); !errors.Is(e, store.ErrIneligible) {
		t.Fatalf("paused %v", e)
	}
	manual := store.Trigger{Kind: "manual", Key: "manual-1", At: trigger.At, Version: 3}
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, manual, manual.At); e != nil {
		t.Fatal(e)
	}
	manual.Key = "manual-2"
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, manual, manual.At); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	items, e := s.Occurrences(ctx, tid, u.ID, cfg.ID, store.OccurrenceCursor{}, 100)
	if e != nil || len(items) != 4 {
		t.Fatalf("persistent history %d %v", len(items), e)
	}
	page, e := s.Occurrences(ctx, tid, u.ID, cfg.ID, store.OccurrenceCursor{RecordedAt: items[1].RecordedAt, ID: items[1].ID}, 2)
	if e != nil || len(page) != 2 || page[0].ID != items[2].ID || page[1].ID != items[3].ID {
		t.Fatalf("pagination %v %v", page, e)
	}
	other, e := s.Login(ctx, store.Identity{Source: "test", Subject: "other", DisplayName: "Other"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	otherUser, _ := s.SessionUser(ctx, other)
	if _, e = s.Occurrences(ctx, tid, otherUser.ID, cfg.ID, store.OccurrenceCursor{}, 100); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("foreign read %v", e)
	}
	if _, e = s.RecordOccurrence(ctx, tid, otherUser.ID, cfg.ID, manual, manual.At); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("foreign write %v", e)
	}
}

func TestOccurrencesTimeDescendingPagination(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	var b [8]byte
	_, _ = rand.Read(b[:])
	schema := "occurrence_page_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	token, e := s.Login(ctx, store.Identity{Source: "test", Subject: "pager", DisplayName: "Pager"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	u, e := s.SessionUser(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenants, e := s.Tenants(ctx, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	tid := tenants[0].ID
	in := store.ScheduleInput{
		Name:   "paged",
		Target: json.RawMessage(`{"kind":"chat","prompt":"hello","agent":"","model":""}`),
		Rule:   json.RawMessage(`{"kind":"hours","time":"09:00","days":[],"hours":1,"timezone":"UTC"}`),
	}
	cfg, e := s.CreateScheduleConfig(ctx, tid, u.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	unrelated, e := s.CreateScheduleConfig(ctx, tid, u.ID, in)
	if e != nil {
		t.Fatal(e)
	}
	base := cfg.CreatedAt.UTC().Truncate(time.Microsecond)
	if _, e = s.RecordOccurrence(ctx, tid, u.ID, unrelated.ID, store.Trigger{Kind: "manual", Key: "noise", At: unrelated.CreatedAt.Add(time.Minute), Version: 1}, unrelated.CreatedAt.Add(2*time.Hour)); e != nil {
		t.Fatal(e)
	}

	// Include distinct timestamps, sub-second microsecond offsets, and three records sharing
	// the exact same recorded_at instant so page boundaries split equal-timestamp ties.
	specs := []struct {
		key        string
		recordedAt time.Time
	}{
		{"m-1", base.Add(1 * time.Hour)},
		{"m-2", base.Add(2 * time.Hour)},
		{"m-3a", base.Add(3 * time.Hour)},
		{"m-3b", base.Add(3 * time.Hour)},
		{"m-3c", base.Add(3 * time.Hour)},
		{"m-4", base.Add(3*time.Hour + 500*time.Millisecond)},
		{"m-5", base.Add(4 * time.Hour)},
	}
	expected := map[string]store.Occurrence{}
	for _, sp := range specs {
		o, err := s.RecordOccurrence(ctx, tid, u.ID, cfg.ID, store.Trigger{Kind: "manual", Key: sp.key, At: base.Add(time.Minute), Version: 1}, sp.recordedAt)
		if err != nil {
			t.Fatalf("record %s: %v", sp.key, err)
		}
		if !o.RecordedAt.Equal(sp.recordedAt) {
			t.Fatalf("record %s: recordedAt %v want %v", sp.key, o.RecordedAt, sp.recordedAt)
		}
		expected[o.ID] = o
	}

	for _, pageSize := range []int{1, 2, 3, 7, 100} {
		var collected []store.Occurrence
		seen := map[string]bool{}
		var cursor store.OccurrenceCursor
		for {
			page, err := s.Occurrences(ctx, tid, u.ID, cfg.ID, cursor, pageSize)
			if err != nil {
				t.Fatalf("limit=%d cursor=%+v: %v", pageSize, cursor, err)
			}
			if len(page) == 0 {
				break
			}
			if len(page) > pageSize {
				t.Fatalf("limit=%d got %d rows", pageSize, len(page))
			}
			for _, item := range page {
				if seen[item.ID] {
					t.Fatalf("limit=%d duplicate occurrence %s", pageSize, item.ID)
				}
				if _, ok := expected[item.ID]; !ok {
					t.Fatalf("limit=%d unexpected occurrence %s from schedule %s", pageSize, item.ID, item.ScheduleID)
				}
				if len(collected) > 0 {
					prev := collected[len(collected)-1]
					if prev.RecordedAt.Before(item.RecordedAt) || (prev.RecordedAt.Equal(item.RecordedAt) && prev.ID <= item.ID) {
						t.Fatalf("limit=%d unordered pair: prev=(%v,%s) curr=(%v,%s)", pageSize, prev.RecordedAt, prev.ID, item.RecordedAt, item.ID)
					}
				}
				seen[item.ID] = true
				collected = append(collected, item)
			}
			last := page[len(page)-1]
			cursor = store.OccurrenceCursor{RecordedAt: last.RecordedAt, ID: last.ID}
			if len(page) < pageSize {
				tail, err := s.Occurrences(ctx, tid, u.ID, cfg.ID, cursor, pageSize)
				if err != nil || len(tail) != 0 {
					t.Fatalf("limit=%d expected empty tail after %+v, got %v %v", pageSize, cursor, tail, err)
				}
				break
			}
		}
		if len(collected) != len(expected) {
			t.Fatalf("limit=%d collected %d want %d", pageSize, len(collected), len(expected))
		}
		if !collected[0].RecordedAt.Equal(specs[len(specs)-1].recordedAt) || !collected[len(collected)-1].RecordedAt.Equal(specs[0].recordedAt) {
			t.Fatalf("limit=%d bounds=(%v..%v)", pageSize, collected[0].RecordedAt, collected[len(collected)-1].RecordedAt)
		}
	}

	sampleID := "00000000-0000-0000-0000-000000000001"
	for _, badLimit := range []int{-1, 0, 101} {
		if _, err := s.Occurrences(ctx, tid, u.ID, cfg.ID, store.OccurrenceCursor{}, badLimit); !errors.Is(err, store.ErrIneligible) {
			t.Fatalf("limit=%d err=%v want ErrIneligible", badLimit, err)
		}
	}
	for _, badCursor := range []store.OccurrenceCursor{
		{RecordedAt: base},
		{ID: sampleID},
		{RecordedAt: base, ID: "not-a-uuid"},
		{RecordedAt: base.Add(500 * time.Nanosecond), ID: sampleID},
	} {
		if _, err := s.Occurrences(ctx, tid, u.ID, cfg.ID, badCursor, 10); !errors.Is(err, store.ErrIneligible) {
			t.Fatalf("cursor=%+v err=%v want ErrIneligible", badCursor, err)
		}
	}
}
