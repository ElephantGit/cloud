//go:build execution_integration && linux

package scheduler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// Opt-in tests require the public runtime/M3 packages. No test-defined Service or
// persistence replaces them. Child processes share only PostgreSQL and retained files.
func TestScheduledModelRuntimeRestart(t *testing.T) { scheduledRuntimeRestart(t, "model") }
func TestScheduledAgentRuntimeRestart(t *testing.T) {
	if os.Getenv("V2_NODE_ASSETS_DIR") == "" {
		t.Fatal("Agent acceptance requires prepared V2_NODE_ASSETS_DIR")
	}
	scheduledRuntimeRestart(t, "agent")
}

func scheduledRuntimeRestart(t *testing.T, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("runtime acceptance requires TEST_DATABASE_URL")
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := fmt.Sprintf("scheduler_runtime_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "s785-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root) // Only this test's newly created directory, after children exit.
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"execute", "read"} {
		cmd := exec.CommandContext(ctx, bin, "-test.run=^TestScheduledRuntimeProcess$", "-test.v")
		cmd.Env = append(os.Environ(), "SCHEDULE_RUNTIME_PHASE="+phase, "SCHEDULE_RUNTIME_MODE="+mode, "SCHEDULE_RUNTIME_SCHEMA="+schema, "SCHEDULE_RUNTIME_ROOT="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s/%s process: %v\n%s", mode, phase, err, out)
		}
		t.Logf("%s/%s process: %s", mode, phase, out)
	}
}

type runtimeRecord struct {
	Occurrence store.Occurrence
	Turn       execution.Turn
	Session    execution.Session
}
type runtimeState struct {
	Scope   execution.Scope
	Records []runtimeRecord
	Markers []string
}

// Compare the persisted wire values, not time.Location pointers lost by JSON transport.
func sameRuntimeValue(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	var x, y any
	_ = json.Unmarshal(left, &x)
	_ = json.Unmarshal(right, &y)
	return reflect.DeepEqual(x, y)
}

// gateFixture only pauses a real provider boundary for cancellation; normal responses
// are the shared credential-free fixture, including the Agent's real shell-tool round.
type gateFixture struct {
	mu      sync.Mutex
	block   bool
	started chan struct{}
}

func (g *gateFixture) Complete(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
	g.mu.Lock()
	block := g.block
	g.mu.Unlock()
	if block {
		select {
		case g.started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
		return
	}
	nodeservices.FixtureProvider{}.Complete(w, r, model)
}

type observeCancellation struct {
	execution.Service
	t     *testing.T
	agent bool
}

func (o *observeCancellation) Cancel(ctx context.Context, scope execution.Scope, ref execution.Ref) (execution.Result, error) {
	r, err := o.Service.Cancel(ctx, scope, ref)
	if err != nil {
		return r, err
	}
	s, err := o.Service.ReadSession(ctx, scope, ref.SessionID)
	if err != nil || (s.EndedAt != nil) != o.agent {
		o.t.Fatalf("Cancel session semantics: %+v %v", s, err)
	}
	return r, nil
}

func TestScheduledRuntimeProcess(t *testing.T) {
	phase := os.Getenv("SCHEDULE_RUNTIME_PHASE")
	if phase == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	root, mode := os.Getenv("SCHEDULE_RUNTIME_ROOT"), os.Getenv("SCHEDULE_RUNTIME_MODE")
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = os.Getenv("SCHEDULE_RUNTIME_SCHEMA") + ",public"
	db, err := store.OpenConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var saved runtimeState
	statePath := filepath.Join(root, "expected.json")
	if phase == "read" {
		raw, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
	} else {
		token, err := db.Login(ctx, store.Identity{Source: "test", Subject: "runtime-schedule", DisplayName: "Owner"}, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		u, err := db.SessionUser(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		tenants, err := db.Tenants(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		saved.Scope = execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
	}
	models, err := executionruntime.ModelsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	gate := &gateFixture{started: make(chan struct{}, 1)}
	choice := models["fixture"]
	choice.Binding.Provider = gate
	models["fixture"] = choice
	var driver *agentruntime.Driver
	projectID := ""
	if mode == "agent" {
		repos, err := projectrepos.New(db, projectrepos.Options{Root: filepath.Join(root, "r")})
		if err != nil {
			t.Fatal(err)
		}
		project, err := repos.SeedDemo(ctx, saved.Scope)
		if err != nil {
			t.Fatal(err)
		}
		projectID = project.ID
		gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
		if err != nil {
			t.Fatal(err)
		}
		driver, err = agentruntime.New(agentruntime.Options{Root: filepath.Join(root, "a"), AssetsDirectory: os.Getenv("V2_NODE_ASSETS_DIR"), GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"), Bindings: map[string]nodeservices.Binding{"fixture": choice.Binding}, ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
			p, err := db.GetProject(ctx, scope, id)
			return agentruntime.Project{MirrorPath: p.MirrorPath, Commit: p.Commit, SnapshotRef: p.SnapshotRef}, err
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
	svc, err := executionruntime.New(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := svc.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	var clock atomic.Int64
	now := time.Now().UTC().Add(time.Minute)
	if phase == "read" {
		now = now.Add(48 * time.Hour)
	}
	observed := &observeCancellation{Service: svc, t: t, agent: mode == "agent"}
	var cfg store.ScheduleConfig
	if phase == "execute" {
		target := json.RawMessage(`{"kind":"chat","model":"fixture","prompt":"hello fixture"}`)
		if mode == "agent" {
			target = json.RawMessage(`{"kind":"chat","executorId":"opencode:fixture","prompt":"Read README and write the requested fixture marker using the shell tool."}`)
		}
		cfg, err = db.CreateScheduleConfig(ctx, saved.Scope.TenantID, saved.Scope.UserID, store.ScheduleInput{Name: "runtime", ProjectID: projectID, Target: target, Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
		if err != nil {
			t.Fatal(err)
		}
		now = cfg.CreatedAt.Add(time.Minute)
	}
	clock.Store(now.UnixNano())
	options := server.Options{Execution: observed, Catalog: svc}
	c, err := server.NewScheduleCoordinator(db, options, func() time.Time { return time.Unix(0, clock.Load()) })
	if err != nil {
		t.Fatal(err)
	}
	host, err := scheduler.NewHost(db, c, svc)
	if err != nil {
		t.Fatal(err)
	}
	hostCtx, stopHost := context.WithCancel(ctx)
	ticks := time.NewTicker(10 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- host.Run(hostCtx, ticks.C) }()
	defer func() {
		stopHost()
		ticks.Stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-host.Ready():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	options.Schedules = host
	httpServer := httptest.NewServer(server.New(db, options))
	defer httpServer.Close()
	token, err := db.Login(ctx, store.Identity{Source: "test", Subject: "runtime-schedule", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	action := func(path string, input any) string {
		t.Helper()
		body, _ := json.Marshal(input)
		for {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpServer.URL+path, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
			res, err := httpServer.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				ID   string `json:"id"`
				Code string `json:"code"`
			}
			err = json.NewDecoder(res.Body).Decode(&out)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			// A scan may own this exact occurrence. Retry only the declared transient
			// conflict, preserving the request key/URL; never create a replacement run.
			if res.StatusCode == 409 && out.Code == "operation_in_progress" {
				timer := time.NewTimer(10 * time.Millisecond)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					t.Fatal(ctx.Err())
				}
				continue
			}
			if res.StatusCode != 200 || out.ID == "" {
				t.Fatalf("HTTP action %d %+v", res.StatusCode, out)
			}
			return out.ID
		}
	}
	waitFor := func(check func() bool) {
		t.Helper()
		poll := time.NewTicker(10 * time.Millisecond)
		defer poll.Stop()
		for {
			if check() {
				return
			}
			select {
			case <-poll.C:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	if phase == "execute" {
		clock.Store(cfg.CreatedAt.Add(time.Hour).UnixNano())
		waitFor(func() bool {
			items, err := db.Occurrences(ctx, saved.Scope.TenantID, saved.Scope.UserID, cfg.ID, store.OccurrenceCursor{}, 100)
			return err == nil && len(items) == 1 && items[0].ExecutionRef != ""
		})
		items, err := db.Occurrences(ctx, saved.Scope.TenantID, saved.Scope.UserID, cfg.ID, store.OccurrenceCursor{}, 100)
		if err != nil || len(items) != 1 || items[0].Kind != "scheduled" {
			t.Fatalf("scheduled trigger: %+v %v", items, err)
		}
		o := items[0]
		var ref execution.Ref
		if err = json.Unmarshal([]byte(o.ExecutionRef), &ref); err != nil {
			t.Fatal(err)
		}
		stream, err := svc.Subscribe(ctx, saved.Scope, ref.SessionID, 0)
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
					t.Fatalf("incomplete actual result: %+v", event.Result)
				}
				if mode == "agent" && event.Result.Answer != nodeservices.FixtureAnswer {
					t.Fatal("Agent did not complete fixture tool round")
				}
				break
			}
		}
		waitFor(func() bool {
			item, err := db.GetOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, o.ID)
			return err == nil && item.Released
		})
		first, err := db.GetOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, o.ID)
		if err != nil || first.Status != "succeeded" || !first.Released {
			t.Fatalf("End did not release: %+v %v", first, err)
		}
		gate.mu.Lock()
		gate.block = true
		gate.mu.Unlock()
		base := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s", saved.Scope.TenantID, cfg.ID)
		secondID := action(base+"/run", map[string]string{"requestId": "cancel-second"})
		if retry := action(base+"/run", map[string]string{"requestId": "cancel-second"}); retry != secondID {
			t.Fatal("HTTP retry created another run")
		}
		second, err := db.GetOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, secondID)
		if err != nil {
			t.Fatal(err)
		}
		if second.ExecutionRef == first.ExecutionRef || second.ID == first.ID {
			t.Fatal("new trigger reused session")
		}
		select {
		case <-gate.started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if id := action(base+"/runs/"+second.ID+"/cancel", map[string]any{}); id != second.ID {
			t.Fatal("wrong cancelled record")
		}
		second, err = db.GetOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, second.ID)
		if err != nil || second.Status != "cancelled" || !second.Released {
			t.Fatalf("cancel cleanup: %+v %v", second, err)
		}
		for _, item := range []store.Occurrence{first, second} {
			caller := execution.Caller{Kind: execution.CallerSchedule, ID: item.ID, AttemptID: item.ID}
			turn, err := svc.ReadAttempt(ctx, saved.Scope, caller)
			if err != nil {
				t.Fatal(err)
			}
			session, err := svc.ReadSession(ctx, saved.Scope, turn.Ref.SessionID)
			if err != nil || session.EndedAt == nil {
				t.Fatalf("session still live: %+v %v", session, err)
			}
			saved.Records = append(saved.Records, runtimeRecord{Occurrence: item, Turn: turn, Session: session})
		}
		if mode == "agent" {
			if saved.Records[0].Session.WorkspaceID == "" || saved.Records[0].Session.WorkspaceID == saved.Records[1].Session.WorkspaceID {
				t.Fatal("triggers share directory identity")
			}
			err = filepath.WalkDir(filepath.Join(root, "a"), func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() && d.Name() == ".git" {
					return filepath.SkipDir
				}
				if !d.IsDir() && d.Name() == "ora-v2-result.txt" {
					saved.Markers = append(saved.Markers, path)
				}
				return nil
			})
			if err != nil || len(saved.Markers) != 1 {
				t.Fatalf("tool marker after End: %v %v", saved.Markers, err)
			}
		}
		raw, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(statePath, raw, 0600); err != nil {
			t.Fatal(err)
		}
	} else {
		for _, expected := range saved.Records {
			o, err := db.GetOccurrence(ctx, saved.Scope.TenantID, saved.Scope.UserID, expected.Occurrence.ID)
			if err != nil || !sameRuntimeValue(o, expected.Occurrence) {
				t.Fatalf("occurrence changed on restart: %+v %v", o, err)
			}
			turn, err := svc.ReadAttempt(ctx, saved.Scope, expected.Turn.Caller)
			if err != nil || !sameRuntimeValue(turn, expected.Turn) {
				t.Fatalf("persistent result changed: %+v %v", turn, err)
			}
			session, err := svc.ReadSession(ctx, saved.Scope, expected.Session.ID)
			if err != nil || !sameRuntimeValue(session, expected.Session) {
				t.Fatalf("session changed: %+v %v", session, err)
			}
			caller := turn.Caller
			caller.AttemptID = "after-restart"
			if _, err = svc.Append(ctx, saved.Scope, execution.AppendRequest{SessionID: session.ID, Caller: caller, Prompt: execution.TextPrompt("must not resume")}); !errors.Is(err, execution.ErrSessionEnded) {
				t.Fatal(err)
			}
		}
		items, err := db.Occurrences(ctx, saved.Scope.TenantID, saved.Scope.UserID, saved.Records[0].Occurrence.ScheduleID, store.OccurrenceCursor{}, 100)
		if err != nil || len(items) != 2 {
			t.Fatalf("restart backfilled triggers: %+v %v", items, err)
		}
	}
	for _, path := range saved.Markers {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != nodeservices.FixtureMarker {
			t.Fatalf("End/restart removed tool output: %q %v", data, err)
		}
	}
	t.Logf("%s: %d durable occurrences, complete result + cancelled result, sessions ended, %d retained tool markers", phase, len(saved.Records), len(saved.Markers))
}
