package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type executionFixture struct {
	s      *store.Store
	config *pgxpool.Config
	admin  *pgx.Conn
	scope  execution.Scope
}

func executionDatabase(t *testing.T) *executionFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	schema := "execution_" + hex.EncodeToString(id[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	f := &executionFixture{admin: admin}
	t.Cleanup(func() {
		if f.s != nil {
			f.s.Close()
		}
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(ctx)
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	f.config = config
	if _, err := admin.Exec(ctx, "SET search_path TO "+pgx.Identifier{schema}.Sanitize()+",public"); err != nil {
		t.Fatal(err)
	}
	f.s, err = store.OpenConfig(ctx, config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	f.scope = executionMember(t, f.s, "owner")
	return f
}

func executionMember(t *testing.T, s *store.Store, subject string) execution.Scope {
	t.Helper()
	ctx := context.Background()
	token, err := s.Login(ctx, store.Identity{Source: "test", Subject: subject, DisplayName: subject}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := s.Tenants(ctx, u.ID)
	if err != nil || len(tenants) != 1 {
		t.Fatalf("tenants=%+v err=%v", tenants, err)
	}
	return execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
}

func executionTurn(sid, tid string) execution.Turn {
	return execution.Turn{Ref: execution.Ref{SessionID: sid, TurnID: tid}, Caller: execution.Caller{Kind: execution.CallerChat, ID: "conversation", AttemptID: tid}, Prompt: execution.TextPrompt("问题🙂 " + tid)}
}

func createExecution(t *testing.T, f *executionFixture, sid string, turn execution.Turn) {
	t.Helper()
	err := f.s.CreateExecutionSession(t.Context(), f.scope, execution.Session{ID: sid, Model: execution.Model{ID: "fixture", Name: "Fixture", BindingVersion: "v1"}}, turn)
	if err != nil {
		t.Fatal(err)
	}
}

func finishExecution(t *testing.T, f *executionFixture, turn execution.Turn, answer string) execution.Result {
	t.Helper()
	result, err := f.s.FinishExecutionTurn(t.Context(), f.scope, turn.Ref, execution.Result{Status: execution.StatusCompleted, Answer: answer, AnswerComplete: true, StopReason: "stop"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestExecutionReopenRecoveryAndPagination(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	first := executionTurn("ended", "first")
	createExecution(t, f, "ended", first)
	if cursor, err := f.s.ReadExecutionCursor(ctx, f.scope, "ended"); err != nil || cursor != 0 {
		t.Fatalf("initial cursor=%d err=%v", cursor, err)
	}
	if err := f.s.EndExecutionSession(ctx, f.scope, "ended"); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("ended active execution: %v", err)
	}
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: first.Ref, Kind: execution.EventTextDelta, TextDelta: "答案🙂"}); err != nil {
		t.Fatal(err)
	}
	want := finishExecution(t, f, first, "答案🙂")
	second := executionTurn("ended", "second")
	if err := f.s.AppendExecutionTurn(ctx, f.scope, "ended", second); err != nil {
		t.Fatal(err)
	}
	finishExecution(t, f, second, "第二轮")
	for range 2 {
		if err := f.s.EndExecutionSession(ctx, f.scope, "ended"); err != nil {
			t.Fatal(err)
		}
	}
	live := executionTurn("live", "interrupted")
	createExecution(t, f, "live", live)
	for _, delta := range []string{"部分", "🙂"} {
		if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: live.Ref, Kind: execution.EventTextDelta, TextDelta: delta}); err != nil {
			t.Fatal(err)
		}
	}
	completed := executionTurn("completed", "kept")
	createExecution(t, f, "completed", completed)
	kept := finishExecution(t, f, completed, "keep completed")
	f.s.Close()
	var err error
	f.s, err = store.OpenConfig(ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.s.RecoverExecutions(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, sid := range []string{"ended", "live", "completed"} {
		session, err := f.s.ReadExecutionSession(ctx, f.scope, sid)
		if err != nil || session.EndedAt == nil || session.CreatedAt.IsZero() || session.Model.BindingVersion != "v1" {
			t.Fatalf("session=%+v err=%v", session, err)
		}
		if err := f.s.AppendExecutionTurn(ctx, f.scope, sid, executionTurn(sid, "late")); !errors.Is(err, execution.ErrSessionEnded) {
			t.Fatalf("append after end=%v", err)
		}
	}
	for _, tc := range []struct {
		caller execution.Caller
		want   execution.Result
	}{
		{first.Caller, want}, {completed.Caller, kept},
		{live.Caller, execution.Result{Status: execution.StatusInterrupted, Answer: "部分🙂", FailureCode: "server_restarted"}},
	} {
		turn, err := f.s.ReadExecutionAttempt(ctx, f.scope, tc.caller)
		if err != nil || turn.Result == nil || !reflect.DeepEqual(*turn.Result, tc.want) || turn.EndedAt == nil || turn.StartedAt.IsZero() {
			t.Fatalf("turn=%+v want=%+v err=%v", turn, tc.want, err)
		}
	}
	page, err := f.s.ReadExecutionTurns(ctx, f.scope, "ended", "", 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Ref != first.Ref || page.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", page, err)
	}
	last, err := f.s.ReadExecutionTurns(ctx, f.scope, "ended", page.NextCursor, 1)
	if err != nil || len(last.Items) != 1 || last.Items[0].Ref != second.Ref || last.NextCursor != "" {
		t.Fatalf("last page=%+v err=%v", last, err)
	}
	if _, err := f.s.ReadExecutionTurns(ctx, f.scope, "live", page.NextCursor, 1); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("cross-session cursor=%v", err)
	}
	for _, tc := range []struct {
		sid   string
		count int
	}{{"ended", 4}, {"live", 4}, {"completed", 2}} {
		events, err := f.s.ReadExecutionEvents(ctx, f.scope, tc.sid, 0, 100)
		if err != nil || len(events) != tc.count || events[len(events)-1].Kind != execution.EventSessionEnded {
			t.Fatalf("events=%+v err=%v", events, err)
		}
		if cursor, err := f.s.ReadExecutionCursor(ctx, f.scope, tc.sid); err != nil || cursor != uint64(tc.count) {
			t.Fatalf("reopened cursor=%d err=%v", cursor, err)
		}
		for i, event := range events {
			if event.Sequence != uint64(i+1) || event.OccurredAt.IsZero() {
				t.Fatalf("event sequence/time=%+v", event)
			}
		}
		if _, err := f.s.ReadExecutionEvents(ctx, f.scope, tc.sid, uint64(tc.count+1), 100); !errors.Is(err, execution.ErrInvalidRequest) {
			t.Fatalf("ahead cursor=%v", err)
		}
	}
}

func TestExecutionCallerIdentityAndScope(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	turn := executionTurn("original", "attempt")
	turn.Caller = execution.Caller{Kind: execution.CallerWorkflow, ID: "run", NodeID: "node", AttemptID: "attempt", Rounds: []execution.Round{{ContainerID: "outer", Index: 1}, {ContainerID: "inner", Index: 2}}}
	createExecution(t, f, "original", turn)
	duplicate := turn
	duplicate.Ref = execution.Ref{SessionID: "orphan", TurnID: "duplicate"}
	if err := f.s.CreateExecutionSession(ctx, f.scope, execution.Session{ID: "orphan"}, duplicate); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	if _, err := f.s.ReadExecutionSession(ctx, f.scope, "orphan"); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("failed admission left a session: %v", err)
	}
	for i := range 3 {
		changed := turn
		changed.Ref = execution.Ref{SessionID: fmt.Sprintf("changed-%d", i), TurnID: fmt.Sprintf("turn-%d", i)}
		changed.Caller.Rounds = append([]execution.Round(nil), turn.Caller.Rounds...)
		switch i {
		case 0:
			changed.Caller.NodeID = "sibling"
		case 1:
			changed.Caller.Rounds[0].Index++
		case 2:
			changed.Caller.Rounds[0], changed.Caller.Rounds[1] = changed.Caller.Rounds[1], changed.Caller.Rounds[0]
		}
		createExecution(t, f, changed.Ref.SessionID, changed)
		got, err := f.s.ReadExecutionAttempt(ctx, f.scope, changed.Caller)
		if err != nil || got.Ref != changed.Ref {
			t.Fatalf("full caller identity=%+v %v", got, err)
		}
	}
	plain := executionTurn("plain", "empty-rounds")
	createExecution(t, f, "plain", plain)
	plain.Caller.Rounds = []execution.Round{}
	if got, err := f.s.ReadExecutionAttempt(ctx, f.scope, plain.Caller); err != nil || got.Ref != plain.Ref {
		t.Fatalf("empty rounds=%+v %v", got, err)
	}
	other := executionMember(t, f.s, "other")
	checks := []func() error{
		func() error { _, err := f.s.ReadExecutionCursor(ctx, other, "original"); return err },
		func() error { _, err := f.s.ReadExecutionSession(ctx, other, "original"); return err },
		func() error { _, err := f.s.ReadExecutionTurns(ctx, other, "original", "", 10); return err },
		func() error { _, err := f.s.ReadExecutionAttempt(ctx, other, turn.Caller); return err },
		func() error { _, err := f.s.ReadExecutionEvents(ctx, other, "original", 0, 10); return err },
		func() error {
			return f.s.AppendExecutionTurn(ctx, other, "original", executionTurn("original", "forged"))
		},
		func() error {
			_, err := f.s.FinishExecutionTurn(ctx, other, turn.Ref, execution.Result{Status: execution.StatusCancelled})
			return err
		},
		func() error { return f.s.EndExecutionSession(ctx, other, "original") },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, execution.ErrNotFound) {
			t.Fatalf("scope check %d=%v", i, err)
		}
	}
	otherTurn := turn
	otherTurn.Ref = execution.Ref{SessionID: "other", TurnID: "other-turn"}
	if err := f.s.CreateExecutionSession(ctx, other, execution.Session{ID: "other"}, otherTurn); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE users SET status='inactive' WHERE id=$1`, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CheckExecutionScope(ctx, f.scope); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("inactive user=%v", err)
	}
}

func TestExecutionConcurrentEventsAndTerminalResult(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	turn := executionTurn("concurrent", "turn")
	createExecution(t, f, "concurrent", turn)
	const count = 12
	start := make(chan struct{})
	errorsCh := make(chan error, count)
	for i := range count {
		go func() {
			<-start
			_, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: turn.Ref, Kind: execution.EventTextDelta, TextDelta: fmt.Sprint(i)})
			errorsCh <- err
		}()
	}
	close(start)
	for range count {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	type finish struct {
		result execution.Result
		err    error
	}
	results := make(chan finish, 2)
	for _, answer := range []string{"winner A", "winner B"} {
		go func() {
			result, err := f.s.FinishExecutionTurn(ctx, f.scope, turn.Ref, execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: answer, StopReason: "stop"})
			results <- finish{result, err}
		}()
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || !reflect.DeepEqual(a.result, b.result) {
		t.Fatalf("terminal overwrite: %+v %+v", a, b)
	}
	events, err := f.s.ReadExecutionEvents(ctx, f.scope, "concurrent", 0, 100)
	if err != nil || len(events) != count+1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("sequence=%d at %d", event.Sequence, i)
		}
	}
	if events[count].Kind != execution.EventTurnEnded || !reflect.DeepEqual(*events[count].Result, a.result) {
		t.Fatalf("terminal event=%+v", events[count])
	}
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: turn.Ref, Kind: execution.EventTextDelta, TextDelta: "late"}); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("late delta=%v", err)
	}
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: turn.Ref, Kind: execution.EventTurnEnded, Result: &a.result}); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("non-atomic terminal event=%v", err)
	}
	// JSON fields retain Unicode and explicit usage without a separate lossy projection.
	usageTurn := executionTurn("usage", "usage-turn")
	createExecution(t, f, "usage", usageTurn)
	usage := execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "🙂", StopReason: "stop", Usage: json.RawMessage(`{"inputTokens":2,"outputTokens":3,"totalTokens":5}`)}
	if _, err := f.s.FinishExecutionTurn(ctx, f.scope, usageTurn.Ref, usage); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.ReadExecutionAttempt(ctx, f.scope, usageTurn.Caller)
	var counters map[string]int
	if err != nil || got.Result == nil || json.Unmarshal(got.Result.Usage, &counters) != nil || counters["totalTokens"] != 5 || got.Result.Answer != "🙂" {
		t.Fatalf("usage round trip=%+v err=%v", got, err)
	}
}

func TestExecutionAppendDistinguishesBusyFromCallerConflict(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	first := executionTurn("busy", "first")
	createExecution(t, f, "busy", first)
	second := executionTurn("busy", "second")
	if err := f.s.AppendExecutionTurn(ctx, f.scope, "busy", second); !errors.Is(err, execution.ErrWorkspaceBusy) {
		t.Fatalf("append during active turn=%v", err)
	}
	finishExecution(t, f, first, "done")
	duplicate := second
	duplicate.Caller = first.Caller
	if err := f.s.AppendExecutionTurn(ctx, f.scope, "busy", duplicate); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("caller identity conflict=%v", err)
	}
	if err := f.s.AppendExecutionTurn(ctx, f.scope, "busy", second); err != nil {
		t.Fatalf("append after first turn ended=%v", err)
	}
	page, err := f.s.ReadExecutionTurns(ctx, f.scope, "busy", "", 10)
	if err != nil || len(page.Items) != 2 || page.Items[0].Ref != first.Ref || page.Items[1].Ref != second.Ref {
		t.Fatalf("failed admissions left turns: %+v err=%v", page, err)
	}
}

func TestExecutionEventsReadWhileWriterOwnsSessionLock(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	turn := executionTurn("read", "turn")
	createExecution(t, f, "read", turn)
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: turn.Ref, Kind: execution.EventTextDelta, TextDelta: "committed"}); err != nil {
		t.Fatal(err)
	}
	tx, err := f.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	// Hold a real writer lock and an uncommitted counter change throughout the read.
	// The timeout only bounds a regression that would wait for this lock forever.
	if _, err := tx.Exec(ctx, `UPDATE v2_execution_sessions SET event_sequence=event_sequence+1 WHERE id='read'`); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	events, err := f.s.ReadExecutionEvents(readCtx, f.scope, "read", 0, 10)
	if err != nil || len(events) != 1 || events[0].Sequence != 1 || events[0].TextDelta != "committed" {
		t.Fatalf("read waited for writer or observed uncommitted event: %+v err=%v", events, err)
	}
	if _, err := f.s.ReadExecutionEvents(readCtx, f.scope, "read", 2, 10); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("accepted uncommitted cursor: %v", err)
	}
}
