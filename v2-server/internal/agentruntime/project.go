//go:build linux

package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/node"
	"github.com/wanglongan587/cloud/v2-server/internal/nodehistory"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

const (
	pluginID      = "official/ora-space.opencode"
	pluginVersion = "0.6.11"
	packageSHA    = "88c00f81997fc7fe892f2d58a686f535678e562c7e000d33eb7ee3b1d0f754b5"
)

type controlWait struct {
	kind string
	done chan node.Message
}

type checkout struct {
	id, path string
	ready    chan struct{}
	err      error
}

// One reader owns all projections. Control replies and sessions are dispatched by execution ID;
// no per-turn goroutine may call Client.Receive or close the project's shared transport.
type projectRuntime struct {
	driver        *Driver
	id, root      string
	configuration Project
	ready         chan struct{}
	initErr       error
	ctx           context.Context
	cancel        context.CancelFunc
	runtime       *node.Runtime
	services      *nodeservices.Service
	reader        *nodehistory.Reader
	journal       *os.File
	loopDone      chan struct{}
	dead          chan struct{}
	deadOnce      sync.Once
	closeMu       sync.Mutex
	closed        bool
	registered    bool
	mu            sync.Mutex
	fatal         bool
	controls      map[string]*controlWait
	agents        map[string]*agentSession
	checkouts     map[string]*checkout
	owners        map[string]string
	acknowledged  map[string]uint64
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func (d *Driver) project(id string, config Project) *projectRuntime {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	if p := d.projects[id]; p != nil {
		return p
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &projectRuntime{driver: d, id: id, root: filepath.Join(d.opts.Root, digest(id)[:16]), configuration: config,
		ready: make(chan struct{}), ctx: ctx, cancel: cancel, loopDone: make(chan struct{}), dead: make(chan struct{}),
		controls: map[string]*controlWait{}, agents: map[string]*agentSession{}, checkouts: map[string]*checkout{}, owners: map[string]string{}, acknowledged: map[string]uint64{}}
	d.projects[id] = p
	go func() {
		p.initErr = p.start()
		if p.initErr != nil {
			p.cancel()
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var cleanupErr error
			if p.runtime != nil {
				cleanupErr = p.runtime.Stop(cleanup)
				<-p.loopDone
			} else if p.registered {
				cleanupErr = node.RecoverRuntime(cleanup, filepath.Join(p.root, "w"), 3*time.Second)
			}
			if p.reader != nil {
				cleanupErr = errors.Join(cleanupErr, p.reader.Close())
				p.reader = nil
			}
			if p.journal != nil {
				cleanupErr = errors.Join(cleanupErr, p.journal.Close())
				p.journal = nil
			}
			if p.services != nil {
				cleanupErr = errors.Join(cleanupErr, p.services.Close())
			}
			p.closed = cleanupErr == nil
			p.deadOnce.Do(func() { close(p.dead) })
		}
		close(p.ready)
	}()
	return p
}

func (p *projectRuntime) start() error {
	opts := p.driver.opts
	if !filepath.IsAbs(p.configuration.MirrorPath) {
		return execution.ErrUnavailable
	}
	if err := p.register(); err != nil {
		return err
	}
	p.registered = true
	var err error
	p.services, err = nodeservices.Start(nodeservices.Options{StateDirectory: p.root, Repository: p.configuration.MirrorPath, GitBackend: opts.GitBackend,
		PackagePath: filepath.Join(opts.AssetsDirectory, "opencode-0.6.11.orax"), PackageSHA256: packageSHA, Bindings: opts.Bindings})
	if err != nil {
		return err
	}
	assets := make(map[string]node.Asset)
	for _, name := range []string{"ora-node", "ora-process-host", "ora-process-guardian"} {
		path := filepath.Join(opts.AssetsDirectory, "bin", name)
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		assets[name] = node.Asset{Path: path, SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	ipc := node.Options{ControllerID: "v2-execution", ExpectedNodeID: "v2-" + digest(p.id), HeartbeatInterval: time.Second, FrameTimeout: 15 * time.Second, RequiredCapabilities: []string{"plugin_install", "agent_session"}}
	p.runtime, err = node.StartRuntime(p.ctx, node.RuntimeOptions{Root: filepath.Join(p.root, "w"), Node: assets["ora-node"], Host: assets["ora-process-host"], Guardian: assets["ora-process-guardian"], IPC: ipc,
		StartupTimeout: 20 * time.Second, StopGrace: 3 * time.Second, Configure: func(paths node.Paths) ([]byte, error) {
			return p.services.NodeConfiguration(paths, ipc, filepath.Join(opts.AssetsDirectory, "bin", "deno"))
		}})
	if err != nil {
		return err
	}
	// Start the receiver before any control wait. It also drains terminal events during cleanup.
	p.journal, err = os.OpenFile(filepath.Join(p.root, "node-events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		close(p.loopDone)
		return err
	}
	p.reader, err = nodehistory.Open(filepath.Join(p.root, "w", "n"))
	if err != nil {
		close(p.loopDone)
		return err
	}
	go p.receive()
	download := map[string]string{"url": p.services.PackageURL, "sha256": packageSHA}
	release := map[string]any{"kind": "targets", "targets": []any{map[string]any{"target": "x86_64-unknown-linux-gnu", "download": download}}}
	plugin := map[string]any{"plugin_id": pluginID, "version": pluginVersion, "release": release}
	input := map[string]any{"spec": map[string]any{"node_id": p.nodeID(), "plugins": []any{plugin}}}
	message, err := p.control(p.ctx, "i-"+p.driver.boot, "install_plugins", "plugins_result", input)
	if err != nil {
		return err
	}
	var installed struct {
		Kind   string
		Result struct {
			Items []struct{ Outcome struct{ Kind string } }
		}
	}
	if json.Unmarshal(message.Payload, &installed) != nil || installed.Kind != "plugins_completed" || len(installed.Result.Items) != 1 || installed.Result.Items[0].Outcome.Kind != "installed" {
		return execution.ErrUnavailable
	}
	return nil
}

func (p *projectRuntime) nodeID() string { return p.runtime.Client.Identity().NodeID }

func (p *projectRuntime) send(kind, id string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return execution.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()
	if err := p.runtime.Client.Send(ctx, node.Message{Type: kind, Version: 1, OperationID: "op-" + id, ExecutionID: id, Payload: data}); err != nil {
		return execution.ErrUnavailable
	}
	return nil
}

func (p *projectRuntime) control(ctx context.Context, id, command, expected string, payload any) (node.Message, error) {
	wait := &controlWait{kind: expected, done: make(chan node.Message, 1)}
	p.mu.Lock()
	p.controls[id] = wait
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.controls, id); p.mu.Unlock() }()
	if err := p.send(command, id, payload); err != nil {
		return node.Message{}, err
	}
	select {
	case message := <-wait.done:
		return message, nil
	case <-p.dead:
		return node.Message{}, execution.ErrUnavailable
	case <-ctx.Done():
		return node.Message{}, ctx.Err()
	}
}

func (p *projectRuntime) prepareCheckout(workspace string, config Project) (*checkout, error) {
	p.mu.Lock()
	entry := p.checkouts[workspace]
	if entry == nil {
		entry = &checkout{id: "c-" + p.driver.boot + "-" + digest(workspace), ready: make(chan struct{})}
		p.checkouts[workspace] = entry
		go func() {
			defer close(entry.ready)
			ref := digest(workspace)
			workspaceID := ref[:8] + "-" + ref[8:12] + "-4" + ref[13:16] + "-8" + ref[17:20] + "-" + ref[20:]
			message, err := p.control(p.ctx, entry.id, "clone_repository", "clone_result", map[string]any{"spec": map[string]any{"node_id": p.nodeID(), "repository_url": p.services.URL + "/repo.git", "snapshot_ref": config.SnapshotRef, "base_commit": config.Commit, "workspace_ref": "refs/heads/workspaces/" + workspaceID + "/main"}})
			if err != nil {
				entry.err = execution.ErrUnavailable
				return
			}
			var cloned struct {
				Kind   string
				Result struct{ Path, Commit string }
			}
			if json.Unmarshal(message.Payload, &cloned) != nil || cloned.Kind != "clone_ready" || cloned.Result.Commit != config.Commit {
				entry.err = execution.ErrUnavailable
				return
			}
			rel, err := filepath.Rel(filepath.Join(p.root, "w", "r"), cloned.Result.Path)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				entry.err = execution.ErrUnavailable
				return
			}
			entry.path = cloned.Result.Path
		}()
	}
	p.mu.Unlock()
	select {
	case <-entry.ready:
		return entry, entry.err
	case <-p.dead:
		return nil, execution.ErrUnavailable
	}
}

// Every received durable envelope is first fsynced. Agent events additionally require the
// caller's successful SQL publication before ACK; the journal alone never proves a turn complete.
func (p *projectRuntime) receive() {
	defer close(p.loopDone)
	defer p.transportFailed()
	for {
		message, err := p.runtime.Client.Receive(p.ctx)
		if err != nil {
			return
		}
		if message.Sequence != nil {
			if err := json.NewEncoder(p.journal).Encode(message); err != nil || p.journal.Sync() != nil {
				return
			}
			if last, exists := p.acknowledged[message.ExecutionID]; exists && *message.Sequence <= last {
				if p.runtime.Client.Ack(p.ctx, message) != nil {
					return
				}
				continue
			}
		}
		p.mu.Lock()
		agent, control := p.agents[message.ExecutionID], p.controls[message.ExecutionID]
		p.mu.Unlock()
		var outcome *turnOutcome
		ack := true
		if agent != nil {
			outcome, ack = agent.observe(message)
		}
		if message.Sequence != nil && ack {
			if p.runtime.Client.Ack(p.ctx, message) != nil {
				if outcome != nil {
					agent.complete(*outcome)
				}
				return
			}
			p.acknowledged[message.ExecutionID] = *message.Sequence
		}
		if outcome != nil {
			agent.complete(*outcome)
		}
		if control != nil && message.Type == control.kind {
			select {
			case control.done <- message:
			default:
			}
		}
	}
}

func (p *projectRuntime) transportFailed() {
	_ = p.runtime.Client.Close()
	p.mu.Lock()
	p.fatal = true
	agents := make([]*agentSession, 0, len(p.agents))
	for _, a := range p.agents {
		agents = append(agents, a)
	}
	p.mu.Unlock()
	p.deadOnce.Do(func() { close(p.dead) })
	for _, a := range agents {
		a.fail("agent_transport_closed", execution.ErrUnavailable)
	}
}

func (p *projectRuntime) close(ctx context.Context) error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	select {
	case <-p.ready:
	case <-ctx.Done():
		p.cancel()
		return execution.ErrStopUnconfirmed
	}
	if p.closed {
		return nil
	}
	if p.initErr != nil {
		if p.runtime != nil {
			if p.runtime.Stop(ctx) != nil {
				return execution.ErrStopUnconfirmed
			}
		} else if p.registered && node.RecoverRuntime(ctx, filepath.Join(p.root, "w"), 3*time.Second) != nil {
			return execution.ErrStopUnconfirmed
		}
		if p.services != nil && p.services.Close() != nil {
			return execution.ErrStopUnconfirmed
		}
		p.closed = true
		return nil
	}
	p.mu.Lock()
	agents := make([]*agentSession, 0, len(p.agents))
	for _, a := range p.agents {
		agents = append(agents, a)
	}
	p.mu.Unlock()
	for _, a := range agents {
		a.requestEnd()
	}
	// Stop supplies the final process-tree proof even when transport or cooperative End failed.
	if err := p.runtime.Stop(ctx); err != nil {
		return execution.ErrStopUnconfirmed
	}
	p.cancel()
	<-p.loopDone
	for _, a := range agents {
		a.markStopped()
	}
	if err := p.services.Close(); err != nil {
		return execution.ErrStopUnconfirmed
	}
	if err := errors.Join(p.reader.Close(), p.journal.Close()); err != nil {
		return execution.ErrStopUnconfirmed
	}
	p.closed = true
	return nil
}
