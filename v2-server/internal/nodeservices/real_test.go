//go:build linux

package nodeservices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/node"
)

// This is the real process/filesystem boundary: fixture model responses, unmodified Node,
// Deno and OpenCode, real HTTPS clone, actual shell tool and process guardian cleanup.
func TestRealAgentCloneInstallToolAndEnd(t *testing.T) {
	assets := os.Getenv("V2_NODE_ASSETS_DIR")
	if assets == "" {
		t.Skip("set V2_NODE_ASSETS_DIR to prepared assets for the real Agent test")
	}
	preparationStarted := time.Now()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "va-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	bare, commit, snapshot := seedRepository(t, root)
	provider := &pausingFixture{started: make(chan struct{}), stopped: make(chan struct{})}
	s, err := Start(Options{StateDirectory: root, Repository: bare, GitBackend: filepath.Join(gitCommand(t, root, "--exec-path"), "git-http-backend"), PackagePath: filepath.Join(assets, "opencode-0.6.11.orax"), PackageSHA256: "88c00f81997fc7fe892f2d58a686f535678e562c7e000d33eb7ee3b1d0f754b5", Bindings: map[string]Binding{"binding": {testModel, provider}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	asset := func(name string) node.Asset {
		path := filepath.Join(assets, "bin", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return node.Asset{Path: path, SHA256: hex.EncodeToString(sum[:])}
	}
	ipc := node.Options{ControllerID: "controller", ExpectedNodeID: "node", HeartbeatInterval: time.Second, FrameTimeout: 10 * time.Second, RequiredCapabilities: []string{"plugin_install", "agent_session"}}
	opts := node.RuntimeOptions{Root: filepath.Join(root, "w"), Node: asset("ora-node"), Host: asset("ora-process-host"), Guardian: asset("ora-process-guardian"), IPC: ipc, StartupTimeout: 15 * time.Second, StopGrace: 2 * time.Second, Configure: func(p node.Paths) ([]byte, error) {
		return s.NodeConfiguration(p, ipc, filepath.Join(assets, "bin", "deno"))
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := node.StartRuntime(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	stop := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := r.Stop(stopCtx); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(stop)
	t.Cleanup(func() {
		if t.Failed() {
			for _, name := range []string{"nb.log", "hb.log"} {
				data, _ := os.ReadFile(filepath.Join(opts.Root, name))
				t.Logf("%s: %s", name, data)
			}
		}
	})
	send := func(kind, execution string, payload any) {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Client.Send(ctx, node.Message{Type: kind, Version: 1, OperationID: "op-" + execution, ExecutionID: execution, Payload: data}); err != nil {
			t.Fatal(err)
		}
	}
	// Retain every received envelope in an fsynced file before acknowledging it. This exercises
	// the transport's durability ordering without pretending to implement M3's PostgreSQL store.
	journal, err := os.Create(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	await := func(kind, execution string) json.RawMessage {
		t.Helper()
		for {
			m, err := r.Client.Receive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.NewEncoder(journal).Encode(m); err != nil {
				t.Fatal(err)
			}
			if err := journal.Sync(); err != nil {
				t.Fatal(err)
			}
			if m.Sequence != nil {
				if err := r.Client.Ack(ctx, m); err != nil {
					t.Fatal(err)
				}
			}
			if m.Type == "agent_session_ended" && kind != "agent_session_ended" {
				t.Fatalf("early Agent end: %s", m.Payload)
			}
			if m.Type == kind && m.ExecutionID == execution {
				return m.Payload
			}
		}
	}
	send("clone_repository", "clone", map[string]any{"spec": map[string]any{"node_id": "node", "repository_url": s.URL + "/repo.git", "snapshot_ref": snapshot, "base_commit": commit, "workspace_ref": "refs/heads/workspaces/22222222-2222-4222-8222-222222222222/main"}})
	cloned := await("clone_result", "clone")
	var clone struct {
		Kind   string
		Result struct{ Path, Commit string }
	}
	if json.Unmarshal(cloned, &clone) != nil || clone.Kind != "clone_ready" || clone.Result.Commit != commit {
		t.Fatalf("clone: %s", cloned)
	}
	if rel, err := filepath.Rel(filepath.Join(opts.Root, "r"), clone.Result.Path); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("clone escaped runtime: %s", clone.Result.Path)
	}
	t.Log("HTTPS clone matched snapshot commit")
	send("install_plugins", "install", map[string]any{"spec": map[string]any{"node_id": "node", "plugins": []any{map[string]any{"plugin_id": "official/ora-space.opencode", "version": "0.6.11", "release": map[string]any{"kind": "targets", "targets": []any{map[string]any{"target": "x86_64-unknown-linux-gnu", "download": map[string]string{"url": s.PackageURL, "sha256": s.opts.PackageSHA256}}}}}}}})
	installed := await("plugins_result", "install")
	var plugins struct {
		Kind   string
		Result struct {
			Items []struct{ Outcome struct{ Kind string } }
		}
	}
	if json.Unmarshal(installed, &plugins) != nil || plugins.Kind != "plugins_completed" || len(plugins.Result.Items) != 1 || plugins.Result.Items[0].Outcome.Kind != "installed" {
		t.Fatalf("plugins: %s", installed)
	}
	t.Logf("pinned OpenCode package installed by real Node; local preparation %s", time.Since(preparationStarted))
	turnStarted := time.Now()
	if err := s.Permit("agent", "binding"); err != nil {
		t.Fatal(err)
	}
	send("start_agent_session", "agent", map[string]any{"spec": map[string]any{"node_id": "node", "agent_plugin_id": "official/ora-space.opencode", "agent_plugin_version": "0.6.11", "checkout_execution_id": "clone", "model_binding_id": "binding", "git_identity": map[string]string{"name": "V2", "email": "v2@example.test"}, "initial_turn": map[string]any{"turn_id": "first", "content": []any{map[string]string{"type": "text", "text": "Inspect README and save the fixture result."}}}}})
	var answer strings.Builder
	for {
		data := await("thread_event", "agent")
		var event struct {
			VisibleText string `json:"visible_text"`
			Record      map[string]any
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		answer.WriteString(event.VisibleText)
		if event.Record["type"] == "turnEnded" {
			if fmt.Sprint(event.Record["stopReason"]) != "end_turn" && fmt.Sprint(event.Record["stop_reason"]) != "end_turn" {
				t.Fatalf("turn result: %s", data)
			}
			break
		}
	}
	if !strings.Contains(answer.String(), FixtureAnswer) {
		t.Fatalf("answer=%q", answer.String())
	}
	marker, err := os.ReadFile(filepath.Join(clone.Result.Path, "ora-v2-result.txt"))
	if err != nil || string(marker) != FixtureMarker {
		t.Fatalf("actual shell output=%q, %v", marker, err)
	}
	t.Logf("real shell tool wrote marker; Agent returned terminal answer; prepared first turn %s", time.Since(turnStarted))
	provider.pause.Store(true)
	send("submit_user_turn", "agent", map[string]any{"node_id": "node", "command_id": "second-command", "turn": map[string]any{"turn_id": "second", "content": []any{map[string]string{"type": "text", "text": "Continue, then wait for cancellation."}}}})
	// Receive must stay live while the next request starts; command/event output has its own
	// bounded Node transport. No sleep is used to assume the provider has begun.
	accepted := await("session_command_accepted", "agent")
	if !strings.Contains(string(accepted), "second-command") {
		t.Fatalf("append acceptance=%s", accepted)
	}
	select {
	case <-provider.started:
	case <-ctx.Done():
		t.Fatal("second turn did not call provider")
	}
	send("end_session", "agent", map[string]string{"node_id": "node", "command_id": "end", "reason": "cancelled"})
	ended := await("agent_session_ended", "agent")
	var terminal struct {
		Kind   string
		Result struct{ Reason string }
	}
	if json.Unmarshal(ended, &terminal) != nil || terminal.Kind != "agent_session_ended" || terminal.Result.Reason != "cancelled" {
		t.Fatalf("terminal=%s", ended)
	}
	s.End("agent")
	select {
	case <-provider.stopped:
	case <-ctx.Done():
		t.Fatal("ended Agent left provider request running")
	}
	t.Log("second turn cancellation closed the in-flight model request")
	stop()
	// Stop confirms no owned executable or unknown cwd remains. A second start further proves
	// host recovery is usable and the tool result survives normal cleanup.
	r, err = node.StartRuntime(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if marker, err = os.ReadFile(filepath.Join(clone.Result.Path, "ora-v2-result.txt")); err != nil || string(marker) != FixtureMarker {
		t.Fatalf("restart lost checkout: %v", err)
	}
	stop()
	t.Log("EndSession, guardian cleanup and host restart passed; checkout preserved")
}

type pausingFixture struct {
	pause            atomic.Bool
	started, stopped chan struct{}
}

func (p *pausingFixture) Complete(w http.ResponseWriter, r *http.Request, m Model) {
	if !p.pause.Load() {
		FixtureProvider{}.Complete(w, r, m)
		return
	}
	close(p.started)
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = flushWriter{w}.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Waiting\"},\"finish_reason\":null}]}\n\n"))
	<-r.Context().Done()
	close(p.stopped)
}
