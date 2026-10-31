// Package executionruntime wires v2's single-process execution service to PostgreSQL and
// model/Agent drivers. A process-local mutex owns admission; there is no distributed scheduler.
package executionruntime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/modelturn"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/strictjson"
)

// AgentDriver owns project Nodes and their physical Agent sessions. Run may publish the
// terminal result through emit before returning, so durable Node events can be ACKed afterward.
type AgentDriver interface {
	Run(context.Context, execution.Scope, execution.Session, execution.Turn, func(context.Context, execution.Event) error) (execution.Result, error)
	End(context.Context, string) error
	Close(context.Context) error
}

// ModelChoice separates the public immutable selection from the private provider binding.
type ModelChoice struct {
	Model   execution.Model
	Binding nodeservices.Binding
}

// Config is trusted startup configuration. Agent may be nil for model-only operation.
type Config struct {
	Store  *store.Store
	Models map[string]ModelChoice
	Agent  AgentDriver
}

type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
	ref    execution.Ref
	scope  execution.Scope
	group  string
	err    error
}

// Service owns all execution workers. Close must finish before the database is closed.
type Service struct {
	db               *store.Store
	models           map[string]ModelChoice
	agent            AgentDriver
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	workers          map[string]*worker
	groups           map[string]string
	workspaces       map[string]string
	workspaceSession map[string]string
	ending           map[string]bool
	closed           bool
	failed           bool
}

var _ execution.Service = (*Service)(nil)
var _ execution.Catalog = (*Service)(nil)

// New closes old persisted sessions before admitting work. Node process recovery belongs to
// AgentDriver initialization; callers must construct that driver before exposing this service.
func New(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Store == nil || len(cfg.Models) == 0 {
		return nil, execution.ErrUnavailable
	}
	models := make(map[string]ModelChoice, len(cfg.Models))
	for id, choice := range cfg.Models {
		if id == "" || choice.Model.ID != id || choice.Model.Name == "" || choice.Model.BindingVersion == "" || choice.Binding.Provider == nil || (choice.Binding.Protocol != nodeservices.OpenAICompletions && choice.Binding.Protocol != nodeservices.AnthropicMessages) {
			return nil, execution.ErrInvalidRequest
		}
		models[id] = choice
	}
	if err := cfg.Store.RecoverExecutions(ctx); err != nil {
		return nil, err
	}
	owned, cancel := context.WithCancel(ctx)
	return &Service{db: cfg.Store, models: models, agent: cfg.Agent, ctx: owned, cancel: cancel,
		workers: map[string]*worker{}, groups: map[string]string{}, workspaces: map[string]string{}, workspaceSession: map[string]string{}, ending: map[string]bool{}}, nil
}

// Start validates the project and freezes a binding before recording the first Agent turn.
func (s *Service) Start(ctx context.Context, scope execution.Scope, req execution.StartRequest) (execution.Ref, error) {
	return s.start(ctx, scope, req.Caller, req.Prompt, &req.Target, req.ExecutorID, "")
}

// StartModel starts an independent model conversation without creating a directory or Node.
func (s *Service) StartModel(ctx context.Context, scope execution.Scope, req execution.ModelStartRequest) (execution.Ref, error) {
	return s.start(ctx, scope, req.Caller, req.Prompt, nil, "", req.ModelID)
}

func (s *Service) start(ctx context.Context, scope execution.Scope, caller execution.Caller, prompt execution.Prompt, target *execution.Target, executorID, modelID string) (execution.Ref, error) {
	if err := validate(caller, prompt); err != nil {
		return execution.Ref{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed || s.ctx.Err() != nil {
		return execution.Ref{}, execution.ErrUnavailable
	}
	if err := s.db.CheckExecutionScope(ctx, scope); err != nil {
		return execution.Ref{}, publicError(err)
	}
	// Retained attempts remain readable even when the original runtime has ended.
	if old, err := s.db.ReadExecutionAttempt(ctx, scope, caller); err == nil {
		session, e := s.db.ReadExecutionSession(ctx, scope, old.Ref.SessionID)
		if e != nil {
			return execution.Ref{}, publicError(e)
		}
		if !sameJSON(old.Prompt, prompt) || !reflect.DeepEqual(session.Target, target) || (target == nil && session.Model.ID != modelID) || (target != nil && (session.Executor == nil || session.Executor.ID != executorID)) {
			return execution.Ref{}, execution.ErrConflict
		}
		return old.Ref, nil
	} else if !errors.Is(err, execution.ErrNotFound) {
		return execution.Ref{}, publicError(err)
	}
	group := groupKey(scope, caller)
	if s.groups[group] != "" {
		return execution.Ref{}, execution.ErrWorkspaceBusy
	}
	var executor *execution.Executor
	if target != nil {
		if target.ProjectID == "" {
			return execution.Ref{}, execution.ErrInvalidRequest
		}
		if s.agent == nil {
			return execution.Ref{}, execution.ErrUnavailable
		}
		if _, err := s.db.GetProject(ctx, scope, target.ProjectID); err != nil {
			return execution.Ref{}, publicError(err)
		}
	search:
		for id, choice := range s.models {
			for _, a := range agentExecutors {
				if e, ok := executorFor(a, choice); ok && e.ID == executorID {
					modelID = id
					executor = &e
					break search
				}
			}
		}
		if executor == nil {
			return execution.Ref{}, execution.ErrNotFound
		}
	}
	choice, ok := s.models[modelID]
	// A direct model turn speaks OpenAI chat completions; Messages bindings serve Agents only.
	if !ok || (target == nil && choice.Binding.Protocol != nodeservices.OpenAICompletions) {
		return execution.Ref{}, execution.ErrNotFound
	}
	session := execution.Session{ID: newID(), Target: target, Executor: executor, Model: choice.Model, CreatedAt: time.Now().UTC()}
	if target != nil {
		session.WorkspaceID = newID()
		if caller.Kind == execution.CallerWorkflow {
			key := group + "/" + target.ProjectID
			if id := s.workspaces[key]; id != "" {
				session.WorkspaceID = id
			} else {
				s.workspaces[key] = session.WorkspaceID
			}
			// A workflow's next node may use a different Agent/model. End its idle predecessor
			// before lending that run's directory to a new logical session.
			if prior := s.workspaceSession[session.WorkspaceID]; prior != "" {
				if err := s.agent.End(ctx, prior); err != nil {
					return execution.Ref{}, execution.ErrStopUnconfirmed
				}
				if err := s.db.EndExecutionSession(ctx, scope, prior); err != nil {
					return execution.Ref{}, publicError(err)
				}
			}
		}
	}
	turn := execution.Turn{Ref: execution.Ref{SessionID: session.ID, TurnID: newID()}, Caller: caller, Prompt: copyPrompt(prompt), StartedAt: time.Now().UTC()}
	if err := s.db.CreateExecutionSession(ctx, scope, session, turn); err != nil {
		return execution.Ref{}, publicError(err)
	}
	if session.WorkspaceID != "" {
		s.workspaceSession[session.WorkspaceID] = session.ID
	}
	s.launch(scope, session, turn, group)
	return turn.Ref, nil
}

// Append continues a live logical session. A durable terminal event may precede driver cleanup;
// the next turn waits for that worker without releasing its directory or requiring another event.
// A server restart never reconstructs old workers.
func (s *Service) Append(ctx context.Context, scope execution.Scope, req execution.AppendRequest) (execution.Ref, error) {
	if err := validate(req.Caller, req.Prompt); err != nil {
		return execution.Ref{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.closed || s.failed || s.ctx.Err() != nil {
			return execution.Ref{}, execution.ErrUnavailable
		}
		session, err := s.db.ReadExecutionSession(ctx, scope, req.SessionID)
		if err != nil {
			return execution.Ref{}, publicError(err)
		}
		if old, e := s.db.ReadExecutionAttempt(ctx, scope, req.Caller); e == nil {
			if old.Ref.SessionID != req.SessionID || !sameJSON(old.Prompt, req.Prompt) {
				return execution.Ref{}, execution.ErrConflict
			}
			return old.Ref, nil
		} else if !errors.Is(e, execution.ErrNotFound) {
			return execution.Ref{}, publicError(e)
		}
		if session.EndedAt != nil || s.ending[session.ID] {
			return execution.Ref{}, execution.ErrSessionEnded
		}
		first, err := s.db.ReadExecutionTurns(ctx, scope, session.ID, "", 1)
		if err != nil {
			return execution.Ref{}, publicError(err)
		}
		if len(first.Items) != 1 || first.Items[0].Caller.Kind != req.Caller.Kind || first.Items[0].Caller.ID != req.Caller.ID {
			return execution.Ref{}, execution.ErrConflict
		}
		group := groupKey(scope, req.Caller)
		if active := s.groups[group]; active != "" {
			prior := s.workers[active]
			if prior == nil {
				return execution.Ref{}, execution.ErrWorkspaceBusy
			}
			retained, err := s.readTurn(ctx, scope, prior.ref)
			if err != nil {
				return execution.Ref{}, err
			}
			if retained.Result == nil {
				return execution.Ref{}, execution.ErrWorkspaceBusy
			}
			// Publication precedes the Node ACK and worker cleanup. Keep ownership
			// intact, but do not tell an observer of that terminal event to wait for
			// a second event that will never arrive. Cleanup itself needs this mutex.
			s.mu.Unlock()
			var waitErr error
			select {
			case <-ctx.Done():
				waitErr = ctx.Err()
			case <-s.ctx.Done():
				waitErr = execution.ErrUnavailable
			case <-prior.done:
			}
			s.mu.Lock()
			if waitErr != nil {
				return execution.Ref{}, waitErr
			}
			// Another Append, End, Close or permission change may have won while
			// unlocked. Recheck admission and attempt identity before writing anything.
			continue
		}
		turn := execution.Turn{Ref: execution.Ref{SessionID: session.ID, TurnID: newID()}, Caller: req.Caller, Prompt: copyPrompt(req.Prompt), StartedAt: time.Now().UTC()}
		if err := s.db.AppendExecutionTurn(ctx, scope, session.ID, turn); err != nil {
			return execution.Ref{}, publicError(err)
		}
		s.launch(scope, session, turn, group)
		return turn.Ref, nil
	}
}

// launch is called under mu; workers use the backend's lifetime, never an HTTP request context.
func (s *Service) launch(scope execution.Scope, session execution.Session, turn execution.Turn, group string) {
	ctx, cancel := context.WithCancel(s.ctx)
	w := &worker{cancel: cancel, done: make(chan struct{}), ref: turn.Ref, scope: scope, group: group}
	s.workers[turn.Ref.TurnID] = w
	s.groups[group] = turn.Ref.TurnID
	go func() {
		defer cancel()
		emit := func(_ context.Context, event execution.Event) error {
			write, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			event.Ref = turn.Ref
			if event.Kind == execution.EventTurnEnded {
				if event.Result == nil {
					return execution.ErrInvalidRequest
				}
				_, err := s.db.FinishExecutionTurn(write, scope, turn.Ref, *event.Result)
				return err
			}
			_, err := s.db.AppendExecutionEvent(write, scope, event)
			return err
		}
		var result execution.Result
		var agentErr error
		if session.Target != nil {
			result, agentErr = s.agent.Run(ctx, scope, session, turn, emit)
			if agentErr != nil && result.Status == "" {
				result = failedResult(ctx, agentErr)
			}
		} else {
			messages, err := s.messages(ctx, scope, turn)
			if err != nil {
				result = failedResult(ctx, err)
			} else {
				result, _ = modelturn.Run(ctx, s.models[session.Model.ID].Binding, messages, func(c context.Context, delta string) error {
					return emit(c, execution.Event{Kind: execution.EventTextDelta, TextDelta: delta})
				})
			}
		}
		if result.Status == "" {
			result = failedResult(ctx, execution.ErrUnavailable)
		}
		write, stop := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := s.db.FinishExecutionTurn(write, scope, turn.Ref, result)
		stop()
		if session.Target != nil && (agentErr != nil || result.Status != execution.StatusCompleted) {
			// A failed/cancelled physical Agent is never advertised as resumable. Keep
			// admission occupied until stop is confirmed, even if its result is already stored.
			s.mu.Lock()
			s.ending[session.ID] = true
			s.mu.Unlock()
			finish, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if endErr := s.agent.End(finish, session.ID); endErr != nil {
				err = errors.Join(err, execution.ErrStopUnconfirmed)
			} else {
				err = errors.Join(err, s.db.EndExecutionSession(finish, scope, session.ID))
			}
			cancel()
		}
		s.mu.Lock()
		w.err = err
		if err != nil {
			s.failed = true
		}
		delete(s.groups, group)
		delete(s.workers, turn.Ref.TurnID)
		close(w.done)
		s.mu.Unlock()
	}()
}

func (s *Service) messages(ctx context.Context, scope execution.Scope, current execution.Turn) ([]modelturn.Message, error) {
	var messages []modelturn.Message
	cursor := ""
	for {
		page, err := s.db.ReadExecutionTurns(ctx, scope, current.Ref.SessionID, cursor, 100)
		if err != nil {
			return nil, err
		}
		for _, turn := range page.Items {
			text, err := turn.Prompt.Text()
			if err != nil {
				return nil, err
			}
			messages = append(messages, modelturn.Message{Role: "user", Content: text})
			if turn.Ref.TurnID == current.Ref.TurnID {
				return messages, nil
			}
			if turn.Result != nil {
				messages = append(messages, modelturn.Message{Role: "assistant", Content: turn.Result.Answer})
			}
		}
		if page.NextCursor == "" {
			return nil, execution.ErrNotFound
		}
		cursor = page.NextCursor
	}
}

// Cancel ends an Agent session because the pinned Node has only EndSession cancellation.
// Model-only cancellation stops the requested turn and leaves its conversation open.
func (s *Service) Cancel(ctx context.Context, scope execution.Scope, ref execution.Ref) (execution.Result, error) {
	turn, err := s.readTurn(ctx, scope, ref)
	if err != nil {
		return execution.Result{}, err
	}
	if turn.Result != nil {
		return *turn.Result, nil
	}
	session, err := s.db.ReadExecutionSession(ctx, scope, ref.SessionID)
	if err != nil {
		return execution.Result{}, publicError(err)
	}
	if session.Target != nil {
		// Serialize with Append: a late retry for an already finished turn must not
		// stop a newer turn. Once cancellation wins, no new turn can enter this session.
		s.mu.Lock()
		turn, err = s.readTurn(ctx, scope, ref)
		if err == nil && turn.Result == nil {
			s.ending[ref.SessionID] = true
		}
		s.mu.Unlock()
		if err != nil {
			return execution.Result{}, err
		}
		if turn.Result != nil {
			return *turn.Result, nil
		}
		if err := s.End(ctx, scope, ref.SessionID); err != nil {
			return execution.Result{}, err
		}
		turn, err = s.readTurn(ctx, scope, ref)
		if err != nil {
			return execution.Result{}, err
		}
		if turn.Result == nil {
			return execution.Result{}, execution.ErrStopUnconfirmed
		}
		return *turn.Result, nil
	}
	s.mu.Lock()
	w := s.workers[ref.TurnID]
	if w != nil {
		w.cancel()
	}
	s.mu.Unlock()
	if w == nil {
		// Completion can win after the initial read and remove the worker. Its durable
		// result is still the authority; absence from the map is not a stop failure.
		turn, err = s.readTurn(ctx, scope, ref)
		if err != nil {
			return execution.Result{}, err
		}
		if turn.Result != nil {
			return *turn.Result, nil
		}
		return execution.Result{}, execution.ErrStopUnconfirmed
	}
	select {
	case <-ctx.Done():
		return execution.Result{}, ctx.Err()
	case <-w.done:
	}
	turn, err = s.readTurn(ctx, scope, ref)
	if err != nil {
		return execution.Result{}, err
	}
	if turn.Result == nil {
		return execution.Result{}, execution.ErrStopUnconfirmed
	}
	return *turn.Result, nil
}

// End stops and joins the active worker before persisting the session terminal event.
func (s *Service) End(ctx context.Context, scope execution.Scope, sid string) error {
	session, err := s.db.ReadExecutionSession(ctx, scope, sid)
	if err != nil {
		return publicError(err)
	}
	if session.EndedAt != nil {
		return nil
	}
	s.mu.Lock()
	s.ending[sid] = true
	var wait []*worker
	for _, w := range s.workers {
		if w.ref.SessionID == sid {
			w.cancel()
			wait = append(wait, w)
		}
	}
	s.mu.Unlock()
	for _, w := range wait {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
		}
		if w.err != nil {
			return execution.ErrStopUnconfirmed
		}
	}
	if session.Target != nil {
		if err := s.agent.End(ctx, sid); err != nil {
			return execution.ErrStopUnconfirmed
		}
	}
	return publicError(s.db.EndExecutionSession(ctx, scope, sid))
}

// Close shuts down workers and physical Agents. Persisted sessions are ended by recovery on
// the next start, so no old conversation is advertised as resumable across process restarts.
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	var wait []*worker
	for _, w := range s.workers {
		wait = append(wait, w)
	}
	s.mu.Unlock()
	for _, w := range wait {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
		}
	}
	if s.agent != nil {
		return s.agent.Close(ctx)
	}
	return nil
}

func (s *Service) ReadSession(ctx context.Context, scope execution.Scope, id string) (execution.Session, error) {
	v, e := s.db.ReadExecutionSession(ctx, scope, id)
	return v, publicError(e)
}
func (s *Service) ReadTurns(ctx context.Context, scope execution.Scope, id, cursor string, limit int) (execution.TurnPage, error) {
	v, e := s.db.ReadExecutionTurns(ctx, scope, id, cursor, limit)
	return v, publicError(e)
}
func (s *Service) ReadAttempt(ctx context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
	v, e := s.db.ReadExecutionAttempt(ctx, scope, caller)
	return v, publicError(e)
}
func (s *Service) readTurn(ctx context.Context, scope execution.Scope, ref execution.Ref) (execution.Turn, error) {
	cursor := ""
	for {
		p, err := s.ReadTurns(ctx, scope, ref.SessionID, cursor, 100)
		if err != nil {
			return execution.Turn{}, err
		}
		for _, t := range p.Items {
			if t.Ref == ref {
				return t, nil
			}
		}
		if p.NextCursor == "" {
			return execution.Turn{}, execution.ErrNotFound
		}
		cursor = p.NextCursor
	}
}

func (s *Service) ListModels(ctx context.Context, scope execution.Scope) ([]execution.Model, error) {
	if err := s.db.CheckExecutionScope(ctx, scope); err != nil {
		return nil, publicError(err)
	}
	// Consumers use the catalog as an execution-readiness gate, so an unusable
	// runtime must not keep advertising otherwise valid deployment selections.
	s.mu.Lock()
	unavailable := s.closed || s.failed || s.ctx.Err() != nil
	s.mu.Unlock()
	if unavailable {
		return nil, execution.ErrUnavailable
	}
	items := make([]execution.Model, 0, len(s.models))
	for _, c := range s.models {
		if c.Binding.Protocol == nodeservices.OpenAICompletions {
			items = append(items, c.Model)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
func (s *Service) ListExecutors(ctx context.Context, scope execution.Scope) ([]execution.Executor, error) {
	// ListModels supplies the scope and readiness checks; Messages bindings are not in it.
	if _, err := s.ListModels(ctx, scope); err != nil {
		return nil, err
	}
	items := []execution.Executor{}
	if s.agent == nil {
		return items, nil
	}
	ids := slices.Sorted(maps.Keys(s.models))
	// OpenCode stays first, so pages that preselect the first executor keep their default.
	for _, a := range agentExecutors {
		for _, id := range ids {
			if e, ok := executorFor(a, s.models[id]); ok {
				items = append(items, e)
			}
		}
	}
	return items, nil
}
func groupKey(scope execution.Scope, caller execution.Caller) string {
	return scope.TenantID + "/" + scope.UserID + "/" + string(caller.Kind) + "/" + caller.ID
}
func validate(c execution.Caller, p execution.Prompt) error {
	if c.ID == "" || c.AttemptID == "" || (c.Kind != execution.CallerChat && c.Kind != execution.CallerWorkflow && c.Kind != execution.CallerSchedule) {
		return execution.ErrInvalidRequest
	}
	if c.Kind != execution.CallerWorkflow && (c.NodeID != "" || len(c.Rounds) != 0) {
		return execution.ErrInvalidRequest
	}
	for _, r := range c.Rounds {
		if r.ContainerID == "" || r.Index < 0 {
			return execution.ErrInvalidRequest
		}
	}
	size := 0
	for _, raw := range p {
		size += len(raw)
		if size > 1<<20 || !strictjson.Valid(raw) {
			return execution.ErrInvalidRequest
		}
	}
	_, err := p.Text()
	return err
}
func copyPrompt(p execution.Prompt) execution.Prompt {
	v := make(execution.Prompt, len(p))
	for i, b := range p {
		v[i] = append(json.RawMessage(nil), b...)
	}
	return v
}
func sameJSON(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	var x, y any
	if json.Unmarshal(aa, &x) != nil || json.Unmarshal(bb, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func failedResult(ctx context.Context, err error) execution.Result {
	if errors.Is(ctx.Err(), context.Canceled) {
		return execution.Result{Status: execution.StatusCancelled, StopReason: "cancelled"}
	}
	return execution.Result{Status: execution.StatusFailed, FailureCode: "execution_failed"}
}
func publicError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, execution.ErrNotFound, execution.ErrInvalidRequest, execution.ErrConflict, execution.ErrWorkspaceBusy, execution.ErrSessionEnded, execution.ErrStopUnconfirmed} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, store.ErrNotFound) {
		return execution.ErrNotFound
	}
	return execution.ErrUnavailable
}
