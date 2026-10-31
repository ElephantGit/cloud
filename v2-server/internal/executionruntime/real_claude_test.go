//go:build linux

package executionruntime

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
)

// TestRealClaudeCodeRunsTheMessagesFixture runs the pinned Claude Code plugin on the real Node
// through the public execution port: a frozen agent configuration, a Messages grant, the
// fixture asking for Bash, the real tool writing the marker, and an Append on the same session.
func TestRealClaudeCodeRunsTheMessagesFixture(t *testing.T) {
	assets := os.Getenv("V2_NODE_ASSETS_DIR")
	if assets == "" {
		t.Skip("set V2_NODE_ASSETS_DIR for real Claude Code/Node/PostgreSQL evidence")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "vc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(ctx); err != nil {
		t.Fatal(err)
	}
	f.ctx = ctx
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
		if t.Failed() {
			logRealDiagnostics(t, driverRoot)
		}
	})
	bindings := map[string]nodeservices.Binding{}
	for id, choice := range f.models {
		bindings[id] = choice.Binding
	}
	driver, err := agentruntime.New(agentruntime.Options{
		Root: driverRoot, AssetsDirectory: assets,
		GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"),
		Bindings:   bindings,
		ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
			project, err := f.db.GetProject(ctx, scope, id)
			return agentruntime.Project{MirrorPath: project.MirrorPath, Commit: project.Commit, SnapshotRef: project.SnapshotRef}, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = New(ctx, Config{Store: f.db, Models: f.models, Agent: driver})
	if err != nil {
		driver.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if err := f.service.Close(cleanup); err != nil {
			t.Error(err)
		}
		if err := driver.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	caller := execution.Caller{Kind: execution.CallerChat, ID: "claude-conversation", AttemptID: "first"}
	started := time.Now()
	first, err := f.service.Start(ctx, f.scope, execution.StartRequest{Caller: caller, Target: execution.Target{ProjectID: project.ID}, ExecutorID: "claude:fixture", Prompt: execution.TextPrompt("Read the repository README and write the requested fixture marker using the shell tool.")})
	if err != nil {
		t.Fatal(err)
	}
	result, sequence := observeTurn(t, f, first, 0)
	requireRealClaudeResult(t, result)
	t.Logf("first Claude Code turn completed in %s", time.Since(started).Round(time.Millisecond))
	if markers := realFixtureMarkers(t, driverRoot); len(markers) != 1 {
		t.Fatalf("Claude Code Bash tool wrote %d checkout markers, want 1: %v", len(markers), markers)
	}
	caller.AttemptID = "follow-up"
	second, err := f.service.Append(ctx, f.scope, execution.AppendRequest{SessionID: first.SessionID, Caller: caller, Prompt: execution.TextPrompt("Do it once more.")})
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("Append opened another session: %+v", second)
	}
	result, _ = observeTurn(t, f, second, sequence)
	requireRealClaudeResult(t, result)
	if err := f.service.End(ctx, f.scope, first.SessionID); err != nil {
		t.Fatal(err)
	}
}

// requireRealClaudeResult accepts the fixture answer; unlike OpenCode, Claude Code reports the
// fixture's token usage, so usage is not part of the assertion.
func requireRealClaudeResult(t *testing.T, result execution.Result) {
	t.Helper()
	if result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != nodeservices.FixtureAnswer {
		t.Fatalf("real Claude Code result=%+v", result)
	}
}

// logRealDiagnostics prints the tails of the Node, host and event logs under root.
func logRealDiagnostics(t *testing.T, root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || (entry.Name() != "nb.log" && entry.Name() != "hb.log" && entry.Name() != "node-events.jsonl") {
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
}

// TestConfiguredMessagesProviderRunsClaudeCode makes external Messages requests only after an
// explicit opt-in. Claude Code on the configured provider must use the real Bash tool to write
// the marker and answer; it shares the OpenAI acceptance's opaque-error and cleanup rules.
func TestConfiguredMessagesProviderRunsClaudeCode(t *testing.T) {
	// A separate switch, so opting in to the OpenAI acceptance does not demand Messages config.
	if os.Getenv("V2_REAL_MESSAGES_PROVIDER_TEST") != "1" {
		t.Skip("external Messages provider NOT verified; set V2_REAL_MESSAGES_PROVIDER_TEST=1 with all required configuration to opt in")
	}
	for _, name := range []string{"V2_ANTHROPIC_MODEL_BASE_URL", "V2_ANTHROPIC_MODEL_API_KEY", "V2_ANTHROPIC_MODEL_NAME", "TEST_DATABASE_URL", "V2_NODE_ASSETS_DIR"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("explicit external Messages provider acceptance requires %s", name)
		}
	}
	models, err := ModelsFromEnvironment(os.Getenv)
	configuredOK(t, "configure external Messages model", err)
	choice, ok := models["messages-configured"]
	if !ok {
		t.Fatal("configured Messages model is unavailable; fixture cannot satisfy external acceptance")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
	configuredOK(t, "locate Git backend", err)
	f := configuredDatabase(t, ctx, models)
	driverRoot := filepath.Join(f.root, "a")
	f.driver, err = agentruntime.New(agentruntime.Options{
		Root: driverRoot, AssetsDirectory: os.Getenv("V2_NODE_ASSETS_DIR"), GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"),
		Bindings: map[string]nodeservices.Binding{"messages-configured": choice.Binding},
		ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
			project, err := f.db.GetProject(ctx, scope, id)
			return agentruntime.Project{MirrorPath: project.MirrorPath, Commit: project.Commit, SnapshotRef: project.SnapshotRef}, err
		},
	})
	configuredOK(t, "initialize real Agent driver (requires a short native Linux home path)", err)
	f.service, err = New(ctx, Config{Store: f.db, Models: models, Agent: f.driver})
	configuredOK(t, "initialize public execution Service", err)
	repositories, err := projectrepos.New(f.db, projectrepos.Options{Root: filepath.Join(f.root, "r")})
	configuredOK(t, "initialize isolated project repository", err)
	project, err := repositories.SeedDemo(ctx, f.scope)
	configuredOK(t, "seed isolated real Git project", err)
	ref, err := f.service.Start(ctx, f.scope, execution.StartRequest{
		Caller: execution.Caller{Kind: execution.CallerChat, ID: "external-claude", AttemptID: "first"}, Target: execution.Target{ProjectID: project.ID}, ExecutorID: "claude:configured",
		Prompt: execution.TextPrompt("Use the Bash tool in this repository to read README.md, then run exactly:\n```sh\nprintf '%s\\n' 'v2 configured provider tool executed' > " + configuredMarkerName + "\n```\nDo not modify any other files. After the tool succeeds, reply with one short plain-text confirmation sentence."),
	})
	configuredOK(t, "Start real Claude Code admission", err)
	result, _ := observeTurn(t, &f.runtimeFixture, ref, 0)
	configuredComplete(t, "Claude Code", result)
	configuredToolMarker(t, driverRoot)
	configuredOK(t, "End Claude Code session", f.service.End(ctx, f.scope, ref.SessionID))
	// Never print answers, provider/DB URLs, credentials or raw Node diagnostics, even on failure.
	t.Log("external Messages model drove Claude Code's real Bash tool to a completed answer")
}
