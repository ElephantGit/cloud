//go:build linux

package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
	"github.com/wanglongan587/cloud/v2-server/internal/nodehistory"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

// The fake replaces only the IPC peer and supervisor proof. The dispatcher, control waits,
// session ownership, history reader, journal, publication and lifecycle are the real driver.
type fakeNodeIPC struct {
	project *projectRuntime
	id      node.Identity
	in      chan node.Message
	sent    chan node.Message
	acked   chan node.Message
	closed  chan struct{}
	once    sync.Once
	stopOK  atomic.Bool
	stops   atomic.Int32
	history map[string][]json.RawMessage
	seq     map[string]uint64
	hook    func(*fakeNodeIPC, node.Message) bool
}

func (f *fakeNodeIPC) Identity() node.Identity { return f.id }
func (f *fakeNodeIPC) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}
func (f *fakeNodeIPC) Receive(ctx context.Context) (node.Message, error) {
	select {
	case <-ctx.Done():
		return node.Message{}, ctx.Err()
	case <-f.closed:
		return node.Message{}, net.ErrClosed
	case m := <-f.in:
		return m, nil
	}
}
func (f *fakeNodeIPC) Ack(ctx context.Context, m node.Message) error {
	select {
	case f.acked <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (f *fakeNodeIPC) Send(ctx context.Context, m node.Message) error {
	select {
	case <-f.closed:
		return net.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case f.sent <- m:
	}
	if f.hook != nil && f.hook(f, m) {
		return nil
	}
	switch m.Type {
	case "install_plugins":
		f.reply(m, "plugins_result", map[string]any{"kind": "plugins_completed", "result": map[string]any{"items": []any{map[string]any{"outcome": map[string]string{"kind": "installed"}}}}})
	case "clone_repository":
		f.reply(m, "clone_result", map[string]any{"kind": "clone_ready", "result": map[string]string{"path": filepath.Join(f.project.root, "w", "r", m.ExecutionID), "commit": f.project.configuration.Commit}})
	}
	return nil
}
func (f *fakeNodeIPC) reply(command node.Message, kind string, payload any) {
	data, _ := json.Marshal(payload)
	f.in <- node.Message{Type: kind, Version: 1, OperationID: command.OperationID, ExecutionID: command.ExecutionID, Payload: data}
}
func (f *fakeNodeIPC) stop(context.Context) error {
	f.stops.Add(1)
	if !f.stopOK.Load() {
		return execution.ErrStopUnconfirmed
	}
	return f.Close()
}

type ipcHarness struct {
	driver  *Driver
	created chan *fakeNodeIPC
	config  Project
	scope   execution.Scope
}

func newIPCHarness(t *testing.T, hook func(*fakeNodeIPC, node.Message) bool) *ipcHarness {
	t.Helper()
	root := t.TempDir()
	config := Project{MirrorPath: filepath.Join(root, "mirror.git"), Commit: "0123456789012345678901234567890123456789", SnapshotRef: "refs/snapshots/demo"}
	model := nodeservices.Model{ID: "fixture", Name: "Fixture", ContextWindow: 4096, MaxTokens: 256}
	bindings := map[string]nodeservices.Binding{
		"fixture":          {Model: model, Protocol: nodeservices.OpenAICompletions, Provider: nodeservices.FixtureProvider{}},
		"messages-fixture": {Model: model, Protocol: nodeservices.AnthropicMessages, Provider: nodeservices.MessagesFixtureProvider{}},
	}
	h := &ipcHarness{created: make(chan *fakeNodeIPC, 16), config: config, scope: execution.Scope{TenantID: "tenant", UserID: "user"}}
	d := &Driver{opts: Options{Root: filepath.Join(root, "projects"), Bindings: bindings, ResolveProject: func(context.Context, execution.Scope, string) (Project, error) { return config, nil }}, boot: "fake", projects: map[string]*projectRuntime{}, sessions: map[string]*agentSession{}, controlTimeout: time.Second}
	h.driver = d
	d.startProject = func(p *projectRuntime) error {
		if err := os.MkdirAll(filepath.Join(p.root, "w", "n"), 0700); err != nil {
			return err
		}
		packagePath := filepath.Join(p.root, "test.orax")
		data := []byte("synthetic test package")
		if err := os.WriteFile(packagePath, data, 0600); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		var err error
		p.services, err = nodeservices.Start(nodeservices.Options{StateDirectory: p.root, Repository: config.MirrorPath, GitBackend: "/bin/false", Packages: []nodeservices.Package{{Name: "test", Path: packagePath, SHA256: hex.EncodeToString(sum[:])}}, Bindings: bindings})
		if err != nil {
			return err
		}
		p.reader, err = nodehistory.Open(filepath.Join(p.root, "w", "n"))
		if err != nil {
			return err
		}
		p.journal, err = os.OpenFile(filepath.Join(p.root, "node-events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		f := &fakeNodeIPC{project: p, id: node.Identity{NodeID: "node", IncarnationID: d.nextID("incarnation")}, in: make(chan node.Message, 64), sent: make(chan node.Message, 64), acked: make(chan node.Message, 64), closed: make(chan struct{}), history: map[string][]json.RawMessage{}, seq: map[string]uint64{}, hook: hook}
		f.stopOK.Store(true)
		p.client, p.stop, p.receiveStarted = f, f.stop, true
		go p.receive()
		h.created <- f
		m, err := p.control(p.ctx, d.nextID("install"), "install_plugins", "plugins_result", map[string]string{"node_id": f.id.NodeID})
		if err != nil {
			return err
		}
		var result struct{ Kind string }
		if json.Unmarshal(m.Payload, &result) != nil || result.Kind != "plugins_completed" {
			return execution.ErrUnavailable
		}
		return nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := d.Close(ctx); err != nil {
			t.Errorf("driver cleanup: %v", err)
		}
	})
	return h
}

func (h *ipcHarness) session(id, workspace string) execution.Session {
	return execution.Session{ID: id, WorkspaceID: workspace, Target: &execution.Target{ProjectID: "project"}, Executor: &execution.Executor{ID: "executor", AgentID: pluginID, PluginVersion: pluginVersion, Model: execution.Model{ID: "fixture"}}}
}
func ipcContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func receiveIPC[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("fake IPC operation did not finish")
		var zero T
		return zero
	}
}
func nextIPCCommand(t *testing.T, f *fakeNodeIPC, kind string) node.Message {
	t.Helper()
	for {
		m := receiveIPC(t, f.sent)
		if m.Type == kind {
			return m
		}
	}
}
func historyLine(sequence int, kind string, extra map[string]any) json.RawMessage {
	v := map[string]any{"at": "2026-10-30T12:00:00Z", "seq": sequence, "type": kind}
	for k, value := range extra {
		v[k] = value
	}
	data, _ := json.Marshal(v)
	return data
}
func (f *fakeNodeIPC) finishTurn(t *testing.T, command node.Message, turnID, answer string) {
	t.Helper()
	lines := f.history[command.ExecutionID]
	if len(lines) == 0 {
		lines = append(lines, historyLine(0, "meta", map[string]any{"sessionId": command.ExecutionID, "schemaVersion": 2}))
	}
	lines = append(lines, historyLine(len(lines), "update", map[string]any{"update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]string{"type": "text", "text": "prompt"}}}))
	lines = append(lines, historyLine(len(lines), "update", map[string]any{"update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": answer}}}))
	end := historyLine(len(lines), "turnEnded", map[string]any{"stop_reason": "end_turn"})
	lines = append(lines, end)
	f.history[command.ExecutionID] = lines
	path := filepath.Join(f.project.root, "w", "n", "sessions", command.ExecutionID[:2], command.ExecutionID[2:4], command.ExecutionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(file, string(line)); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	f.seq[command.ExecutionID]++
	sequence := f.seq[command.ExecutionID]
	payload, _ := json.Marshal(map[string]any{"turn_id": turnID, "history_sequence": len(lines) - 1, "record": end})
	f.in <- node.Message{Type: "thread_event", Version: 1, OperationID: command.OperationID, ExecutionID: command.ExecutionID, Sequence: &sequence, Payload: payload}
}
func (f *fakeNodeIPC) terminal(command node.Message) {
	f.seq[command.ExecutionID]++
	sequence := f.seq[command.ExecutionID]
	payload, _ := json.Marshal(map[string]any{"kind": "agent_session_ended", "result": map[string]any{"reason": "user_ended", "node": f.id}})
	f.in <- node.Message{Type: "agent_session_ended", Version: 1, OperationID: command.OperationID, ExecutionID: command.ExecutionID, Sequence: &sequence, Payload: payload}
}

type ipcRunOutcome struct {
	result execution.Result
	err    error
}

func startIPCRun(h *ipcHarness, ctx context.Context, session execution.Session, turn string, emit Emit) <-chan ipcRunOutcome {
	done := make(chan ipcRunOutcome, 1)
	go func() {
		result, err := h.driver.Run(ctx, h.scope, session, execution.Turn{Ref: execution.Ref{SessionID: session.ID, TurnID: turn}, Prompt: execution.TextPrompt("prompt")}, emit)
		done <- ipcRunOutcome{result, err}
	}()
	return done
}
func discardIPCEvent(context.Context, execution.Event) error { return nil }

func TestDriverIPCFullTurnPublicationAppendAndEnd(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	publicationStarted, publish := make(chan struct{}), make(chan struct{})
	done := startIPCRun(h, ctx, session, "first", func(ctx context.Context, event execution.Event) error {
		if event.Kind == execution.EventTurnEnded {
			close(publicationStarted)
			select {
			case <-publish:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	f := receiveIPC(t, h.created)
	start := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, start, "first", "complete answer")
	receiveIPC(t, publicationStarted)
	select {
	case ack := <-f.acked:
		t.Fatalf("ACK before terminal publication: %+v", ack)
	case result := <-done:
		t.Fatalf("Run returned before terminal publication: %+v", result)
	default:
	}
	close(publish)
	ack := receiveIPC(t, f.acked)
	if ack.ExecutionID != start.ExecutionID || ack.Sequence == nil || *ack.Sequence != 1 {
		t.Fatalf("wrong retained ACK: %+v", ack)
	}
	first := receiveIPC(t, done)
	if first.err != nil || first.result.Status != execution.StatusCompleted || first.result.Answer != "complete answer" || !first.result.AnswerComplete {
		t.Fatalf("first turn: %+v", first)
	}
	secondDone := startIPCRun(h, ctx, session, "second", discardIPCEvent)
	appendCommand := nextIPCCommand(t, f, "submit_user_turn")
	if appendCommand.ExecutionID != start.ExecutionID {
		t.Fatal("Append silently replaced physical Agent")
	}
	f.finishTurn(t, appendCommand, "second", "second answer")
	second := receiveIPC(t, secondDone)
	if second.err != nil || second.result.Answer != "second answer" || !second.result.AnswerComplete {
		t.Fatalf("second turn: %+v", second)
	}
	receiveIPC(t, f.acked)
	ended := make(chan error, 1)
	go func() { ended <- h.driver.End(ctx, session.ID) }()
	endCommand := nextIPCCommand(t, f, "end_session")
	select {
	case err := <-ended:
		t.Fatalf("End completed before Node terminal: %v", err)
	default:
	}
	f.terminal(endCommand)
	if err := receiveIPC(t, ended); err != nil {
		t.Fatal(err)
	}
	if err := h.driver.End(ctx, session.ID); err != nil {
		t.Fatalf("End is not idempotent: %v", err)
	}
}

// Claude Code is installed on first use with its declaration binding, and its session carries
// the frozen configuration Node re-proves before granting Messages access.
func TestDriverIPCClaudeSessionInstallsAndFreezesItsConfiguration(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	claude := h.session("claude", "workspace")
	claude.Executor.AgentID, claude.Executor.PluginVersion = "official/ora-space.claude", "0.4.5"
	mismatched := claude
	mismatched.ID, mismatched.WorkspaceID = "mismatched", "other"
	if got := receiveIPC(t, startIPCRun(h, ctx, mismatched, "first", discardIPCEvent)); !errors.Is(got.err, execution.ErrUnsupported) {
		t.Fatalf("Claude Code on an OpenAI binding=%+v", got)
	}
	executor := *claude.Executor
	executor.Model.ID = "messages-fixture"
	claude.Executor = &executor
	done := startIPCRun(h, ctx, claude, "first", discardIPCEvent)
	f := receiveIPC(t, h.created)
	pkg, err := nodeservices.HostClaudePackage()
	if err != nil {
		t.Fatal(err)
	}
	var install struct {
		Spec struct {
			Plugins []struct {
				PluginID           string `json:"plugin_id"`
				Version            string
				AgentConfiguration map[string]any `json:"agent_configuration"`
			}
		}
	}
	for {
		m := nextIPCCommand(t, f, "install_plugins")
		if err := json.Unmarshal(m.Payload, &install); err != nil {
			t.Fatal(err)
		}
		// The harness installs its own synthetic package first.
		if len(install.Spec.Plugins) == 1 && install.Spec.Plugins[0].PluginID == "official/ora-space.claude" {
			break
		}
	}
	if got, want := install.Spec.Plugins[0].AgentConfiguration, map[string]any{"schema_version": 2.0, "declaration_sha256": nodeservices.ClaudeDeclarationSHA256}; install.Spec.Plugins[0].Version != "0.4.5" || !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude Code install=%+v", install.Spec.Plugins[0])
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	var start struct {
		Spec struct {
			PluginID           string         `json:"agent_plugin_id"`
			Binding            string         `json:"model_binding_id"`
			AgentConfiguration map[string]any `json:"agent_configuration"`
		}
	}
	if err := json.Unmarshal(command.Payload, &start); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"plugin_id": "official/ora-space.claude", "plugin_version": "0.4.5", "target": pkg.Target, "package_sha256": pkg.SHA256,
		"schema_version": 2.0, "declaration_fingerprint": nodeservices.ClaudeDeclarationSHA256, "adapter_version": 1.0, "configuration_revision": 0.0,
		"settings_json": "{}", "protocol": "anthropic-messages",
		"model": map[string]any{"id": "fixture", "name": "Fixture", "context_window": 4096.0, "max_tokens": 256.0},
	}
	if start.Spec.PluginID != "official/ora-space.claude" || start.Spec.Binding != "messages-fixture" || !reflect.DeepEqual(start.Spec.AgentConfiguration, want) {
		t.Fatalf("Claude Code start=%+v", start.Spec)
	}
	f.finishTurn(t, command, "first", "claude answer")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "claude answer" {
		t.Fatalf("Claude Code turn=%+v", got)
	}
}

func TestDriverIPCRetriesFailedPreparationAndWorkspaceReservation(t *testing.T) {
	var cloneCalls atomic.Int32
	h := newIPCHarness(t, func(f *fakeNodeIPC, m node.Message) bool {
		if m.Type == "clone_repository" && cloneCalls.Add(1) == 1 {
			f.reply(m, "clone_result", map[string]string{"kind": "clone_failed"})
			return true
		}
		return false
	})
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	first := receiveIPC(t, startIPCRun(h, ctx, session, "first", discardIPCEvent))
	if !errors.Is(first.err, execution.ErrUnavailable) {
		t.Fatalf("transient clone failure = %+v", first)
	}
	f := receiveIPC(t, h.created)
	failedClone := nextIPCCommand(t, f, "clone_repository")
	done := startIPCRun(h, ctx, session, "retry", discardIPCEvent)
	retriedClone := nextIPCCommand(t, f, "clone_repository")
	if retriedClone.ExecutionID == failedClone.ExecutionID {
		t.Fatal("clone retry reused execution ID; a late reply could satisfy it")
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "retry", "recovered")
	result := receiveIPC(t, done)
	if result.err != nil || result.result.Answer != "recovered" || cloneCalls.Load() != 2 {
		t.Fatalf("same session/workspace did not recover: %+v cloneCalls=%d", result, cloneCalls.Load())
	}
	// Another logical session cannot acquire this checkout before a valid physical End.
	busy := receiveIPC(t, startIPCRun(h, ctx, h.session("other", "workspace"), "busy", discardIPCEvent))
	if !errors.Is(busy.err, execution.ErrWorkspaceBusy) {
		t.Fatalf("live checkout was shared: %+v", busy)
	}
	ended := make(chan error, 1)
	go func() { ended <- h.driver.End(ctx, session.ID) }()
	f.terminal(nextIPCCommand(t, f, "end_session"))
	if err := receiveIPC(t, ended); err != nil {
		t.Fatal(err)
	}
	done = startIPCRun(h, ctx, h.session("other", "workspace"), "after-end", discardIPCEvent)
	command = nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "after-end", "next session")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "next session" {
		t.Fatalf("checkout not released after confirmed End: %+v", got)
	}
}

func TestDriverIPCInitialFailureCanRetrySameSession(t *testing.T) {
	var installs atomic.Int32
	h := newIPCHarness(t, func(f *fakeNodeIPC, m node.Message) bool {
		if m.Type == "install_plugins" && installs.Add(1) == 1 {
			f.reply(m, "plugins_result", map[string]string{"kind": "plugins_failed"})
			return true
		}
		return false
	})
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	got := receiveIPC(t, startIPCRun(h, ctx, session, "first", discardIPCEvent))
	if !errors.Is(got.err, execution.ErrUnavailable) {
		t.Fatalf("install failure: %+v", got)
	}
	old := receiveIPC(t, h.created)
	done := startIPCRun(h, ctx, session, "retry", discardIPCEvent)
	f := receiveIPC(t, h.created)
	if f.id == old.id || old.stops.Load() == 0 {
		t.Fatal("retry did not stop failed incarnation before replacing it")
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "retry", "recovered install")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "recovered install" {
		t.Fatalf("init failure remained cached: %+v", got)
	}
}

func TestDriverIPCDisconnectRequiresStopProofBeforeRebuild(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	done := startIPCRun(h, ctx, h.session("first", "workspace"), "turn", discardIPCEvent)
	old := receiveIPC(t, h.created)
	nextIPCCommand(t, old, "start_agent_session")
	old.stopOK.Store(false)
	old.Close()
	if got := receiveIPC(t, done); got.result.FailureCode != "agent_transport_closed" || got.err == nil {
		t.Fatalf("disconnect result: %+v", got)
	}
	got := receiveIPC(t, startIPCRun(h, ctx, h.session("next", "workspace"), "blocked", discardIPCEvent))
	if !errors.Is(got.err, execution.ErrUnavailable) || errors.Is(got.err, execution.ErrWorkspaceBusy) {
		t.Fatalf("unconfirmed dead project = %+v", got)
	}
	select {
	case <-h.created:
		t.Fatal("replacement started without supervisor stop proof")
	default:
	}
	if err := h.driver.End(ctx, "first"); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatalf("disconnect falsely confirmed old session stopped: %v", err)
	}
	old.stopOK.Store(true)
	done = startIPCRun(h, ctx, h.session("next", "workspace"), "retry", discardIPCEvent)
	f := receiveIPC(t, h.created)
	if f.id == old.id || old.stops.Load() < 2 {
		t.Fatal("replacement did not retry supervisor stop")
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "retry", "after disconnect")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "after disconnect" {
		t.Fatalf("dead project did not rebuild: %+v", got)
	}
	if err := h.driver.End(ctx, "first"); err != nil {
		t.Fatalf("supervisor stop proof did not settle old session: %v", err)
	}
}

func TestDriverIPCEndDisconnectedSessionStopsProjectBeforeWorkspaceReuse(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	session := h.session("first", "workspace")
	done := startIPCRun(h, ctx, session, "turn", discardIPCEvent)
	old := receiveIPC(t, h.created)
	nextIPCCommand(t, old, "start_agent_session")
	old.stopOK.Store(false)
	t.Cleanup(func() { old.stopOK.Store(true) })
	old.Close()
	if got := receiveIPC(t, done); got.result.FailureCode != "agent_transport_closed" || got.err == nil {
		t.Fatalf("disconnect result: %+v", got)
	}
	if err := h.driver.End(ctx, session.ID); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatalf("End without supervisor proof: %v", err)
	}
	if old.stops.Load() != 1 {
		t.Fatalf("End did not request supervisor stop: calls=%d", old.stops.Load())
	}
	h.driver.mu.Lock()
	retained := h.driver.projects["project"] == old.project
	a := h.driver.sessions[session.ID]
	h.driver.mu.Unlock()
	old.project.mu.Lock()
	owner := old.project.owners[session.WorkspaceID]
	old.project.mu.Unlock()
	if !retained || owner != session.ID {
		t.Fatalf("unconfirmed End released project or workspace: retained=%v owner=%q", retained, owner)
	}
	select {
	case <-a.stopped:
		t.Fatal("unconfirmed End marked physical session stopped")
	default:
	}

	// End alone must recover: no other Run or Driver.Close supplies the stop proof.
	old.stopOK.Store(true)
	ended := make(chan error, 2)
	for range 2 {
		go func() { ended <- h.driver.End(ctx, session.ID) }()
	}
	for range 2 {
		if err := receiveIPC(t, ended); err != nil {
			t.Fatalf("End retry after stop became available: %v", err)
		}
	}
	h.driver.mu.Lock()
	remaining := h.driver.projects["project"]
	h.driver.mu.Unlock()
	if remaining != nil {
		t.Fatal("confirmed End retained the dead project")
	}
	if err := h.driver.End(ctx, session.ID); err != nil || old.stops.Load() != 2 {
		t.Fatalf("End was not idempotent: err=%v supervisorCalls=%d", err, old.stops.Load())
	}
	done = startIPCRun(h, ctx, h.session("next", "workspace"), "next-turn", discardIPCEvent)
	f := receiveIPC(t, h.created)
	if f.id == old.id {
		t.Fatal("next session reused the stopped incarnation")
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "next-turn", "workspace reused")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "workspace reused" {
		t.Fatalf("same workspace did not recover after End: %+v", got)
	}
}

func TestDriverIPCEndDisconnectedSessionHonorsCloseWaitContext(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	done := startIPCRun(h, ctx, session, "turn", discardIPCEvent)
	f := receiveIPC(t, h.created)
	nextIPCCommand(t, f, "start_agent_session")
	f.Close()
	receiveIPC(t, done)
	if err := f.project.lockClose(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.project.unlockClose)
	ending, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := h.driver.End(ending, session.ID); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatalf("End while another stop owns the gate: %v", err)
	}
	if !errors.Is(ending.Err(), context.DeadlineExceeded) {
		t.Fatal("End did not wait for supervisor proof until the caller's deadline")
	}
	if f.stops.Load() != 0 {
		t.Fatalf("End bypassed another stop's gate: calls=%d", f.stops.Load())
	}
}

func TestDriverIPCControlTimeoutRetiresIncarnation(t *testing.T) {
	var clones atomic.Int32
	h := newIPCHarness(t, func(_ *fakeNodeIPC, m node.Message) bool {
		return m.Type == "clone_repository" && clones.Add(1) == 1
	})
	h.driver.controlTimeout = 100 * time.Millisecond
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	got := receiveIPC(t, startIPCRun(h, ctx, session, "timeout", discardIPCEvent))
	if !errors.Is(got.err, execution.ErrUnavailable) {
		t.Fatalf("missing clone reply: %+v", got)
	}
	old := receiveIPC(t, h.created)
	late := nextIPCCommand(t, old, "clone_repository")
	done := startIPCRun(h, ctx, session, "retry", discardIPCEvent)
	f := receiveIPC(t, h.created)
	command := nextIPCCommand(t, f, "start_agent_session")
	if f.id == old.id || old.stops.Load() == 0 {
		t.Fatal("timed-out clone's incarnation was reused")
	}
	// A valid-looking late reply on the retired transport cannot affect the new request.
	old.reply(late, "clone_result", map[string]any{"kind": "clone_ready", "result": map[string]string{"path": "/untrusted/late", "commit": h.config.Commit}})
	f.finishTurn(t, command, "retry", "after timeout")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "after timeout" {
		t.Fatalf("clone timeout remained cached or accepted late reply: %+v", got)
	}
}

func TestDriverIPCPublicationFailureNeverAcknowledgesHistory(t *testing.T) {
	h := newIPCHarness(t, nil)
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	done := startIPCRun(h, ctx, session, "turn", func(context.Context, execution.Event) error {
		return errors.New("synthetic persistence failure")
	})
	f := receiveIPC(t, h.created)
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "turn", "must not be published")
	got := receiveIPC(t, done)
	if got.err == nil || got.result.Status != execution.StatusFailed || got.result.FailureCode != "agent_event_write_failed" || got.result.AnswerComplete {
		t.Fatalf("publication failure reported success: %+v", got)
	}
	endCommand := nextIPCCommand(t, f, "end_session")
	f.terminal(endCommand)
	if err := h.driver.End(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-f.acked:
		t.Fatalf("ACK after failed publication: %+v", m)
	default:
	}
}

func TestDriverIPCInstallationTimeoutCanRebuild(t *testing.T) {
	var installs atomic.Int32
	h := newIPCHarness(t, func(_ *fakeNodeIPC, m node.Message) bool {
		return m.Type == "install_plugins" && installs.Add(1) == 1
	})
	h.driver.controlTimeout = 100 * time.Millisecond
	ctx := ipcContext(t)
	session := h.session("session", "workspace")
	if got := receiveIPC(t, startIPCRun(h, ctx, session, "timeout", discardIPCEvent)); !errors.Is(got.err, execution.ErrUnavailable) {
		t.Fatalf("missing plugin reply did not fail: %+v", got)
	}
	old := receiveIPC(t, h.created)
	done := startIPCRun(h, ctx, session, "retry", discardIPCEvent)
	f := receiveIPC(t, h.created)
	if f.id == old.id || old.stops.Load() == 0 {
		t.Fatal("silent installation retained its incarnation")
	}
	command := nextIPCCommand(t, f, "start_agent_session")
	f.finishTurn(t, command, "retry", "after installation timeout")
	if got := receiveIPC(t, done); got.err != nil || got.result.Answer != "after installation timeout" {
		t.Fatalf("installation timeout remained cached: %+v", got)
	}
}

func TestProjectCloseHonorsContextWhileAnotherStopOwnsGate(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	lifetime, stop := context.WithCancel(context.Background())
	defer stop()
	p := &projectRuntime{ready: ready, ctx: lifetime, cancel: stop}
	if err := p.lockClose(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer p.unlockClose()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.close(ctx); !errors.Is(err, execution.ErrStopUnconfirmed) {
		t.Fatalf("close waited past caller cancellation: %v", err)
	}
}
