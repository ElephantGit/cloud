package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type brokenHistory struct{ *store.Store }

func (b brokenHistory) ScheduleHistory(context.Context, string, string, string, store.OccurrenceCursor, int) ([]store.Occurrence, error) {
	return nil, errors.New("private database credentials")
}
func (b brokenHistory) ScheduleHistoryItem(context.Context, string, string, string, string) (store.Occurrence, error) {
	return store.Occurrence{}, errors.New("private database credentials")
}

func TestHistoryRevocationAndUnavailable(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("history_%d", time.Now().UnixNano())
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, e := store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	edit, e := pgx.ConnectConfig(ctx, config.ConnConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer edit.Close(ctx)
	token, e := s.Login(ctx, store.Identity{Source: "test", Subject: "owner", DisplayName: "Owner"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	user, e := s.SessionUser(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenants, e := s.Tenants(ctx, user.ID)
	if e != nil {
		t.Fatal(e)
	}
	tid := tenants[0].ID
	cfg, e := s.CreateScheduleConfig(ctx, tid, user.ID, store.ScheduleInput{Name: "test", Target: []byte(`{"kind":"chat"}`), Rule: []byte(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if e != nil {
		t.Fatal(e)
	}
	at := cfg.CreatedAt.Add(time.Hour)
	o, e := s.RecordOccurrence(ctx, tid, user.ID, cfg.ID, store.Trigger{Kind: "manual", Key: "one", Version: 1, At: at}, at)
	if e != nil {
		t.Fatal(e)
	}
	base := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs", tid, cfg.ID)
	ref := execution.Ref{SessionID: "access-session", TurnID: "access-turn"}
	encoded, _ := json.Marshal(ref)
	for _, evidence := range []store.ExecutionEvidence{{Event: "begin"}, {Event: "accepted", Ref: string(encoded)}} {
		o, e = s.ApplyOccurrenceEvidence(ctx, tid, user.ID, o.ID, o.Version, evidence)
		if e != nil {
			t.Fatal(e)
		}
	}
	check := func(backend server.Store, want int) {
		t.Helper()
		reads := 0
		stub := &executiontest.Stub{
			ReadAttemptFunc: func(_ context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
				reads++
				if scope != (execution.Scope{TenantID: tid, UserID: user.ID}) {
					t.Error("incorrect execution scope")
				}
				return execution.Turn{Ref: ref, Caller: caller, StartedAt: at}, nil
			},
			ReadSessionFunc: func(context.Context, execution.Scope, string) (execution.Session, error) {
				return execution.Session{ID: ref.SessionID}, nil
			},
		}
		srv := httptest.NewServer(server.New(backend, server.Options{Execution: stub}))
		defer srv.Close()
		for _, path := range []string{base, base + "/" + o.ID, base + "/" + o.ID + "/result"} {
			req, _ := http.NewRequest("GET", srv.URL+path, nil)
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
			res, e := http.DefaultClient.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != want || strings.Contains(string(raw), "private database") {
				t.Fatalf("%d %s", res.StatusCode, raw)
			}
		}
		if want != 200 && reads != 0 {
			t.Fatal("execution read before occurrence authorization")
		}
	}
	check(s, 200)
	check(struct{ server.Store }{s}, 503)
	check(brokenHistory{s}, 500)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := edit.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec("DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2", tid, user.ID)
	check(s, 404)
	exec("INSERT INTO memberships(tenant_id,user_id,role) VALUES($1,$2,'member')", tid, user.ID)
	exec("UPDATE tenants SET status='disabled' WHERE id=$1", tid)
	check(s, 404)
	exec("UPDATE tenants SET status='active' WHERE id=$1", tid)
	check(s, 200)
	// Reopening the actual pool preserves occurrence identity and state.
	s.Close()
	s, e = store.OpenConfig(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	check(s, 200)
	exec("UPDATE users SET status='disabled' WHERE id=$1", user.ID)
	check(s, 404)
	exec("UPDATE users SET status='active' WHERE id=$1", user.ID)
	// Read current states through HTTP twice with pool reopening.
	for _, tc := range []struct {
		status, reason string
		version        int64
	}{
		{"reconciling", "lookup_required", 10},
		{"skipped", "execution_busy", 10},
		{"skipped", "execution_not_started", 10},
	} {
		exec("UPDATE v2_schedule_occurrences SET status=$2,reason=$3,version=10 WHERE id=$1", o.ID, tc.status, tc.reason)
		for repeat := 0; repeat < 2; repeat++ {
			s.Close()
			s, e = store.OpenConfig(ctx, config)
			if e != nil {
				t.Fatal(e)
			}
			srv := httptest.NewServer(server.New(s, server.Options{}))
			req, _ := http.NewRequest("GET", srv.URL+base+"/"+o.ID, nil)
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
			res, e := http.DefaultClient.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			var got store.Occurrence
			e = json.NewDecoder(res.Body).Decode(&got)
			res.Body.Close()
			srv.Close()
			var beforeSnapshot, afterSnapshot any
			json.Unmarshal(o.Snapshot, &beforeSnapshot)
			json.Unmarshal(got.Snapshot, &afterSnapshot)
			if e != nil || res.StatusCode != 200 || got.Status != tc.status || got.Reason != tc.reason || got.Version != tc.version || got.ID != o.ID || got.ExecutionRef != o.ExecutionRef || got.Released != o.Released || !reflect.DeepEqual(beforeSnapshot, afterSnapshot) {
				t.Fatalf("reopen %d: %+v err=%v", repeat, got, e)
			}
		}
	}
	s.Close()
}
