//go:build linux

// Command v2-server is Cloud v2's temporary single-process backend; see README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"golang.org/x/sys/unix"
)

// defaultDatabase is the database v2-server uses on the repository's compose PostgreSQL.
const defaultDatabase = "postgres://ora:ora-local@127.0.0.1:55432/ora_v2?sslmode=disable"

func main() {
	addr := flag.String("addr", envOr("V2_ADDR", "127.0.0.1:8090"), "listen address")
	dbURL := flag.String("db", envOr("V2_DATABASE_URL", defaultDatabase), "PostgreSQL URL")
	cleanup := flag.Bool("cleanup", false, "stop and remove owned conversation directories; retain repository mirrors and database history")
	fixture := flag.Bool("fixture-check", false, "run the real Node fixture through the public execution service, then exit")
	flag.Parse()

	if e := run(*addr, *dbURL, *cleanup, *fixture); e != nil {
		slog.Error("v2-server", "error", e)
		os.Exit(1)
	}
}

func run(addr, dbURL string, cleanup, fixture bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	base, e := dataDirectory()
	if e != nil {
		return e
	}
	lock, e := lockDirectory(base)
	if e != nil {
		return e
	}
	defer lock.Close()
	if cleanup {
		return agentruntime.Cleanup(ctx, filepath.Join(base, "nodes"))
	}

	db, e := store.Open(ctx, dbURL)
	if e != nil {
		return e
	}
	defer db.Close()
	projects, e := projectrepos.New(db, projectrepos.Options{Root: filepath.Join(base, "repositories")})
	if e != nil {
		return e
	}
	models, e := executionruntime.ModelsFromEnvironment(os.Getenv)
	if e != nil {
		return e
	}
	var driver *agentruntime.Driver
	if assets := os.Getenv("V2_NODE_ASSETS_DIR"); assets != "" {
		gitDir, e := exec.CommandContext(ctx, "git", "--exec-path").Output()
		if e != nil {
			return errors.New("git executable unavailable")
		}
		bindings := map[string]nodeservices.Binding{}
		for id, choice := range models {
			bindings[id] = choice.Binding
		}
		driver, e = agentruntime.New(agentruntime.Options{Root: filepath.Join(base, "nodes"), AssetsDirectory: assets, GitBackend: filepath.Join(strings.TrimSpace(string(gitDir)), "git-http-backend"), Bindings: bindings, ResolveProject: func(c context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
			p, e := db.GetProject(c, scope, id)
			return agentruntime.Project{MirrorPath: p.MirrorPath, Commit: p.Commit, SnapshotRef: p.SnapshotRef}, e
		}})
		if e != nil {
			return e
		}
	} else if e := agentruntime.Recover(ctx, filepath.Join(base, "nodes")); e != nil {
		return e
	}
	var agent executionruntime.AgentDriver
	if driver != nil {
		agent = driver
	}
	runtime, e := executionruntime.New(ctx, executionruntime.Config{Store: db, Models: models, Agent: agent})
	if e != nil {
		if driver != nil {
			_ = driver.Close(context.Background())
		}
		return e
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runtime.Close(shutdown); err != nil {
			slog.Error("execution shutdown unconfirmed", "error", err)
		}
	}()
	if fixture {
		if driver == nil {
			return errors.New("fixture-check requires V2_NODE_ASSETS_DIR; run task v2:node:prepare first")
		}
		return checkFixture(ctx, db, projects, runtime, filepath.Join(base, "nodes"))
	}

	srv := &http.Server{
		Addr: addr,
		Handler: server.New(db, server.Options{
			SessionTTL: 7 * 24 * time.Hour,
			DemoUser:   store.Identity{Source: "dev", Subject: "demo", DisplayName: "演示用户"},
			Projects:   projects,
			Execution:  runtime,
			Catalog:    runtime,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	slog.Info("v2-server listening", "addr", addr)

	select {
	case e := <-errs:
		return e
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := srv.Shutdown(shutdown); e != nil {
		return e
	}
	if e := <-errs; !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return nil
}

func dataDirectory() (string, error) {
	root := os.Getenv("V2_DATA_DIR")
	if root == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		root = filepath.Join(home, ".ora-v2")
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	return root, os.MkdirAll(root, 0700)
}

// One v2 process owns the data directory; cleanup cannot race an active server.
func lockDirectory(root string) (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(root, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("v2 data directory is already in use")
	}
	return f, nil
}

func checkFixture(ctx context.Context, db *store.Store, projects *projectrepos.Service, runtime *executionruntime.Service, nodeRoot string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	before, e := fixtureMarkers(nodeRoot)
	if e != nil {
		return e
	}
	token, e := db.Login(ctx, store.Identity{Source: "dev", Subject: "demo", DisplayName: "演示用户"}, time.Hour)
	if e != nil {
		return e
	}
	defer db.Logout(context.Background(), token)
	user, e := db.SessionUser(ctx, token)
	if e != nil {
		return e
	}
	tenants, e := db.Tenants(ctx, user.ID)
	if e != nil {
		return e
	}
	if len(tenants) == 0 {
		return errors.New("demo tenant unavailable")
	}
	scope := execution.Scope{TenantID: tenants[0].ID, UserID: user.ID}
	p, e := projects.SeedDemo(ctx, scope)
	if e != nil {
		return e
	}
	started := time.Now()
	caller := execution.Caller{Kind: execution.CallerChat, ID: fmt.Sprintf("fixture-%d", started.UnixNano()), AttemptID: "first"}
	ref, e := runtime.Start(ctx, scope, execution.StartRequest{Caller: caller, Target: execution.Target{ProjectID: p.ID}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("Inspect README and save the fixture result.")})
	if e != nil {
		return e
	}
	stream, e := runtime.Subscribe(ctx, scope, ref.SessionID, 0)
	if e != nil {
		return e
	}
	defer stream.Close()
	for {
		event, e := stream.Next(ctx)
		if e != nil {
			return e
		}
		if event.Kind == execution.EventTurnEnded && event.Ref == ref {
			if event.Result == nil || event.Result.Status != execution.StatusCompleted || !event.Result.AnswerComplete || event.Result.Answer != nodeservices.FixtureAnswer {
				return errors.New("fixture did not complete with the real shell-tool answer")
			}
			break
		}
	}
	if e := runtime.End(ctx, scope, ref.SessionID); e != nil {
		return e
	}
	turn, e := runtime.ReadAttempt(ctx, scope, caller)
	if e != nil {
		return e
	}
	if turn.Result == nil || turn.Result.Answer != nodeservices.FixtureAnswer {
		return errors.New("fixture result unavailable after End")
	}
	after, e := fixtureMarkers(nodeRoot)
	if e != nil {
		return e
	}
	verified := false
	for path, value := range after {
		if _, existed := before[path]; !existed && value == nodeservices.FixtureMarker {
			verified = true
		}
	}
	if !verified {
		return errors.New("fixture shell tool did not produce a new verified checkout marker")
	}
	slog.Info("real Node fixture passed through execution.Service", "session", ref.SessionID, "turn", ref.TurnID, "elapsed", time.Since(started))
	return nil
}

func fixtureMarkers(root string) (map[string]string, error) {
	markers := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && entry.Name() == "ora-v2-result.txt" {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > 4096 {
				return errors.New("oversized fixture marker")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			markers[path] = string(data)
		}
		return nil
	})
	return markers, err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
