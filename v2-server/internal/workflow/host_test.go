package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

const hostGraph = `{"nodes":[
 {"id":"s","type":"workflow","data":{"kind":"start"}},
 {"id":"a","type":"workflow","data":{"kind":"agent","agentConfig":{"schemaVersion":3,"prompt":"first {{#s.input#}}"}}},
 {"id":"b","type":"workflow","data":{"kind":"agent","agentConfig":{"schemaVersion":3,"prompt":"second {{#a.output#}}"}}},
 {"id":"o","type":"workflow","data":{"kind":"output","outputs":[{"name":"answer","variableSelector":["b","output"]}]}}
 ],"edges":[{"id":"sa","source":"s","target":"a"},{"id":"ab","source":"a","target":"b"},{"id":"bo","source":"b","target":"o"}]}`

type hostFixture struct {
	db    *store.Store
	scope execution.Scope
	host  *workflow.Host
	wf    string
	in    workflow.Admission
}

func newHostFixture(t *testing.T, runtime execution.Service) *hostFixture {
	t.Helper()
	db := storetest.Open(t)
	user, tenant := storetest.Login(t, db, "host")
	f := &hostFixture{db: db, scope: execution.Scope{TenantID: tenant, UserID: user}}
	d, err := db.CreateWorkflow(t.Context(), f.scope, "doc", workflow.CreateDocument{Name: "Host", Graph: json.RawMessage(hostGraph)})
	if err != nil {
		t.Fatal(err)
	}
	f.wf = d.ID
	snapshot, err := db.PublishWorkflow(t.Context(), f.scope, d.ID, "publish", workflow.PublishDocument{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject(t.Context(), f.scope, store.Project{ID: "project", Name: "Project", RepositoryURL: "https://example.test/repo", Commit: "test", MirrorPath: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	f.in = workflow.Admission{SnapshotID: snapshot.ID, ProjectID: project.ID, Input: workflow.Object{"input": "hello"}, Executors: map[string]string{"a": "executor", "b": "executor"}}
	f.host, err = workflow.NewHost(context.Background(), db, runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.host.Close)
	return f
}

func (f *hostFixture) admit(t *testing.T, key string) workflow.Record {
	t.Helper()
	r, err := f.db.CreateWorkflowRun(t.Context(), f.scope, f.wf, key, f.in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type hostStream struct {
	event *execution.Event
}

func (s *hostStream) Next(ctx context.Context) (execution.Event, error) {
	if s.event == nil {
		<-ctx.Done()
		return execution.Event{}, ctx.Err()
	}
	event := *s.event
	s.event = nil
	return event, nil
}
func (*hostStream) Close() error { return nil }

func completedHostStream(id string) execution.Stream {
	return &hostStream{event: &execution.Event{Kind: execution.EventTurnEnded, Ref: execution.Ref{SessionID: id, TurnID: "turn"}, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "answer " + id}}}
}

func awaitRun(t *testing.T, f *hostFixture, id string, predicate func(workflow.Record) bool) workflow.Record {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		r, err := f.db.GetWorkflowRun(ctx, f.scope, f.wf, id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(r) {
			return r
		}
		select {
		case <-ctx.Done():
			t.Fatalf("run did not reach expected state: %+v", r)
		case <-tick.C:
		}
	}
}

func TestHostRunsDurableSerialWorkflowIndependentlyOfHTTP(t *testing.T) {
	runtime := &executiontest.Stub{}
	f := newHostFixture(t, runtime)
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var calls []string
	runtime.StartFunc = func(ctx context.Context, scope execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		r, err := f.db.GetWorkflowRun(ctx, scope, f.wf, in.Caller.ID)
		if err != nil || r.Checkpoint.Active == nil || !reflect.DeepEqual(r.Checkpoint.Active.Caller, in.Caller) || r.Checkpoint.Ref != (execution.Ref{}) {
			t.Errorf("Start preceded durable intent: %+v %v", r, err)
			return execution.Ref{}, execution.ErrInvalidRequest
		}
		if in.Caller.NodeID == "a" {
			close(started)
			select {
			case <-ctx.Done():
				return execution.Ref{}, ctx.Err()
			case <-release:
			}
		}
		prompt, _ := in.Prompt.Text()
		mu.Lock()
		calls = append(calls, prompt)
		mu.Unlock()
		return execution.Ref{SessionID: in.Caller.NodeID, TurnID: "turn"}, nil
	}
	runtime.SubscribeFunc = func(_ context.Context, _ execution.Scope, id string, _ uint64) (execution.Stream, error) {
		return completedHostStream(id), nil
	}
	runtime.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
		mu.Lock()
		calls = append(calls, "end "+id)
		mu.Unlock()
		return nil
	}
	r := f.admit(t, "run")
	request, cancelRequest := context.WithCancel(t.Context())
	if _, err := f.host.Launch(request, f.scope, f.wf, r.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	cancelRequest()
	// Concurrent retries cannot construct a second machine, even while Start blocks.
	var retries sync.WaitGroup
	for range 8 {
		retries.Go(func() {
			if _, err := f.host.Launch(t.Context(), f.scope, f.wf, r.ID); err != nil {
				t.Error(err)
			}
		})
	}
	retries.Wait()
	close(release)
	final := awaitRun(t, f, r.ID, func(r workflow.Record) bool { return r.FinishedAt != nil })
	if final.Checkpoint.State.Status != "succeeded" || final.Checkpoint.State.Nodes.O("o").O("output").S("answer") != "answer b" {
		t.Fatalf("final: %+v", final)
	}
	f.host.Close()
	if !reflect.DeepEqual(calls, []string{"first hello", "end a", "second answer a", "end b"}) {
		t.Fatal(calls)
	}
	page, err := f.db.ListWorkflowAttempts(t.Context(), f.scope, f.wf, r.ID, 0, 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor != "1" || page.Items[0].Ref.SessionID != "a" || page.Items[0].SettledAt == nil {
		t.Fatalf("first receipt page: %+v %v", page, err)
	}
	page, err = f.db.ListWorkflowAttempts(t.Context(), f.scope, f.wf, r.ID, 1, 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor != "" || page.Items[0].Ref.SessionID != "b" || page.Items[0].SettledAt == nil {
		t.Fatalf("second receipt page: %+v %v", page, err)
	}
}

func TestHostShutdownJoinsWorkerAndRestartNeverReplays(t *testing.T) {
	runtime := &executiontest.Stub{}
	f := newHostFixture(t, runtime)
	started := make(chan struct{})
	var starts, ends atomic.Int32
	runtime.StartFunc = func(_ context.Context, _ execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		starts.Add(1)
		return execution.Ref{SessionID: in.Caller.NodeID, TurnID: "turn"}, nil
	}
	runtime.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
		close(started)
		return &hostStream{}, nil
	}
	runtime.EndFunc = func(context.Context, execution.Scope, string) error { ends.Add(1); return nil }
	r := f.admit(t, "run")
	if _, err := f.host.Launch(t.Context(), f.scope, f.wf, r.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not subscribe")
	}
	f.host.Close()
	final, err := f.db.GetWorkflowRun(t.Context(), f.scope, f.wf, r.ID)
	if err != nil || final.Checkpoint.State.Error != "interrupted" || final.FinishedAt == nil || ends.Load() != 1 {
		t.Fatalf("shutdown did not settle: %+v %v ends=%d", final, err, ends.Load())
	}
	pending := f.admit(t, "not-launched")
	// The application has already recovered the shared runtime before constructing
	// this host. Neither the finished run nor an abandoned admission is replayed.
	restarted, err := workflow.NewHost(t.Context(), f.db, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	for _, id := range []string{r.ID, pending.ID} {
		got, err := restarted.Launch(t.Context(), f.scope, f.wf, id)
		if err != nil || got.Checkpoint.State.Error != "interrupted" {
			t.Fatalf("restart: %+v %v", got, err)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("restart dispatched %d attempts", starts.Load())
	}
}

func TestHostUnconfirmedStopCanBeReconciledWithoutNextNode(t *testing.T) {
	runtime := &executiontest.Stub{}
	f := newHostFixture(t, runtime)
	var starts atomic.Int32
	var allowStop atomic.Bool
	var caller execution.Caller
	var callerMu sync.Mutex
	runtime.StartFunc = func(_ context.Context, _ execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		starts.Add(1)
		callerMu.Lock()
		caller = in.Caller
		callerMu.Unlock()
		return execution.Ref{SessionID: "a", TurnID: "turn"}, nil
	}
	runtime.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
		return completedHostStream("a"), nil
	}
	runtime.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, want execution.Caller) (execution.Turn, error) {
		callerMu.Lock()
		defer callerMu.Unlock()
		if !reflect.DeepEqual(want, caller) {
			return execution.Turn{}, execution.ErrNotFound
		}
		return execution.Turn{Ref: execution.Ref{SessionID: "a", TurnID: "turn"}, Caller: caller}, nil
	}
	runtime.EndFunc = func(context.Context, execution.Scope, string) error {
		if !allowStop.Load() {
			return execution.ErrStopUnconfirmed
		}
		return nil
	}
	r := f.admit(t, "run")
	if _, err := f.host.Launch(t.Context(), f.scope, f.wf, r.ID); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, f, r.ID, func(r workflow.Record) bool { return r.Checkpoint.Ref.SessionID != "" })
	// Wait until Cancel observes the stopped worker and encounters the same fence.
	deadline := time.After(5 * time.Second)
	for {
		_, err := f.host.Cancel(t.Context(), f.scope, f.wf, r.ID)
		if errors.Is(err, execution.ErrStopUnconfirmed) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-deadline:
			t.Fatal("worker stop was not observable")
		case <-time.After(10 * time.Millisecond):
		}
	}
	before, err := f.db.GetWorkflowRun(t.Context(), f.scope, f.wf, r.ID)
	if err != nil || before.FinishedAt != nil || before.Checkpoint.Active == nil || starts.Load() != 1 {
		t.Fatalf("unconfirmed stop released run: %+v %v", before, err)
	}
	allowStop.Store(true)
	final, err := f.host.Cancel(t.Context(), f.scope, f.wf, r.ID)
	if err != nil || final.Checkpoint.State.Status != "cancelled" || final.FinishedAt == nil || starts.Load() != 1 {
		t.Fatalf("cancel reconciliation: %+v %v starts=%d", final, err, starts.Load())
	}
}

func TestHostCancelWhileAcceptedStartReturnsRetainsReceiptAndSettles(t *testing.T) {
	runtime := &executiontest.Stub{}
	f := newHostFixture(t, runtime)
	accepted := make(chan execution.Caller, 1)
	release := make(chan struct{})
	ref := execution.Ref{SessionID: "accepted", TurnID: "turn"}
	var starts, ends atomic.Int32
	runtime.StartFunc = func(_ context.Context, _ execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		starts.Add(1)
		accepted <- in.Caller
		<-release
		return ref, nil // Accepted before cancellation, response delivered afterwards.
	}
	runtime.EndFunc = func(context.Context, execution.Scope, string) error { ends.Add(1); return nil }
	r := f.admit(t, "run")
	if _, err := f.host.Launch(t.Context(), f.scope, f.wf, r.ID); err != nil {
		t.Fatal(err)
	}
	var caller execution.Caller
	select {
	case caller = <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("Start was not accepted")
	}
	runtime.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, want execution.Caller) (execution.Turn, error) {
		if !reflect.DeepEqual(want, caller) {
			return execution.Turn{}, execution.ErrNotFound
		}
		return execution.Turn{Ref: ref, Caller: caller}, nil
	}
	if _, err := f.host.Cancel(t.Context(), f.scope, f.wf, r.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	final := awaitRun(t, f, r.ID, func(r workflow.Record) bool { return r.FinishedAt != nil })
	if final.Checkpoint.State.Status != "cancelled" || starts.Load() != 1 || ends.Load() == 0 {
		t.Fatalf("cancel left accepted turn unsettled: %+v", final)
	}
	page, err := f.db.ListWorkflowAttempts(t.Context(), f.scope, f.wf, r.ID, 0, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].Ref != ref || page.Items[0].SettledAt == nil {
		t.Fatalf("lost accepted receipt: %+v %v", page, err)
	}
}
