package executionruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type runtimeFixture struct {
	ctx     context.Context
	db      *store.Store
	service *Service
	config  *pgxpool.Config
	models  map[string]ModelChoice
	scope   execution.Scope
}

func runtimeDatabase(t *testing.T, provider nodeservices.Provider) *runtimeFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	schema := "runtime_" + hex.EncodeToString(id[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	f := &runtimeFixture{ctx: ctx}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if f.service != nil {
			if err := f.service.Close(cleanup); err != nil {
				t.Error(err)
			}
		}
		if f.db != nil {
			f.db.Close()
		}
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(cleanup)
	})
	f.config, err = pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	f.config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	f.db, err = store.OpenConfig(ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	f.scope = runtimeMember(t, f, "owner")
	f.models, err = ModelsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if provider != nil {
		choice := f.models["fixture"]
		choice.Binding.Provider = provider
		f.models["fixture"] = choice
	}
	f.service, err = New(ctx, Config{Store: f.db, Models: f.models})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func runtimeMember(t *testing.T, f *runtimeFixture, subject string) execution.Scope {
	t.Helper()
	token, err := f.db.Login(f.ctx, store.Identity{Source: "test", Subject: subject, DisplayName: subject}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.db.SessionUser(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := f.db.Tenants(f.ctx, u.ID)
	if err != nil || len(tenants) != 1 {
		t.Fatalf("tenants=%v err=%v", tenants, err)
	}
	return execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
}

func modelRequest(conversation, attempt, prompt string) execution.ModelStartRequest {
	return execution.ModelStartRequest{Caller: execution.Caller{Kind: execution.CallerChat, ID: conversation, AttemptID: attempt}, ModelID: "fixture", Prompt: execution.TextPrompt(prompt)}
}

func startModel(t *testing.T, f *runtimeFixture, req execution.ModelStartRequest) execution.Ref {
	t.Helper()
	ref, err := f.service.StartModel(f.ctx, f.scope, req)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func observeTurn(t *testing.T, f *runtimeFixture, ref execution.Ref, after uint64) (execution.Result, uint64) {
	t.Helper()
	stream, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, after)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for {
		event, err := stream.Next(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.Ref == ref && event.Kind == execution.EventTurnEnded {
			if event.Result == nil {
				t.Fatal("terminal event lacks result")
			}
			// A terminal Agent event may precede driver quiescence. Join the worker before
			// asking this helper to admit the next turn; production callers may receive Busy.
			f.service.mu.Lock()
			w := f.service.workers[ref.TurnID]
			f.service.mu.Unlock()
			if w != nil {
				select {
				case <-w.done:
				case <-f.ctx.Done():
					t.Fatal(f.ctx.Err())
				}
			}
			return *event.Result, event.Sequence
		}
	}
}

func TestRuntimeCatalogAdvertisesOnlyAUsableService(t *testing.T) {
	f := runtimeDatabase(t, nil)
	models, err := f.service.ListModels(f.ctx, f.scope)
	if err != nil || len(models) != 1 || models[0].ID != "fixture" {
		t.Fatalf("model-only deployment lacks fixture: %+v %v", models, err)
	}
	executors, err := f.service.ListExecutors(f.ctx, f.scope)
	if err != nil || len(executors) != 0 {
		t.Fatalf("model-only deployment advertised an Agent: %+v %v", executors, err)
	}
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ListModels(f.ctx, f.scope); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatalf("closed runtime advertised model readiness: %v", err)
	}
	if _, err := f.service.ListExecutors(f.ctx, f.scope); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatalf("closed runtime advertised Agent readiness: %v", err)
	}
}

func TestRuntimeFixtureAppendReplayEndAndReopen(t *testing.T) {
	var calls atomic.Int32
	provider := runtimeProvider(func(w http.ResponseWriter, r *http.Request, m nodeservices.Model) {
		calls.Add(1)
		nodeservices.FixtureProvider{}.Complete(w, r, m)
	})
	f := runtimeDatabase(t, provider)
	req := modelRequest("conversation", "first", "你好🙂")
	ref := startModel(t, f, req)
	result, firstCursor := observeTurn(t, f, ref, 0)
	if result.Status != execution.StatusCompleted || !result.AnswerComplete || result.Answer != "V2 fixture response." || result.Usage != nil {
		t.Fatalf("result=%+v", result)
	}
	// JSONB may normalize key ordering; equivalent content must still return the same attempt.
	retry := req
	retry.Prompt = execution.Prompt{json.RawMessage(`{"text":"你好🙂","type":"text"}`)}
	if got, err := f.service.StartModel(f.ctx, f.scope, retry); err != nil || got != ref {
		t.Fatalf("retry=%v err=%v", got, err)
	}
	changed := req
	changed.Prompt = execution.TextPrompt("changed")
	if _, err := f.service.StartModel(f.ctx, f.scope, changed); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("changed prompt=%v", err)
	}
	appendReq := execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("conversation", "second", "").Caller, Prompt: execution.TextPrompt("第二问")}
	second, err := f.service.Append(f.ctx, f.scope, appendReq)
	if err != nil {
		t.Fatal(err)
	}
	_, secondCursor := observeTurn(t, f, second, firstCursor)
	if got, err := f.service.Append(f.ctx, f.scope, appendReq); err != nil || got != second {
		t.Fatalf("append retry=%v err=%v", got, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("idempotent calls=%d", calls.Load())
	}
	if _, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, secondCursor+1); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("ahead cursor=%v", err)
	}
	for range 2 {
		if err := f.service.End(f.ctx, f.scope, ref.SessionID); err != nil {
			t.Fatal(err)
		}
	}
	stream, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var events []execution.Event
	for {
		event, err := stream.Next(f.ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) < 5 || events[len(events)-1].Kind != execution.EventSessionEnded {
		t.Fatalf("missing session terminal: %+v", events)
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("sequence gap: %+v", events)
		}
	}
	endCursor := events[len(events)-1].Sequence
	acknowledged, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, endCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer acknowledged.Close()
	if _, err := acknowledged.Next(f.ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("acknowledged EOF=%v", err)
	}
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.db.Close()
	f.db, err = store.OpenConfig(f.ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := f.service.ReadAttempt(f.ctx, f.scope, req.Caller)
	if err != nil || turn.Result == nil || !reflect.DeepEqual(*turn.Result, result) {
		t.Fatalf("reopened turn=%+v err=%v", turn, err)
	}
	page, err := f.service.ReadTurns(f.ctx, f.scope, ref.SessionID, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	appendReq.Caller.AttemptID = "after-restart"
	if _, err := f.service.Append(f.ctx, f.scope, appendReq); !errors.Is(err, execution.ErrSessionEnded) {
		t.Fatalf("append ended=%v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("restart replayed model: %d", calls.Load())
	}
}

func TestRuntimeStartupInterruptsOldLiveSession(t *testing.T) {
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	caller := modelRequest("crashed", "attempt", "").Caller
	ref := execution.Ref{SessionID: "old-session", TurnID: "old-turn"}
	if err := f.db.CreateExecutionSession(f.ctx, f.scope, execution.Session{ID: ref.SessionID, Model: f.models["fixture"].Model}, execution.Turn{Ref: ref, Caller: caller, Prompt: execution.TextPrompt("old prompt")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.AppendExecutionEvent(f.ctx, f.scope, execution.Event{Ref: ref, Kind: execution.EventTextDelta, TextDelta: "partial"}); err != nil {
		t.Fatal(err)
	}
	var err error
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := f.service.ReadAttempt(f.ctx, f.scope, caller)
	if err != nil || turn.Result == nil || turn.Result.Status != execution.StatusInterrupted || turn.Result.Answer != "partial" {
		t.Fatalf("recovery=%+v err=%v", turn, err)
	}
	session, err := f.service.ReadSession(f.ctx, f.scope, ref.SessionID)
	if err != nil || session.EndedAt == nil {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	caller.AttemptID = "next"
	if _, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: caller, Prompt: execution.TextPrompt("next")}); !errors.Is(err, execution.ErrSessionEnded) {
		t.Fatalf("recovered append=%v", err)
	}
}

type runtimeProvider func(http.ResponseWriter, *http.Request, nodeservices.Model)

func (p runtimeProvider) Complete(w http.ResponseWriter, r *http.Request, m nodeservices.Model) {
	p(w, r, m)
}

type providerCall struct {
	ctx                        context.Context
	release, cancelled, exited chan struct{}
	once                       sync.Once
}

func (c *providerCall) unblock() { c.once.Do(func() { close(c.release) }) }

type blockingProvider struct{ started chan *providerCall }

func (p *blockingProvider) Complete(w http.ResponseWriter, r *http.Request, _ nodeservices.Model) {
	c := &providerCall{ctx: r.Context(), release: make(chan struct{}), cancelled: make(chan struct{}), exited: make(chan struct{})}
	defer close(c.exited)
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
	p.started <- c
	select {
	case <-r.Context().Done():
		close(c.cancelled)
		<-c.release
	case <-c.release:
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}
}
func nextProviderCall(t *testing.T, f *runtimeFixture, p *blockingProvider) *providerCall {
	t.Helper()
	select {
	case c := <-p.started:
		t.Cleanup(c.unblock)
		return c
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
		return nil
	}
}

func TestRuntimeObserverCloseAndAdmissionContextDoNotCancel(t *testing.T) {
	p := &blockingProvider{started: make(chan *providerCall, 4)}
	f := runtimeDatabase(t, p)
	request, stop := context.WithCancel(f.ctx)
	ref, err := f.service.StartModel(request, f.scope, modelRequest("chat", "first", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	stop()
	call := nextProviderCall(t, f, p)
	stream, err := f.service.Subscribe(f.ctx, f.scope, ref.SessionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	observed := make(chan error, 1)
	go func() { _, err := stream.Next(f.ctx); observed <- err }()
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("closed observer=%v", err)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	if call.ctx.Err() != nil {
		t.Fatalf("observer/request cancelled execution: %v", call.ctx.Err())
	}
	call.unblock()
	result, _ := observeTurn(t, f, ref, 0)
	if result.Status != execution.StatusCompleted || result.Answer != "partial" {
		t.Fatalf("result=%+v", result)
	}
}

func TestRuntimeBusyAndIndependentConversations(t *testing.T) {
	p := &blockingProvider{started: make(chan *providerCall, 8)}
	f := runtimeDatabase(t, p)
	ref := startModel(t, f, modelRequest("chat-a", "first", "hello"))
	first := nextProviderCall(t, f, p)
	if _, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("chat-a", "append", "").Caller, Prompt: execution.TextPrompt("next")}); !errors.Is(err, execution.ErrWorkspaceBusy) {
		t.Fatalf("active session append=%v", err)
	}
	secondRef := startModel(t, f, modelRequest("chat-b", "parallel", "hello"))
	second := nextProviderCall(t, f, p)
	workflow := modelRequest("workflow", "node-one", "hello")
	workflow.Caller.Kind = execution.CallerWorkflow
	workflow.Caller.NodeID = "one"
	workflowRef := startModel(t, f, workflow)
	third := nextProviderCall(t, f, p)
	workflow.Caller.AttemptID = "node-two"
	workflow.Caller.NodeID = "two"
	if _, err := f.service.StartModel(f.ctx, f.scope, workflow); !errors.Is(err, execution.ErrWorkspaceBusy) {
		t.Fatalf("same workflow run=%v", err)
	}
	for _, call := range []*providerCall{first, second, third} {
		if call.ctx.Err() != nil {
			t.Fatalf("independent execution cancelled: %v", call.ctx.Err())
		}
		call.unblock()
	}
	for _, ref := range []execution.Ref{ref, secondRef, workflowRef} {
		result, _ := observeTurn(t, f, ref, 0)
		if result.Status != execution.StatusCompleted {
			t.Fatalf("result=%+v", result)
		}
	}
}

func TestRuntimeModelCancelAndEndWaitForProviderExit(t *testing.T) {
	p := &blockingProvider{started: make(chan *providerCall, 4)}
	f := runtimeDatabase(t, p)
	req := modelRequest("chat", "cancel", "hello")
	ref := startModel(t, f, req)
	first := nextProviderCall(t, f, p)
	type cancellation struct {
		result execution.Result
		err    error
	}
	cancelled := make(chan cancellation, 1)
	go func() { result, err := f.service.Cancel(f.ctx, f.scope, ref); cancelled <- cancellation{result, err} }()
	select {
	case <-first.cancelled:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	select {
	case got := <-cancelled:
		t.Fatalf("cancel returned before provider exit: %+v", got)
	default:
	}
	first.unblock()
	select {
	case got := <-cancelled:
		if got.err != nil || got.result.Status != execution.StatusCancelled {
			t.Fatalf("cancel=%+v", got)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	second, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: modelRequest("chat", "after-cancel", "").Caller, Prompt: execution.TextPrompt("still live")})
	if err != nil {
		t.Fatal(err)
	}
	active := nextProviderCall(t, f, p)
	// Cancelling an old terminal turn must not cancel a newer append.
	if result, err := f.service.Cancel(f.ctx, f.scope, ref); err != nil || result.Status != execution.StatusCancelled {
		t.Fatalf("old cancel=%+v err=%v", result, err)
	}
	if active.ctx.Err() != nil {
		t.Fatal("old turn cancelled the newer turn")
	}
	ended := make(chan error, 1)
	go func() { ended <- f.service.End(f.ctx, f.scope, second.SessionID) }()
	select {
	case <-active.cancelled:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	select {
	case err := <-ended:
		t.Fatalf("End returned before provider exit: %v", err)
	default:
	}
	active.unblock()
	select {
	case err := <-ended:
		if err != nil {
			t.Fatal(err)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	session, err := f.service.ReadSession(f.ctx, f.scope, ref.SessionID)
	if err != nil || session.EndedAt == nil {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	result, _ := observeTurn(t, f, second, 0)
	if result.Status != execution.StatusCancelled {
		t.Fatalf("End result=%+v", result)
	}
}

func TestRuntimeScopeIsolation(t *testing.T) {
	f := runtimeDatabase(t, nil)
	req := modelRequest("private", "first", "hello")
	ref := startModel(t, f, req)
	observeTurn(t, f, ref, 0)
	other := runtimeMember(t, f, "other")
	checks := []func() error{
		func() error { _, err := f.service.ReadSession(f.ctx, other, ref.SessionID); return err },
		func() error { _, err := f.service.ReadTurns(f.ctx, other, ref.SessionID, "", 10); return err },
		func() error { _, err := f.service.ReadAttempt(f.ctx, other, req.Caller); return err },
		func() error { _, err := f.service.Subscribe(f.ctx, other, ref.SessionID, 0); return err },
		func() error { _, err := f.service.Cancel(f.ctx, other, ref); return err },
		func() error { return f.service.End(f.ctx, other, ref.SessionID) },
		func() error {
			_, err := f.service.Append(f.ctx, other, execution.AppendRequest{SessionID: ref.SessionID, Caller: req.Caller, Prompt: req.Prompt})
			return err
		},
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, execution.ErrNotFound) {
			t.Fatalf("scope check %d=%v", i, err)
		}
	}
	if _, err := f.service.ListModels(f.ctx, execution.Scope{TenantID: f.scope.TenantID, UserID: "00000000-0000-0000-0000-000000000001"}); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("untrusted catalog scope=%v", err)
	}
	if _, err := f.service.ListModels(f.ctx, execution.Scope{TenantID: f.scope.TenantID, UserID: "missing"}); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("malformed catalog scope=%v", err)
	}
}

// orchestrationAgent exercises only runtime ownership and SQL publication. It does not
// launch Node, clone repositories, run Agent tools, or constitute real-Node evidence.
type orchestrationAgent struct {
	started chan *agentCall
	ended   chan string
	mu      sync.Mutex
	closed  map[string]bool
}

type agentCall struct {
	ctx     context.Context
	session execution.Session
	release chan struct{}
	once    sync.Once
}

func (c *agentCall) unblock() { c.once.Do(func() { close(c.release) }) }

func (a *orchestrationAgent) Run(ctx context.Context, _ execution.Scope, session execution.Session, _ execution.Turn, emit func(context.Context, execution.Event) error) (execution.Result, error) {
	call := &agentCall{ctx: ctx, session: session, release: make(chan struct{})}
	a.started <- call
	select {
	case <-ctx.Done():
		return execution.Result{Status: execution.StatusCancelled, StopReason: "cancelled"}, nil
	case <-call.release:
		result := execution.Result{Status: execution.StatusCompleted, Answer: "fake driver orchestration", AnswerComplete: true, StopReason: "end_turn"}
		// Real Node drivers must retain this before ACK. The runtime's later Finish must
		// not create a duplicate terminal event or replace the already-retained result.
		err := emit(ctx, execution.Event{Kind: execution.EventTurnEnded, Result: &result})
		return result, err
	}
}

func (a *orchestrationAgent) End(_ context.Context, sid string) error {
	// The worker and an explicit End/Cancel may both confirm the same physical stop.
	// Record its effect once, as the real driver's idempotent session stop does.
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed == nil {
		a.closed = map[string]bool{}
	}
	if !a.closed[sid] {
		a.closed[sid] = true
		a.ended <- sid
	}
	return nil
}
func (a *orchestrationAgent) Close(context.Context) error { return nil }

func nextAgentCall(t *testing.T, f *runtimeFixture, a *orchestrationAgent) *agentCall {
	t.Helper()
	select {
	case call := <-a.started:
		t.Cleanup(call.unblock)
		return call
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
		return nil
	}
}

// Each Agent runs only the bindings of its protocol: Claude Code gets the Messages bindings under
// the same slots OpenCode uses, and a direct model turn can never select a Messages binding.
func TestRuntimeExecutorDirectoryPairsAgentsWithTheirProtocol(t *testing.T) {
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	a := &orchestrationAgent{started: make(chan *agentCall, 8), ended: make(chan string, 8)}
	var err error
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models, Agent: a})
	if err != nil {
		t.Fatal(err)
	}
	executors, err := f.service.ListExecutors(f.ctx, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	want := []execution.Executor{
		{ID: "opencode:fixture", Name: "OpenCode / Fixture", AgentID: "official/ora-space.opencode", AgentName: "OpenCode", PluginVersion: "0.6.11", Model: f.models["fixture"].Model},
		{ID: "claude:fixture", Name: "Claude Code / Fixture", AgentID: "official/ora-space.claude", AgentName: "Claude Code", PluginVersion: "0.4.5", Model: f.models["messages-fixture"].Model},
	}
	if !reflect.DeepEqual(executors, want) {
		t.Fatalf("executors=%+v\nwant %+v", executors, want)
	}
	if models, err := f.service.ListModels(f.ctx, f.scope); err != nil || len(models) != 1 || models[0].ID != "fixture" {
		t.Fatalf("direct model catalog exposed a Messages binding: %+v %v", models, err)
	}
	direct := modelRequest("direct", "first", "hello")
	direct.ModelID = "messages-fixture"
	if _, err := f.service.StartModel(f.ctx, f.scope, direct); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("direct model turn selected a Messages binding: %v", err)
	}
	if _, err := f.db.CreateProject(f.ctx, f.scope, store.Project{ID: "project", Name: "Demo", RepositoryURL: "https://example.com/demo.git", Commit: "fixed", MirrorPath: "/fake/unused"}); err != nil {
		t.Fatal(err)
	}
	ref, err := f.service.Start(f.ctx, f.scope, execution.StartRequest{Caller: modelRequest("claude", "first", "").Caller, Target: execution.Target{ProjectID: "project"}, ExecutorID: "claude:fixture", Prompt: execution.TextPrompt("run")})
	if err != nil {
		t.Fatal(err)
	}
	call := nextAgentCall(t, f, a)
	if call.session.ID != ref.SessionID || call.session.Executor == nil || !reflect.DeepEqual(*call.session.Executor, want[1]) {
		t.Fatalf("Claude session executor=%+v", call.session.Executor)
	}
	call.unblock()
}

func TestRuntimeAgentOrchestrationSeparatesConversationsAndReusesWorkflowDirectory(t *testing.T) {
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	a := &orchestrationAgent{started: make(chan *agentCall, 8), ended: make(chan string, 8)}
	var err error
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models, Agent: a})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.CreateProject(f.ctx, f.scope, store.Project{ID: "project", Name: "Demo", RepositoryURL: "https://example.com/demo.git", Commit: "fixed", MirrorPath: "/fake/unused"}); err != nil {
		t.Fatal(err)
	}
	start := func(caller execution.Caller) execution.Ref {
		t.Helper()
		ref, err := f.service.Start(f.ctx, f.scope, execution.StartRequest{Caller: caller, Target: execution.Target{ProjectID: "project"}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("run")})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	first := start(modelRequest("chat-one", "first", "").Caller)
	firstCall := nextAgentCall(t, f, a)
	second := start(modelRequest("chat-two", "first", "").Caller)
	secondCall := nextAgentCall(t, f, a)
	if firstCall.session.WorkspaceID == "" || firstCall.session.WorkspaceID == secondCall.session.WorkspaceID {
		t.Fatal("different conversations share an execution directory")
	}
	if err := f.service.End(f.ctx, f.scope, first.SessionID); err != nil {
		t.Fatal(err)
	}
	select {
	case sid := <-a.ended:
		if sid != first.SessionID {
			t.Fatalf("ended other session: %s", sid)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	if secondCall.ctx.Err() != nil {
		t.Fatal("ending one conversation stopped another")
	}
	secondCall.unblock()
	observeTurn(t, f, second, 0)
	workflow := execution.Caller{Kind: execution.CallerWorkflow, ID: "workflow", NodeID: "one", AttemptID: "node-one"}
	old := start(workflow)
	oldCall := nextAgentCall(t, f, a)
	workflow.NodeID, workflow.AttemptID = "two", "node-two"
	if _, err := f.service.Start(f.ctx, f.scope, execution.StartRequest{Caller: workflow, Target: execution.Target{ProjectID: "project"}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("run")}); !errors.Is(err, execution.ErrWorkspaceBusy) {
		t.Fatalf("same active workflow=%v", err)
	}
	oldCall.unblock()
	observeTurn(t, f, old, 0)
	next := start(workflow)
	nextCall := nextAgentCall(t, f, a)
	if nextCall.session.WorkspaceID != oldCall.session.WorkspaceID {
		t.Fatal("workflow did not reuse its directory")
	}
	select {
	case sid := <-a.ended:
		if sid != old.SessionID {
			t.Fatalf("ended wrong predecessor: %s", sid)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	oldSession, err := f.service.ReadSession(f.ctx, f.scope, old.SessionID)
	if err != nil || oldSession.EndedAt == nil {
		t.Fatalf("predecessor still live: %+v err=%v", oldSession, err)
	}
	nextCall.unblock()
	observeTurn(t, f, next, 0)
	if err := f.service.End(f.ctx, f.scope, next.SessionID); err != nil {
		t.Fatal(err)
	}
	events, err := f.db.ReadExecutionEvents(f.ctx, f.scope, next.SessionID, 0, 100)
	if err != nil || len(events) != 2 || events[0].Kind != execution.EventTurnEnded || events[1].Kind != execution.EventSessionEnded {
		t.Fatalf("duplicate driver terminal events=%+v err=%v", events, err)
	}
}

func TestRuntimeActiveAgentCancelEndsSessionButOldTerminalCancelDoesNot(t *testing.T) {
	f := runtimeDatabase(t, nil)
	if err := f.service.Close(f.ctx); err != nil {
		t.Fatal(err)
	}
	a := &orchestrationAgent{started: make(chan *agentCall, 8), ended: make(chan string, 8)}
	var err error
	f.service, err = New(f.ctx, Config{Store: f.db, Models: f.models, Agent: a})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.CreateProject(f.ctx, f.scope, store.Project{ID: "project", Name: "Demo", RepositoryURL: "https://example.com/demo.git", Commit: "fixed", MirrorPath: "/fake/unused"}); err != nil {
		t.Fatal(err)
	}
	request := execution.StartRequest{Caller: modelRequest("chat", "first", "").Caller, Target: execution.Target{ProjectID: "project"}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("run")}
	first, err := f.service.Start(f.ctx, f.scope, request)
	if err != nil {
		t.Fatal(err)
	}
	firstCall := nextAgentCall(t, f, a)
	firstCall.unblock()
	completed, _ := observeTurn(t, f, first, 0)
	second, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: first.SessionID, Caller: modelRequest("chat", "second", "").Caller, Prompt: execution.TextPrompt("next")})
	if err != nil {
		t.Fatal(err)
	}
	secondCall := nextAgentCall(t, f, a)
	actual, err := f.service.Cancel(f.ctx, f.scope, first)
	if err != nil || !reflect.DeepEqual(actual, completed) {
		t.Fatalf("terminal Cancel rewrote result: %+v err=%v", actual, err)
	}
	if secondCall.ctx.Err() != nil {
		t.Fatal("old terminal Agent cancellation stopped the current turn")
	}
	select {
	case sid := <-a.ended:
		t.Fatalf("old terminal Cancel ended %s", sid)
	default:
	}
	cancelled, err := f.service.Cancel(f.ctx, f.scope, second)
	if err != nil || cancelled.Status != execution.StatusCancelled {
		t.Fatalf("active Agent Cancel=%+v err=%v", cancelled, err)
	}
	select {
	case sid := <-a.ended:
		if sid != second.SessionID {
			t.Fatalf("wrong Agent ended: %s", sid)
		}
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	session, err := f.service.ReadSession(f.ctx, f.scope, second.SessionID)
	if err != nil || session.EndedAt == nil {
		t.Fatalf("cancelled Agent session remained live: %+v err=%v", session, err)
	}
	if _, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: second.SessionID, Caller: modelRequest("chat", "after-cancel", "").Caller, Prompt: execution.TextPrompt("must fail")}); !errors.Is(err, execution.ErrSessionEnded) {
		t.Fatalf("Agent Append after active Cancel=%v", err)
	}
	got, err := f.service.ReadAttempt(f.ctx, f.scope, request.Caller)
	if err != nil || got.Result == nil || !reflect.DeepEqual(*got.Result, completed) {
		t.Fatalf("Agent Cancel rewrote previous result: %+v err=%v", got, err)
	}
}

func TestModelsFromEnvironmentRejectsPartialConfigurationWithoutSecrets(t *testing.T) {
	for _, values := range []map[string]string{
		{"V2_MODEL_API_KEY": "secret-key"},
		{"V2_MODEL_BASE_URL": "https://provider.example/v1", "V2_MODEL_API_KEY": "secret-key"},
		{"V2_MODEL_BASE_URL": "http://provider.example/v1", "V2_MODEL_API_KEY": "secret-key", "V2_MODEL_NAME": "model"},
		{"V2_ANTHROPIC_MODEL_API_KEY": "secret-key", "V2_ANTHROPIC_MODEL_NAME": "model"},
		{"V2_ANTHROPIC_MODEL_BASE_URL": "http://provider.example/v1", "V2_ANTHROPIC_MODEL_API_KEY": "secret-key", "V2_ANTHROPIC_MODEL_NAME": "model"},
	} {
		models, err := ModelsFromEnvironment(func(name string) string { return values[name] })
		if err == nil || models != nil || strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("partial/invalid configuration accepted or leaked key: models=%v err=%v", models, err)
		}
	}
	models, err := ModelsFromEnvironment(func(string) string { return "" })
	if err != nil || len(models) != 2 || models["fixture"].Binding.Protocol != nodeservices.OpenAICompletions || models["messages-fixture"].Binding.Protocol != nodeservices.AnthropicMessages {
		t.Fatalf("fixtures=%v err=%v", models, err)
	}
	env := map[string]string{"V2_ANTHROPIC_MODEL_BASE_URL": "https://provider.example/v1", "V2_ANTHROPIC_MODEL_API_KEY": "secret-key", "V2_ANTHROPIC_MODEL_NAME": "claude-model"}
	models, err = ModelsFromEnvironment(func(name string) string { return env[name] })
	if c := models["messages-configured"]; err != nil || len(models) != 3 || c.Binding.Protocol != nodeservices.AnthropicMessages || c.Binding.Model.ID != "claude-model" || c.Model.Name != "claude-model" {
		t.Fatalf("configured Messages provider=%v err=%v", models, err)
	}
}
