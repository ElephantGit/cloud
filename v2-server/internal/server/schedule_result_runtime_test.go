//go:build execution_integration && linux

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// This opt-in test needs the public executionruntime/M3 implementation. It does not
// replace it with a local provider or storage adapter. Two child processes share only
// PostgreSQL and the expected HTTP response; no Service/Stub survives between them.
func TestScheduleResultRuntimeRestart(t *testing.T) { scheduleResultRuntimeRestart(t, "model") }

func TestScheduleAgentResultRuntimeRestart(t *testing.T) {
	if os.Getenv("V2_NODE_ASSETS_DIR") == "" {
		t.Fatal("Agent integration requires prepared V2_NODE_ASSETS_DIR")
	}
	scheduleResultRuntimeRestart(t, "agent")
}

func scheduleResultRuntimeRestart(t *testing.T, mode string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("execution integration requires TEST_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
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
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "h793-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root) // Only this test's new directory, after both children exit.
	state := filepath.Join(root, "expected.json")
	for _, phase := range []string{"execute", "read"} {
		cmd := exec.CommandContext(ctx, bin, "-test.run=^TestScheduleResultRuntimeProcess$", "-test.v")
		cmd.Env = append(os.Environ(), "SCHEDULE_RESULT_PHASE="+phase, "SCHEDULE_RESULT_SCHEMA="+schema, "SCHEDULE_RESULT_STATE="+state, "SCHEDULE_RESULT_MODE="+mode, "SCHEDULE_RESULT_ROOT="+root)
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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
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
	var saved struct {
		Token, Path, ScheduleID, OccurrenceID, ProjectID string
		Scope                                            execution.Scope
		Expected                                         map[string]json.RawMessage
		Markers                                          []string
	}
	statePath := os.Getenv("SCHEDULE_RESULT_STATE")
	mode, root := os.Getenv("SCHEDULE_RESULT_MODE"), os.Getenv("SCHEDULE_RESULT_ROOT")
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
	} else {
		raw, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
	}
	models, err := executionruntime.ModelsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	var driver *agentruntime.Driver
	if mode == "agent" {
		if phase == "execute" {
			repos, err := projectrepos.New(db, projectrepos.Options{Root: filepath.Join(root, "r")})
			if err != nil {
				t.Fatal(err)
			}
			project, err := repos.SeedDemo(ctx, saved.Scope)
			if err != nil {
				t.Fatal(err)
			}
			saved.ProjectID = project.ID
		}
		gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
		if err != nil {
			t.Fatal(err)
		}
		driver, err = agentruntime.New(agentruntime.Options{Root: filepath.Join(root, "a"), AssetsDirectory: os.Getenv("V2_NODE_ASSETS_DIR"), GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"), Bindings: map[string]nodeservices.Binding{"fixture": models["fixture"].Binding}, ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
			project, err := db.GetProject(ctx, scope, id)
			return agentruntime.Project{MirrorPath: project.MirrorPath, Commit: project.Commit, SnapshotRef: project.SnapshotRef}, err
		}})
		if err != nil {
			t.Fatal(err)
		}
		defer driver.Close(context.Background())
	}
	runtimeConfig := executionruntime.Config{Store: db, Models: models}
	if driver != nil {
		runtimeConfig.Agent = driver
	}
	service, err := executionruntime.New(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	if phase == "execute" {
		target := json.RawMessage(`{"kind":"chat","prompt":"hello fixture","model":"fixture"}`)
		if mode == "agent" {
			target = json.RawMessage(`{"kind":"chat","prompt":"Inspect README and save the fixture marker using the shell tool.","model":"fixture","executorId":"opencode:fixture"}`)
		}
		cfg, err := db.CreateScheduleConfig(ctx, saved.Scope.TenantID, saved.Scope.UserID, store.ScheduleInput{Name: "runtime-result", ProjectID: saved.ProjectID, Target: target, Rule: []byte(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
		if err != nil {
			t.Fatal(err)
		}
		at := cfg.CreatedAt.Add(time.Hour)
		o, err := db.RecordOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, cfg.ID, store.Trigger{Kind: "manual", Key: "one", At: at, Version: 1}, at)
		if err != nil {
			t.Fatal(err)
		}
		o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, saved.Scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: "begin"})
		if err != nil {
			t.Fatal(err)
		}
		caller := execution.Caller{Kind: execution.CallerSchedule, ID: o.ID, AttemptID: o.ID}
		var ref execution.Ref
		if mode == "agent" {
			ref, err = service.Start(ctx, saved.Scope, execution.StartRequest{Caller: caller, Target: execution.Target{ProjectID: saved.ProjectID}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("Inspect README and save the fixture marker using the shell tool.")})
		} else {
			ref, err = service.StartModel(ctx, saved.Scope, execution.ModelStartRequest{Caller: caller, ModelID: "fixture", Prompt: execution.TextPrompt("hello fixture")})
		}
		if err != nil {
			t.Fatal(err)
		}
		rawRef, _ := json.Marshal(ref)
		o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, saved.Scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: "accepted", Ref: string(rawRef)})
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
			o, err = db.ApplyOccurrenceEvidence(ctx, saved.Scope.TenantID, saved.Scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: string(rawRef)})
			if err != nil {
				t.Fatal(err)
			}
		}
		saved.ScheduleID, saved.OccurrenceID = cfg.ID, o.ID
		saved.Path = fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs/%s/result", saved.Scope.TenantID, cfg.ID, o.ID)
		if mode == "agent" {
			if err := filepath.WalkDir(filepath.Join(root, "a"), func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() && entry.Name() == ".git" {
					return filepath.SkipDir
				}
				if entry.Type().IsRegular() && entry.Name() == "ora-v2-result.txt" {
					saved.Markers = append(saved.Markers, path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(saved.Markers) != 1 {
				t.Fatalf("actual tool outputs: %v", saved.Markers)
			}
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
		SessionID, TurnID, Mode string
		SessionEndedAt          *time.Time
		Result                  *struct {
			Status, Answer string
			AnswerComplete bool
		}
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != mode || got.Result == nil || got.Result.Status != "completed" || !got.Result.AnswerComplete || got.Result.Answer == "" || got.SessionEndedAt == nil {
		t.Fatalf("incomplete HTTP result: %s", raw)
	}
	turn, err := service.ReadAttempt(ctx, saved.Scope, execution.Caller{Kind: execution.CallerSchedule, ID: saved.OccurrenceID, AttemptID: saved.OccurrenceID})
	if err != nil || turn.Ref.SessionID != got.SessionID || turn.Ref.TurnID != got.TurnID || turn.Result.Answer != got.Result.Answer {
		t.Fatalf("HTTP/public mismatch: %+v %v", turn, err)
	}
	if phase == "execute" {
		saved.Expected = map[string]json.RawMessage{saved.Path: raw}

	} else {
		var expected, actual any
		if err = json.Unmarshal(saved.Expected[saved.Path], &expected); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("HTTP result changed across process restart: %s", raw)
		}
	}
	// List and item reads must also survive a fresh process without a cursor/Scope change.
	base := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs", saved.Scope.TenantID, saved.ScheduleID)
	for _, path := range []string{base, base + "/" + saved.OccurrenceID} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: saved.Token})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("history HTTP %d: %v", res.StatusCode, err)
		}
		if phase == "execute" {
			saved.Expected[path] = body
		} else {
			var expected, actual any
			if err = json.Unmarshal(saved.Expected[path], &expected); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(body, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, actual) {
				t.Fatal("history response changed after process restart")
			}
		}
	}
	if phase == "execute" {
		data, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(statePath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range saved.Markers {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != nodeservices.FixtureMarker {
			t.Fatalf("actual tool file missing or changed after End/restart: %v", err)
		}
	}
	if mode == "agent" && got.Result.Answer != nodeservices.FixtureAnswer {
		t.Fatal("Agent answer differs from actual fixture tool completion")
	}
	t.Logf("%s/%s: list, item and complete result readable; %d retained tool outputs", mode, phase, len(saved.Markers))
	after, err := db.ScheduleHistoryItem(ctx, saved.Scope.TenantID, saved.Scope.UserID, saved.ScheduleID, saved.OccurrenceID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("HTTP read changed occurrence")
	}
}
