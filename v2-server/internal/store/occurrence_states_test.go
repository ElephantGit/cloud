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

func TestExecutionStateRules(t *testing.T) {
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
	schema := "states_" + hex.EncodeToString(b[:])
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, e := pgxpool.ParseConfig(url)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, cfg)
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
	config, e := s.CreateScheduleConfig(ctx, tid, u.ID, store.ScheduleInput{Name: "task", Target: json.RawMessage(`{"kind":"chat","prompt":"hello","agent":"","model":""}`), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if e != nil {
		t.Fatal(e)
	}
	at := config.CreatedAt.Add(time.Hour)
	record := func(key string) store.Occurrence {
		t.Helper()
		o, e := s.RecordOccurrence(ctx, tid, u.ID, config.ID, store.Trigger{Kind: "manual", Key: key, At: at, Version: 1}, at)
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	apply := func(o store.Occurrence, event, ref string) store.Occurrence {
		t.Helper()
		r, e := s.ApplyOccurrenceEvidence(ctx, tid, u.ID, o.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: ref})
		if e != nil {
			t.Fatalf("%s from %s: %v", event, o.Status, e)
		}
		if r.ID != o.ID || !r.RecordedAt.Equal(o.RecordedAt) {
			t.Fatalf("%s mutated cursor identity (%v,%s) -> (%v,%s)", event, o.RecordedAt, o.ID, r.RecordedAt, r.ID)
		}
		return r
	}
	a, bRun := record("one"), record("two")
	type result struct {
		o store.Occurrence
		e error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, o := range []store.Occurrence{a, bRun} {
		go func(o store.Occurrence) {
			<-start
			r, e := s.ApplyOccurrenceEvidence(ctx, tid, u.ID, o.ID, o.Version, store.ExecutionEvidence{Event: "begin"})
			results <- result{r, e}
		}(o)
	}
	close(start)
	var active store.Occurrence
	skipped := 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.e != nil {
			t.Fatal(r.e)
		}
		if r.o.Status == "starting" {
			active = r.o
		} else if r.o.Status == "skipped" && r.o.Reason == "previous_unfinished" {
			skipped++
		}
	}
	if active.ID == "" || skipped != 1 {
		t.Fatal("overlapping dispatch permitted")
	}
	overlap := record("manual-overlap")
	if overlap.Status != "skipped" || overlap.Reason != "previous_unfinished" {
		t.Fatal("manual bypass")
	}
	auto, e := s.RecordOccurrence(ctx, tid, u.ID, config.ID, store.Trigger{Kind: "scheduled", At: at, Version: 1}, at)
	if e != nil || auto.Status != "skipped" {
		t.Fatalf("automatic overlap %v %v", auto, e)
	}
	active, e = s.ApplyOccurrenceEvidence(ctx, tid, u.ID, active.ID, active.Version, store.ExecutionEvidence{Event: "rejected", Reason: "execution_busy"})
	if e != nil {
		t.Fatal(e)
	}
	if !active.Released || active.Status != "skipped" || active.Reason != "execution_busy" {
		t.Fatal("busy not terminal")
	}
	if _, e = s.ApplyOccurrenceEvidence(ctx, tid, u.ID, active.ID, active.Version, store.ExecutionEvidence{Event: "begin"}); !errors.Is(e, store.ErrIneligible) {
		t.Fatal("busy automatically retried")
	}
	unknown := apply(record("unknown"), "begin", "")
	unknown = apply(unknown, "reconcile", "")
	if record("blocked-by-unknown").Status != "skipped" {
		t.Fatal("unknown released task")
	}
	if _, e = s.ApplyOccurrenceEvidence(ctx, tid, u.ID, unknown.ID, unknown.Version, store.ExecutionEvidence{Event: "busy"}); !errors.Is(e, store.ErrIneligible) {
		t.Fatal("unknown treated as busy")
	}
	unknown = apply(unknown, "accepted", "ref-1")
	old := unknown
	unknown = apply(unknown, "completed", "ref-1")
	if unknown.Status != "succeeded" || unknown.Released {
		t.Fatal("result released session")
	}
	if _, e = s.ApplyOccurrenceEvidence(ctx, tid, u.ID, old.ID, old.Version, store.ExecutionEvidence{Event: "failed", Ref: "ref-1"}); !errors.Is(e, store.ErrConflict) {
		t.Fatal("stale callback overwrote result")
	}
	if record("blocked-until-end").Status != "skipped" {
		t.Fatal("result bypassed End")
	}
	unknown = apply(unknown, "released", "ref-1")
	if unknown.Status != "succeeded" {
		t.Fatal("cleanup overwrote result")
	}
	pending := record("not-started")
	running := apply(record("running"), "begin", "")
	running = apply(running, "accepted", "ref-2")
	// Reopen preserves durable state; recovery does not redispatch or assume stopped.
	s.Close()
	s, e = store.OpenConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	pending = apply(pending, "recover", "")
	if pending.Status != "skipped" || pending.Reason != "server_restarted" {
		t.Fatal("replayed unsent trigger")
	}
	running = apply(running, "recover", "")
	if running.Status != "reconciling" || running.Released {
		t.Fatal("restart assumed release")
	}
	running = apply(running, "interrupted", "ref-2")
	running = apply(running, "released", "ref-2")
	unaccepted := apply(record("not-accepted"), "begin", "")
	unaccepted = apply(unaccepted, "reconcile", "")
	unaccepted = apply(unaccepted, "recover", "")
	unaccepted = apply(unaccepted, "not_accepted", "")
	if !unaccepted.Released {
		t.Fatal("definitive nonadmission still blocking")
	}
	cancel := apply(record("cancel"), "begin", "")
	cancel = apply(cancel, "accepted", "ref-3")
	cancel = apply(cancel, "cancel_requested", "")
	if cancel.Released {
		t.Fatal("cancel request released")
	}
	cancel = apply(cancel, "cancelled", "ref-3")
	cancel = apply(cancel, "released", "ref-3")
	queued := record("query-unsent")
	got, e := s.GetOccurrence(ctx, tid, u.ID, queued.ID)
	if e != nil || got.ID != queued.ID {
		t.Fatalf("single query %v %v", got, e)
	}
	got, e = s.FindManualOccurrence(ctx, tid, u.ID, config.ID, "query-unsent")
	if e != nil || !got.ScheduledAt.Equal(queued.ScheduledAt) {
		t.Fatalf("manual retry lookup %v %v", got, e)
	}
	if _, e = s.FindManualOccurrence(ctx, tid, u.ID, config.ID, "missing"); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("missing manual identity")
	}
	recoverable, e := s.RecoverableOccurrences(ctx, tid, u.ID)
	if e != nil || len(recoverable) != 1 || recoverable[0].ID != queued.ID {
		t.Fatalf("recoverable %v %v", recoverable, e)
	}
	queued = apply(queued, "begin", "")
	activeRows, e := s.ActiveOccurrences(ctx, tid, u.ID)
	if e != nil || len(activeRows) != 1 || activeRows[0].ID != queued.ID {
		t.Fatalf("active %v %v", activeRows, e)
	}
	queued, e = s.ApplyOccurrenceEvidence(ctx, tid, u.ID, queued.ID, queued.Version, store.ExecutionEvidence{Event: "rejected", Reason: "invalid_model"})
	if e != nil || !queued.Released || queued.Reason != "invalid_model" {
		t.Fatalf("definite rejection %v %v", queued, e)
	}
	other, e := s.Login(ctx, store.Identity{Source: "test", Subject: "other", DisplayName: "Other"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := s.SessionUser(ctx, other)
	if _, e = s.GetOccurrence(ctx, tid, v.ID, queued.ID); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("foreign get")
	}
	if _, e = s.FindManualOccurrence(ctx, tid, v.ID, config.ID, "query-unsent"); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("foreign lookup")
	}
	foreign, e := s.RecoverableOccurrences(ctx, tid, v.ID)
	if e != nil || len(foreign) != 0 {
		t.Fatalf("foreign recovery %v %v", foreign, e)
	}
	if _, e = s.ApplyOccurrenceEvidence(ctx, tid, v.ID, cancel.ID, cancel.Version, store.ExecutionEvidence{Event: "recover"}); !errors.Is(e, store.ErrNotFound) {
		t.Fatal("foreign transition")
	}
}
