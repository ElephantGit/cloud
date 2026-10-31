//go:build linux

package executionruntime

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// TestRealServiceAgentPostgresRestartAndConcurrentConversations crosses the public execution
// port, real Node/Agent/tool filesystem effects and the production PostgreSQL store. Only model
// responses are fixtures. Run the compiled Linux test on a native Linux filesystem with prepared
// assets; this is independent of browser/chat HTTP integration and does not claim turn-only Cancel.
func TestRealServiceAgentPostgresRestartAndConcurrentConversations(t *testing.T) {
	assets := os.Getenv("V2_NODE_ASSETS_DIR")
	if assets == "" {
		t.Skip("set V2_NODE_ASSETS_DIR for real Service/Node/Agent/PostgreSQL evidence")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "ve-")
	if err != nil {
		t.Fatal(err)
	}
	// The test owns this freshly generated native-home directory; close processes before removal.
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	f.ctx = ctx // The short unit-fixture setup is complete; real processes use the total bound.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if f.service != nil {
			if err := f.service.Close(cleanup); err != nil {
				t.Error(err)
			}
		}
	})
	repositories, err := projectrepos.New(f.db, projectrepos.Options{Root: filepath.Join(root, "r")})
	if err != nil {
		t.Fatal(err)
	}
	project, err := repositories.SeedDemo(ctx, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	driverRoot := filepath.Join(root, "a")
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		_ = filepath.WalkDir(driverRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if entry.Name() != "nb.log" && entry.Name() != "hb.log" && entry.Name() != "node-events.jsonl" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if len(data) > 16384 {
				data = data[len(data)-16384:]
			}
			t.Logf("real fixture diagnostic %s:\n%s", path, data)
			return nil
		})
	})
	newDriver := func() *agentruntime.Driver {
		t.Helper()
		driver, err := agentruntime.New(agentruntime.Options{
			Root: driverRoot, AssetsDirectory: assets,
			GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"),
			Bindings:   map[string]nodeservices.Binding{"fixture": f.models["fixture"].Binding},
			ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
				project, err := f.db.GetProject(ctx, scope, id)
				return agentruntime.Project{MirrorPath: project.MirrorPath, Commit: project.Commit, SnapshotRef: project.SnapshotRef}, err
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return driver
	}
	driver := newDriver()
	f.service, err = New(ctx, Config{Store: f.db, Models: f.models, Agent: driver})
	if err != nil {
		driver.Close(ctx)
		t.Fatal(err)
	}
	request := func(conversation, attempt string) execution.StartRequest {
		return execution.StartRequest{Caller: execution.Caller{Kind: execution.CallerChat, ID: conversation, AttemptID: attempt}, Target: execution.Target{ProjectID: project.ID}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("Read the repository README and write the requested fixture marker using the shell tool.")}
	}
	start := func(req execution.StartRequest) execution.Ref {
		t.Helper()
		ref, err := f.service.Start(ctx, f.scope, req)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	firstRequest := request("first-conversation", "first")
	first := start(firstRequest)
	firstResult, _ := observeTurn(t, f, first, 0)
	requireRealFixtureResult(t, firstResult)
	initialMarkers := realFixtureMarkers(t, driverRoot)
	if len(initialMarkers) != 1 {
		t.Fatalf("first Agent tool wrote %d checkout markers, want 1: %v", len(initialMarkers), initialMarkers)
	}
	if err := f.service.End(ctx, f.scope, first.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(ctx); err != nil {
		t.Fatal(err)
	}
	f.db.Close()
	f.db, err = store.OpenConfig(ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	// Pause both new conversations at the actual provider boundary, proving they are admitted
	// and running concurrently. Releasing the gate delegates unchanged responses to the fixture.
	gate := &concurrentRealFixture{started: make(chan struct{}, 2), release: make(chan struct{})}
	t.Cleanup(gate.unblock)
	choice := f.models["fixture"]
	choice.Binding.Provider = gate
	f.models["fixture"] = choice
	driver = newDriver()
	f.service, err = New(ctx, Config{Store: f.db, Models: f.models, Agent: driver})
	if err != nil {
		driver.Close(ctx)
		t.Fatal(err)
	}
	recovered, err := f.service.ReadAttempt(ctx, f.scope, firstRequest.Caller)
	if err != nil || recovered.Ref != first || recovered.Result == nil || !reflect.DeepEqual(*recovered.Result, firstResult) {
		t.Fatalf("PostgreSQL result after runtime/driver/pool reopen=%+v err=%v", recovered, err)
	}
	oldSession, err := f.service.ReadSession(ctx, f.scope, first.SessionID)
	if err != nil || oldSession.EndedAt == nil {
		t.Fatalf("old session=%+v err=%v", oldSession, err)
	}
	late := firstRequest.Caller
	late.AttemptID = "after-restart"
	if _, err := f.service.Append(ctx, f.scope, execution.AppendRequest{SessionID: first.SessionID, Caller: late, Prompt: execution.TextPrompt("must not resume")}); !errors.Is(err, execution.ErrSessionEnded) {
		t.Fatalf("reopened session Append=%v", err)
	}

	a, b := start(request("parallel-a", "first")), start(request("parallel-b", "first"))
	for range 2 {
		select {
		case <-gate.started:
		case <-ctx.Done():
			t.Fatal("two real Agent conversations did not reach the provider concurrently: ", ctx.Err())
		}
	}
	aSession, err := f.service.ReadSession(ctx, f.scope, a.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	bSession, err := f.service.ReadSession(ctx, f.scope, b.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if aSession.WorkspaceID == "" || aSession.WorkspaceID == bSession.WorkspaceID {
		t.Fatalf("shared checkout identity: A=%s B=%s", aSession.WorkspaceID, bSession.WorkspaceID)
	}
	gate.unblock()
	aResult, _ := observeTurn(t, f, a, 0)
	requireRealFixtureResult(t, aResult)
	bResult, bCursor := observeTurn(t, f, b, 0)
	requireRealFixtureResult(t, bResult)
	markers := realFixtureMarkers(t, driverRoot)
	newMarkers := 0
	for marker := range markers {
		if !initialMarkers[marker] {
			newMarkers++
		}
	}
	if newMarkers != 2 || len(markers) != 3 {
		t.Fatalf("two conversations did not write distinct real checkouts: initial=%v after=%v", initialMarkers, markers)
	}
	// #801: the pinned Node ends the whole Agent session when cancelling an active turn.
	// Hold a real provider request so completion cannot win before Cancel is exercised.
	blocked := gate.blockNextRequest()
	aCaller := request("parallel-a", "cancel-active").Caller
	aActive, err := f.service.Append(ctx, f.scope, execution.AppendRequest{SessionID: a.SessionID, Caller: aCaller, Prompt: execution.TextPrompt("Start another fixture turn and wait for cancellation.")})
	if err != nil {
		t.Fatal(err)
	}
	var providerContext context.Context
	select {
	case providerContext = <-blocked.started:
	case <-ctx.Done():
		t.Fatal("active Agent did not reach the cancellation barrier: ", ctx.Err())
	}
	cancelled, err := f.service.Cancel(ctx, f.scope, aActive)
	if err != nil || cancelled.Status != execution.StatusCancelled {
		t.Fatalf("real active Agent Cancel=%+v err=%v", cancelled, err)
	}
	if providerContext.Err() == nil {
		t.Fatal("Agent Cancel returned before the real provider request was cancelled")
	}
	select {
	case <-blocked.cancelled:
	case <-ctx.Done():
		t.Fatal("real provider did not exit after cancellation: ", ctx.Err())
	}
	aAfterCancel, err := f.service.ReadSession(ctx, f.scope, a.SessionID)
	if err != nil || aAfterCancel.EndedAt == nil {
		t.Fatalf("real Agent Cancel did not end its session: %+v err=%v", aAfterCancel, err)
	}
	aCaller.AttemptID = "after-cancel"
	if _, err := f.service.Append(ctx, f.scope, execution.AppendRequest{SessionID: a.SessionID, Caller: aCaller, Prompt: execution.TextPrompt("must not resume")}); !errors.Is(err, execution.ErrSessionEnded) {
		t.Fatalf("real Agent Append after active Cancel=%v", err)
	}
	if err := f.service.End(ctx, f.scope, a.SessionID); err != nil {
		t.Fatal(err)
	}
	bCaller := request("parallel-b", "second").Caller
	appended, err := f.service.Append(ctx, f.scope, execution.AppendRequest{SessionID: b.SessionID, Caller: bCaller, Prompt: execution.TextPrompt("Read README and write the fixture marker again.")})
	if err != nil {
		t.Fatalf("ending A prevented B append: %v", err)
	}
	appendedResult, _ := observeTurn(t, f, appended, bCursor)
	requireRealFixtureResult(t, appendedResult)
	if err := f.service.End(ctx, f.scope, b.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if got := realFixtureMarkers(t, driverRoot); len(got) != len(markers) {
		t.Fatalf("End removed retained checkouts: before=%v after=%v", markers, got)
	}
	t.Logf("real Service/Agent/PostgreSQL evidence: reopened %s, distinct workspaces %s / %s, %d retained tool markers", first.TurnID, aSession.WorkspaceID, bSession.WorkspaceID, len(markers))
}

func requireRealFixtureResult(t *testing.T, result execution.Result) {
	t.Helper()
	if result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != nodeservices.FixtureAnswer || result.Usage != nil {
		t.Fatalf("real Agent result=%+v", result)
	}
}

func realFixtureMarkers(t *testing.T, root string) map[string]bool {
	t.Helper()
	markers := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "ora-v2-result.txt" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != nodeservices.FixtureMarker {
			t.Errorf("real tool marker %s = %q, want %q", path, data, nodeservices.FixtureMarker)
		}
		markers[path] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return markers
}

type concurrentRealFixture struct {
	started, release chan struct{}
	once             sync.Once
	mu               sync.Mutex
	blocked          *realCancellationBarrier
}

func (p *concurrentRealFixture) unblock() { p.once.Do(func() { close(p.release) }) }

type realCancellationBarrier struct {
	started   chan context.Context
	cancelled chan struct{}
}

func (p *concurrentRealFixture) blockNextRequest() *realCancellationBarrier {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.blocked = &realCancellationBarrier{started: make(chan context.Context, 1), cancelled: make(chan struct{})}
	return p.blocked
}

func (p *concurrentRealFixture) Complete(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
	p.mu.Lock()
	blocked := p.blocked
	p.blocked = nil
	p.mu.Unlock()
	if blocked != nil {
		blocked.started <- r.Context()
		<-r.Context().Done()
		close(blocked.cancelled)
		return
	}
	select {
	case p.started <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
		nodeservices.FixtureProvider{}.Complete(w, r, model)
	case <-r.Context().Done():
	}
}
