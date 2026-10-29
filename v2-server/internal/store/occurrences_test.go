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
	items, e := s.Occurrences(ctx, tid, u.ID, cfg.ID, "", 100)
	if e != nil || len(items) != 4 {
		t.Fatalf("persistent history %d %v", len(items), e)
	}
	page, e := s.Occurrences(ctx, tid, u.ID, cfg.ID, items[1].ID, 2)
	if e != nil || len(page) != 2 || page[0].ID != items[2].ID {
		t.Fatalf("pagination %v %v", page, e)
	}
	other, e := s.Login(ctx, store.Identity{Source: "test", Subject: "other", DisplayName: "Other"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	otherUser, _ := s.SessionUser(ctx, other)
	if _, e = s.Occurrences(ctx, tid, otherUser.ID, cfg.ID, "", 100); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("foreign read %v", e)
	}
	if _, e = s.RecordOccurrence(ctx, tid, otherUser.ID, cfg.ID, manual, manual.At); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("foreign write %v", e)
	}
}
