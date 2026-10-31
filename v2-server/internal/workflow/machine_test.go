package workflow

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

func driveMachine(t *testing.T, m *Machine, answer func(*Attempt) execution.Result) State {
	t.Helper()
	for i := 0; i < 200 && m.Snapshot().Status == "running"; i++ {
		if attempt := m.Step(time.Now()); attempt != nil {
			if err := m.ApplyResult(attempt.Caller, answer(attempt), time.Now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	state := m.Snapshot()
	if state.Status == "running" {
		t.Fatalf("machine stalled: %+v", state)
	}
	return state
}

func TestMachineLoopPreservesCarriedValuesAndDistinctRoundIdentity(t *testing.T) {
	graph := loopTestGraph(validLoopConfig())
	for _, raw := range graph["nodes"].([]any) {
		node, _ := asWorkflowObject(raw)
		if node.S("id") == "worker" {
			node.O("data").O("agentConfig")["prompt"] = "{{#loop-1.draft#}}"
		}
	}
	m, err := NewMachine(Definition{ID: "run", Graph: graph, Executors: map[string]string{"worker": "configured"}})
	if err != nil {
		t.Fatal(err)
	}
	var attempts []execution.Caller
	state := driveMachine(t, m, func(a *Attempt) execution.Result {
		want := ""
		answer := "draft"
		if len(attempts) == 1 {
			want, answer = "draft", "done"
		}
		if a.Prompt != want {
			t.Fatalf("carried value = %q, want %q", a.Prompt, want)
		}
		attempts = append(attempts, a.Caller)
		return execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: answer}
	})
	if state.Status != "succeeded" || state.Nodes.O("loop-1").O("output").S("result") != "done" {
		t.Fatalf("loop %+v", state)
	}
	if len(attempts) != 2 || attempts[0].Rounds[0].Index != 0 || attempts[1].Rounds[0].Index != 1 || attempts[0].Rounds[0].ContainerID != "loop-1" {
		t.Fatal(attempts)
	}
	if attempts[0].AttemptID != "1" || attempts[1].AttemptID != "1" {
		t.Fatal("retry budget must restart in each distinct round")
	}
	if rounds := state.Nodes.O("loop-1").O("loop")["completed"].([]any); len(rounds) != 2 {
		t.Fatal(rounds)
	}
}

func TestMachineIterationContinueKeepsFailureLedger(t *testing.T) {
	graph := iterationEntryGraph()
	for _, raw := range graph["nodes"].([]any) {
		node, _ := asWorkflowObject(raw)
		if node.S("id") == "it" {
			node.O("data").O("iterationConfig")["errorStrategy"] = "continue"
		}
		if node.S("id") == "next" {
			node["data"] = Object{"kind": "agent", "agentConfig": Object{"schemaVersion": 3, "prompt": "{{#agg.output#}}", "retry": retryPolicy(false, 0, 0)}}
		}
	}
	m, err := NewMachine(Definition{ID: "run", Graph: graph, Input: Object{"items": []any{"bad", "good"}}, Executors: map[string]string{"next": "configured"}})
	if err != nil {
		t.Fatal(err)
	}
	var prompts []string
	state := driveMachine(t, m, func(a *Attempt) execution.Result {
		prompts = append(prompts, a.Prompt)
		if a.Prompt == "bad" {
			return execution.Result{Status: execution.StatusFailed, FailureCode: "agent_failed"}
		}
		return execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: a.Prompt}
	})
	output := state.Nodes.O("it").O("output")
	if state.Status != "succeeded" || !reflect.DeepEqual(prompts, []string{"bad", "good"}) || output.N("failed_count") != 1 || !reflect.DeepEqual(output["output"], []any{"good"}) {
		t.Fatalf("iteration: %+v / %v", state, prompts)
	}
	entries := output["entries"].([]any)
	if len(entries) != 2 || Object(entries[0].(map[string]any)).S("status") != "failed" {
		t.Fatal(entries)
	}
}

func TestMachineRetryKeepsFrozenInputsAndUsesNewAttempt(t *testing.T) {
	def := twoAgentDefinition()
	node, _ := asWorkflowObject(def.Graph["nodes"].([]any)[1])
	node.O("data").O("agentConfig")["retry"] = retryPolicy(true, 2, 10)
	m, err := NewMachine(def)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	m.Step(now)
	first := m.Step(now)
	if err := m.ApplyResult(first.Caller, execution.Result{Status: execution.StatusFailed, FailureCode: "agent_failed"}, now); err != nil {
		t.Fatal(err)
	}
	if a := m.Step(now.Add(9 * time.Second)); a != nil {
		t.Fatal("retry started too soon")
	}
	second := m.Step(now.Add(10 * time.Second))
	if second == nil || second.Caller.AttemptID != "2" || second.Prompt != first.Prompt || second.ExecutorID != first.ExecutorID {
		t.Fatalf("retry identity changed: %#v", second)
	}
	if err := m.ApplyResult(first.Caller, execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: "late"}, now); err != execution.ErrConflict {
		t.Fatal("stale result accepted")
	}
}

func TestMachineInterruptDoesNotRetryOrMutateTerminalState(t *testing.T) {
	m, err := NewMachine(twoAgentDefinition())
	if err != nil {
		t.Fatal(err)
	}
	m.Step(time.Now())
	attempt := m.Step(time.Now())
	if err := m.ApplyResult(attempt.Caller, execution.Result{Status: execution.StatusInterrupted}, time.Now()); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot()
	if before.Status != "failed" || before.Error != "interrupted" {
		t.Fatal(before)
	}
	if m.Step(time.Now().Add(time.Hour)) != nil {
		t.Fatal("interrupt replayed")
	}
	m.Interrupt(true)
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("terminal checkpoint changed")
	}
}

func TestMachineUnknownFailureIsNotAssumedRetryable(t *testing.T) {
	def := twoAgentDefinition()
	node, _ := asWorkflowObject(def.Graph["nodes"].([]any)[1])
	delete(node.O("data").O("agentConfig"), "retry")
	m, err := NewMachine(def)
	if err != nil {
		t.Fatal(err)
	}
	m.Step(time.Now())
	attempt := m.Step(time.Now())
	if err := m.ApplyResult(attempt.Caller, execution.Result{Status: execution.StatusFailed}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Nodes.O("a").S("status") != "failed" {
		t.Fatal("unknown failure was retried")
	}
}

func TestMachineContainerBudgetRollsBackOversizedMemberOutput(t *testing.T) {
	m, err := NewMachine(Definition{Graph: loopTestGraph(validLoopConfig()), Executors: map[string]string{"worker": "configured"}})
	if err != nil {
		t.Fatal(err)
	}
	state := driveMachine(t, m, func(*Attempt) execution.Result {
		return execution.Result{Status: execution.StatusCompleted, AnswerComplete: true, Answer: strings.Repeat("x", workflowLocalStateLimit+1)}
	})
	if state.Status != "failed" || state.Error != "loop_state_size_exceeded" || len(jsonText(state.Nodes)) > workflowLocalStateLimit {
		t.Fatal("oversized member retained")
	}
}

func TestMachineRemoteCancellationSettlesActiveContainer(t *testing.T) {
	m, err := NewMachine(Definition{Graph: loopTestGraph(validLoopConfig()), Executors: map[string]string{"worker": "configured"}})
	if err != nil {
		t.Fatal(err)
	}
	state := driveMachine(t, m, func(*Attempt) execution.Result { return execution.Result{Status: execution.StatusCancelled} })
	if state.Status != "cancelled" || state.Nodes.O("loop-1").S("status") != "cancelled" || state.Nodes.O("worker").S("status") != "cancelled" {
		t.Fatalf("active state left behind: %+v", state)
	}
}
