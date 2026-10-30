//go:build execution_integration && linux

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// This opt-in test needs the public executionruntime/M3 implementation. It does not
// replace it with a local provider or storage adapter. Two child processes share only
// PostgreSQL and the expected HTTP response; no Service/Stub survives between them.
func TestScheduleResultRuntimeRestart(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("execution integration requires TEST_DATABASE_URL")
	}
	ctx := t.Context()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("result_restart_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "expected.json")
	for _, phase := range []string{"execute", "read"} {
		cmd := exec.CommandContext(ctx, bin, "-test.run=^TestScheduleResultRuntimeProcess$", "-test.v")
		cmd.Env = append(os.Environ(), "SCHEDULE_RESULT_PHASE="+phase, "SCHEDULE_RESULT_SCHEMA="+schema, "SCHEDULE_RESULT_STATE="+state)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s process: %v\n%s", phase, err, out)
		}
		t.Logf("%s process: %s", phase, out)
	}
}

func TestScheduleResultRuntimeProcess(t *testing.T) {
	phase := os.Getenv("SCHEDULE_RESULT_PHASE")
	if phase == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = os.Getenv("SCHEDULE_RESULT_SCHEMA") + ",public"
	db, err := store.OpenConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	models, err := executionruntime.ModelsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	service, err := executionruntime.New(ctx, executionruntime.Config{Store: db, Models: models})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	var saved struct {
		Token, Path, ScheduleID, OccurrenceID string
		Scope                                 execution.Scope
		Expected                              json.RawMessage
	}
	statePath := os.Getenv("SCHEDULE_RESULT_STATE")
	if phase == "execute" {
		saved.Token, err = db.Login(ctx, store.Identity{Source: "test", Subject: "runtime-result", DisplayName: "Owner"}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		user, err := db.SessionUser(ctx, saved.Token)
		if err != nil {
			t.Fatal(err)
		}
		tenants, err := db.Tenants(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		saved.Scope = execution.Scope{TenantID: tenants[0].ID, UserID: user.ID}
		cfg, err := db.CreateScheduleConfig(ctx, saved.Scope.TenantID, user.ID, store.ScheduleInput{Name: "runtime-result", Target: []byte(`{"kind":"chat","prompt":"hello fixture","model":"fixture"}`), Rule: []byte(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
		if err != nil {
			t.Fatal(err)
		}
		at := cfg.CreatedAt.Add(time.Hour)
		o, err := db.RecordOccurrence(ctx, saved.Scope.TenantID, user.ID, cfg.ID, store.Trigger{Kind: "manual", Key: "one", At: at, Version: 1}, at)
		if err != nil {
			t.Fatal(err)
		}
		o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, user.ID, o.ID, o.Version, store.ExecutionEvidence{Event: "begin"})
		if err != nil {
			t.Fatal(err)
		}
		caller := execution.Caller{Kind: execution.CallerSchedule, ID: o.ID, AttemptID: o.ID}
		ref, err := service.StartModel(ctx, saved.Scope, execution.ModelStartRequest{Caller: caller, ModelID: "fixture", Prompt: execution.TextPrompt("hello fixture")})
		if err != nil {
			t.Fatal(err)
		}
		rawRef, _ := json.Marshal(ref)
		o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, user.ID, o.ID, o.Version, store.ExecutionEvidence{Event: "accepted", Ref: string(rawRef)})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := service.Subscribe(ctx, saved.Scope, ref.SessionID, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		for {
			event, err := stream.Next(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if event.Ref == ref && event.Kind == execution.EventTurnEnded {
				if event.Result == nil || event.Result.Status != execution.StatusCompleted || !event.Result.AnswerComplete || event.Result.Answer == "" {
					t.Fatalf("fixture result: %+v", event.Result)
				}
				break
			}
		}
		if err = service.End(ctx, saved.Scope, ref.SessionID); err != nil {
			t.Fatal(err)
		}
		for _, event := range []string{"completed", "released"} {
			o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, user.ID, o.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: string(rawRef)})
			if err != nil {
				t.Fatal(err)
			}
		}
		saved.ScheduleID, saved.OccurrenceID = cfg.ID, o.ID
		saved.Path = fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs/%s/result", saved.Scope.TenantID, cfg.ID, o.ID)
	} else {
		raw, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
	}
	before, err := db.ScheduleHistoryItem(ctx, saved.Scope.TenantID, saved.Scope.UserID, saved.ScheduleID, saved.OccurrenceID)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.New(db, server.Options{Execution: service}))
	defer srv.Close()
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+saved.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: saved.Token})
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("HTTP %d %s: %v", response.StatusCode, raw, err)
	}
	var got struct {
		SessionID, TurnID string
		SessionEndedAt    *time.Time
		Result            *struct {
			Status, Answer string
			AnswerComplete bool
		}
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Result == nil || got.Result.Status != "completed" || !got.Result.AnswerComplete || got.Result.Answer == "" || got.SessionEndedAt == nil {
		t.Fatalf("incomplete HTTP result: %s", raw)
	}
	turn, err := service.ReadAttempt(ctx, saved.Scope, execution.Caller{Kind: execution.CallerSchedule, ID: saved.OccurrenceID, AttemptID: saved.OccurrenceID})
	if err != nil || turn.Ref.SessionID != got.SessionID || turn.Ref.TurnID != got.TurnID || turn.Result.Answer != got.Result.Answer {
		t.Fatalf("HTTP/public mismatch: %+v %v", turn, err)
	}
	if phase == "execute" {
		saved.Expected = raw
		data, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(statePath, data, 0600); err != nil {
			t.Fatal(err)
		}
	} else {
		var expected, actual any
		if err = json.Unmarshal(saved.Expected, &expected); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("HTTP result changed across process restart: %s", raw)
		}
	}
	after, err := db.ScheduleHistoryItem(ctx, saved.Scope.TenantID, saved.Scope.UserID, saved.ScheduleID, saved.OccurrenceID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("HTTP read changed occurrence")
	}
}
