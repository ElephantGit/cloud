package workflow

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
)

type eventStream struct {
	events []execution.Event
	closed bool
	block  bool
}

func (s *eventStream) Next(ctx context.Context) (execution.Event, error) {
	if s.block {
		<-ctx.Done()
		return execution.Event{}, ctx.Err()
	}
	if len(s.events) == 0 {
		return execution.Event{}, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}
func (s *eventStream) Close() error { s.closed = true; return nil }

func twoAgentDefinition() Definition {
	a := wfAgentNode("a", Object{"schemaVersion": 3, "prompt": "first {{#s.input#}}", "retry": retryPolicy(false, 0, 0)})
	b := wfAgentNode("b", Object{"schemaVersion": 3, "prompt": "second {{#a.output#}}", "retry": retryPolicy(false, 0, 0)})
	out := wfNode("o", "output", "")
	out["data"].(Object)["outputs"] = []any{Object{"name": "answer", "variableSelector": []any{"b", "output"}}}
	return Definition{ID: "run", WorkflowID: "wf", ProjectID: "project", CreatedAt: time.Unix(100, 0), Graph: wfGraph([]Object{wfNode("s", "start", ""), a, b, out}, [][3]string{{"s", "a", ""}, {"a", "b", ""}, {"b", "o", ""}}), Input: Object{"input": "kickoff"}, Executors: map[string]string{"a": "agent:model-one", "b": "agent:model-two"}}
}

var testScope = execution.Scope{TenantID: "tenant", UserID: "owner"}

func discardCheckpoint(context.Context, Checkpoint) error { return nil }

func TestRunnerSerialAgentsUseFinalAnswerAndDrainBeforeNextStart(t *testing.T) {
	def := twoAgentDefinition()
	var calls []string
	var checkpoints []Checkpoint
	var active string
	var streams []*eventStream
	runtime := &executiontest.Stub{}
	runtime.StartFunc = func(_ context.Context, scope execution.Scope, in execution.StartRequest) (execution.Ref, error) {
		if scope != testScope || in.Target.ProjectID != def.ProjectID || in.Caller.ID != def.ID || in.Caller.Kind != execution.CallerWorkflow {
			t.Fatalf("scope or identity changed: %+v %+v", scope, in)
		}
		if active != "" {
			t.Fatal("next node started before End confirmed")
		}
		last := checkpoints[len(checkpoints)-1]
		if last.Active == nil || last.State.Nodes.O(in.Caller.NodeID).S("status") != "running" {
			t.Fatal("effect started before checkpoint")
		}
		active = in.Caller.NodeID
		calls = append(calls, "start "+active+": "+in.ExecutorID)
		prompt, _ := in.Prompt.Text()
		want := "first kickoff"
		if active == "b" {
			want = "second authoritative a"
		}
		if prompt != want {
			t.Fatalf("prompt %q, want %q", prompt, want)
		}
		return execution.Ref{SessionID: active, TurnID: "turn-" + active}, nil
	}
	runtime.SubscribeFunc = func(_ context.Context, _ execution.Scope, id string, _ uint64) (execution.Stream, error) {
		s := &eventStream{events: []execution.Event{
			{Kind: execution.EventHistorySnapshot, Ref: execution.Ref{SessionID: id, TurnID: "turn-" + id}, History: &execution.HistorySnapshot{Incomplete: true}},
			{Kind: execution.EventTurnEnded, Ref: execution.Ref{SessionID: id, TurnID: "unrelated"}, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "wrong turn"}},
			{Kind: execution.EventTurnEnded, Ref: execution.Ref{SessionID: id, TurnID: "turn-" + id}, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "authoritative " + id, StopReason: "end_turn"}},
		}}
		streams = append(streams, s)
		return s, nil
	}
	runtime.EndFunc = func(_ context.Context, _ execution.Scope, id string) error {
		calls = append(calls, "end "+id)
		active = ""
		return nil
	}
	final, err := Run(context.Background(), runtime, testScope, def, func(_ context.Context, c Checkpoint) error { checkpoints = append(checkpoints, c); return nil })
	if err != nil || final.Status != "succeeded" || final.Nodes.O("o").O("output").S("answer") != "authoritative b" {
		t.Fatalf("result %+v / %v", final, err)
	}
	if !reflect.DeepEqual(calls, []string{"start a: agent:model-one", "end a", "start b: agent:model-two", "end b"}) {
		t.Fatal(calls)
	}
	for _, s := range streams {
		if !s.closed {
			t.Fatal("subscription leaked")
		}
	}
	if checkpoints[0].State.Nodes.O("a").S("status") != "" {
		t.Fatal("old checkpoint mutated")
	}
}

func successfulRuntime(t *testing.T) *executiontest.Stub {
	t.Helper()
	return &executiontest.Stub{
		StartFunc: func(_ context.Context, _ execution.Scope, in execution.StartRequest) (execution.Ref, error) {
			return execution.Ref{SessionID: in.Caller.NodeID, TurnID: "turn"}, nil
		},
		SubscribeFunc: func(_ context.Context, _ execution.Scope, id string, _ uint64) (execution.Stream, error) {
			return &eventStream{events: []execution.Event{{Kind: execution.EventTurnEnded, Ref: execution.Ref{SessionID: id, TurnID: "turn"}, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "done"}}}}, nil
		},
		EndFunc: func(context.Context, execution.Scope, string) error { return nil },
	}
}

func TestRunnerStopUnconfirmedKeepsRunningAndBlocksNextNode(t *testing.T) {
	runtime := successfulRuntime(t)
	runtime.EndFunc = func(context.Context, execution.Scope, string) error { return execution.ErrStopUnconfirmed }
	state, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), discardCheckpoint)
	if !errors.Is(err, execution.ErrStopUnconfirmed) || state.Status != "running" || state.Nodes.O("a").S("status") != "running" || len(state.Nodes.O("b")) != 0 {
		t.Fatalf("unconfirmed stop lost fence: %+v %v", state, err)
	}
}

func TestRunnerCommitFailurePreventsDispatch(t *testing.T) {
	runtime := &executiontest.Stub{StartFunc: func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
		t.Fatal("uncommitted effect")
		return execution.Ref{}, nil
	}}
	failed := errors.New("storage unavailable")
	_, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), func(_ context.Context, c Checkpoint) error {
		if c.Active != nil {
			return failed
		}
		return nil
	})
	if !errors.Is(err, failed) {
		t.Fatal(err)
	}
}

func TestRunnerCommitFailureAfterStartDrainsSession(t *testing.T) {
	runtime := successfulRuntime(t)
	ended := false
	runtime.EndFunc = func(context.Context, execution.Scope, string) error { ended = true; return nil }
	failed := errors.New("storage unavailable")
	_, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), func(_ context.Context, c Checkpoint) error {
		if c.Ref.SessionID != "" {
			return failed
		}
		return nil
	})
	if !errors.Is(err, failed) || !ended {
		t.Fatalf("accepted session leaked: %v, %v", err, ended)
	}
}

func TestRunnerCancellationAndShutdownNeverReplay(t *testing.T) {
	for _, user := range []bool{false, true} {
		t.Run(map[bool]string{true: "cancel", false: "shutdown"}[user], func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			runtime := successfulRuntime(t)
			ended := false
			runtime.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
				if user {
					cancel(ErrUserCancelled)
				} else {
					cancel(context.Canceled)
				}
				return &eventStream{block: true}, nil
			}
			runtime.EndFunc = func(ctx context.Context, _ execution.Scope, _ string) error {
				if ctx.Err() != nil {
					t.Fatal("cleanup inherited cancelled context")
				}
				ended = true
				return nil
			}
			var finalCheckpoint Checkpoint
			state, err := Run(ctx, runtime, testScope, twoAgentDefinition(), func(ctx context.Context, c Checkpoint) error {
				if c.State.Status != "running" && ctx.Err() != nil {
					t.Fatal("terminal checkpoint cancelled")
				}
				finalCheckpoint = c
				return nil
			})
			want := "failed"
			if user {
				want = "cancelled"
			}
			if err != nil || state.Status != want || !ended || finalCheckpoint.State.Status != want || len(state.Nodes.O("b")) != 0 {
				t.Fatalf("cancel %+v %v", state, err)
			}
		})
	}
}

func TestRunnerIncompleteAnswerCannotFeedDownstream(t *testing.T) {
	runtime := successfulRuntime(t)
	runtime.SubscribeFunc = func(_ context.Context, _ execution.Scope, id string, _ uint64) (execution.Stream, error) {
		return &eventStream{events: []execution.Event{{Kind: execution.EventTurnEnded, Ref: execution.Ref{SessionID: id, TurnID: "turn"}, Result: &execution.Result{Status: execution.StatusCompleted, Answer: "partial"}}}}, nil
	}
	state, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), discardCheckpoint)
	if err != nil || state.Status != "failed" || state.Error != "answer_incomplete" || len(state.Nodes.O("b")) != 0 {
		t.Fatalf("partial accepted: %+v %v", state, err)
	}
}

func TestRunnerLostStartResponseReadsSameAttempt(t *testing.T) {
	runtime := successfulRuntime(t)
	runtime.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
		return execution.Ref{}, errors.New("response lost")
	}
	runtime.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
		return execution.Turn{Ref: execution.Ref{SessionID: c.NodeID, TurnID: "turn"}, Caller: c}, nil
	}
	state, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), discardCheckpoint)
	if err != nil || state.Status != "succeeded" {
		t.Fatalf("retained identity not resolved: %+v %v", state, err)
	}
}

func TestRunnerLostStreamReadsAuthoritativeResult(t *testing.T) {
	runtime := successfulRuntime(t)
	runtime.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
		return &eventStream{}, nil
	}
	runtime.ReadAttemptFunc = func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
		return execution.Turn{Ref: execution.Ref{SessionID: c.NodeID, TurnID: "turn"}, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "recovered"}}, nil
	}
	state, err := Run(context.Background(), runtime, testScope, twoAgentDefinition(), discardCheckpoint)
	if err != nil || state.Nodes.O("o").O("output").S("answer") != "recovered" {
		t.Fatalf("retained result lost: %+v %v", state, err)
	}
}

func TestMachineRejectsMissingExecutorAndRoleSnapshots(t *testing.T) {
	def := twoAgentDefinition()
	delete(def.Executors, "b")
	if _, err := NewMachine(def); err == nil {
		t.Fatal("missing executor silently accepted")
	}
	def = twoAgentDefinition()
	def.Graph["nodes"].([]any)[1].(map[string]any)["data"].(Object)["agentConfig"].(Object)["roleId"] = "author"
	if _, err := NewMachine(def); err == nil {
		t.Fatal("role silently dropped")
	}
}

func TestMachineOversizedAnswerDoesNotEscapeCheckpointBudget(t *testing.T) {
	m, err := NewMachine(twoAgentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	m.Step(time.Now())
	attempt := m.Step(time.Now())
	if err := m.ApplyResult(attempt.Caller, execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: strings.Repeat("x", workflowStateLimit)}, time.Now()); err != nil {
		t.Fatal(err)
	}
	state := m.Snapshot()
	if state.Status != "failed" || state.Error != "workflow_state_size_exceeded" || len(jsonText(state.Nodes)) >= workflowStateLimit {
		t.Fatal("oversized answer retained")
	}
}
