package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Definition is the immutable, admitted input of one run. Executors maps node IDs to
// catalog IDs resolved at admission; it never silently discards an editor binding.
type Definition struct {
	ID          string            `json:"id"`
	WorkflowID  string            `json:"workflowId"`
	ProjectID   string            `json:"projectId"`
	CreatedAt   time.Time         `json:"createdAt"`
	Graph       Object            `json:"graph"`
	Input       Object            `json:"input"`
	Executors   map[string]string `json:"executors"`
	RolePrompts map[string]string `json:"rolePrompts,omitempty"`
}

// State is a checkpoint. Terminal checkpoints are read-only; a restarted process must
// interrupt an unfinished run instead of reconstructing a machine and replaying it.
type State struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Nodes  Object `json:"nodeStates"`
}

// Attempt is the single next external effect. Persist its running checkpoint before Start.
type Attempt struct {
	Caller     execution.Caller
	ExecutorID string
	Prompt     string
}

// Machine adapts Cloud's pure graph/round engines to the public execution port.
// One worker owns it; no method starts goroutines, touches SQL or starts a process.
type Machine struct {
	definition      Definition
	graph           workflowGraphView
	ambient         map[string]workflowNodeState
	states          map[string]workflowNodeState
	status, failure string
	pending         *Attempt
}

// NewMachine validates and copies the frozen input before any effect is possible.
func NewMachine(def Definition) (*Machine, error) {
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("encode workflow definition: %w", err)
	}
	if len(raw) > workflowStateLimit-workflowFailureReserve {
		return nil, errors.New("workflow definition exceeds execution limit")
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, err
	}
	if nodes, _ := def.Graph["nodes"].([]any); len(nodes) > 256 {
		return nil, errors.New("workflow graph exceeds execution limit")
	}
	if edges, _ := def.Graph["edges"].([]any); len(edges) > 2048 {
		return nil, errors.New("workflow graph exceeds execution limit")
	}
	if problems := Validate(def.Graph); len(problems) > 0 {
		return nil, fmt.Errorf("workflow graph invalid: %s", canonicalJSON(problems))
	}
	def.Graph = Normalize(def.Graph)
	graph := readWorkflowGraph(def.Graph)
	if len(graph.Nodes) > 256 || len(graph.Edges) > 2048 {
		return nil, errors.New("workflow graph exceeds execution limit")
	}
	for _, node := range graph.Nodes {
		if node.Kind != "agent" {
			continue
		}
		if def.Executors[node.ID] == "" {
			return nil, fmt.Errorf("executor binding missing for node %s", node.ID)
		}
		role := node.Data.O("agentConfig").S("roleId")
		if role != "" && def.RolePrompts[role] == "" {
			return nil, fmt.Errorf("role snapshot missing for node %s", node.ID)
		}
	}
	return &Machine{definition: def, graph: graph, ambient: workflowAmbientStates(Object{"workflowId": def.WorkflowID, "createdAtMs": def.CreatedAt.UnixMilli()}, def.Graph), states: map[string]workflowNodeState{}, status: "running"}, nil
}

// Snapshot returns an independent JSON checkpoint suitable for asynchronous persistence.
func (m *Machine) Snapshot() State {
	raw, _ := json.Marshal(workflowEncodedStates(m.states))
	nodes := Object{}
	_ = json.Unmarshal(raw, &nodes)
	return State{Status: m.status, Error: m.failure, Nodes: nodes}
}

// Step advances one pure transition or prepares one Agent attempt. Repeating Step
// while an attempt is active returns that same identity, never a second dispatch.
func (m *Machine) Step(now time.Time) *Attempt {
	if m.status != "running" {
		return nil
	}
	if m.pending != nil {
		return m.pending
	}
	previous := m.Snapshot()
	var attempt *Attempt
	dispatch := func(node workflowNodeView, pool map[string]workflowNodeState) {
		prompt := node.Data.O("agentConfig").S("prompt")
		if prompt == "" {
			prompt = "[no task statement]"
		}
		rendered, err := renderWorkflowTemplate(prompt, pool)
		if err != nil {
			m.states[node.ID] = workflowNodeState{Status: "failed", DisplayName: workflowNodeDisplayName(node), Error: "agent_prompt_template_invalid", Output: Object{"reason": "agent_prompt_template_invalid"}, FinishedAt: nowRFC3339()}
			return
		}
		if role := node.Data.O("agentConfig").S("roleId"); role != "" {
			rendered = m.definition.RolePrompts[role] + "\n\n" + rendered
		}
		previous := m.states[node.ID]
		number := previous.Attempt + 1
		caller := execution.Caller{Kind: execution.CallerWorkflow, ID: m.definition.ID, NodeID: node.ID, AttemptID: strconv.FormatInt(number, 10)}
		if node.Container != "" {
			parent := m.states[node.Container]
			index := 0
			if parent.Loop != nil {
				index = int(parent.Loop.Round - 1)
			}
			if parent.Iteration != nil {
				index = parent.Iteration.Round
			}
			caller.Rounds = []execution.Round{{ContainerID: node.Container, Index: index}}
		}
		m.states[node.ID] = workflowNodeState{Status: "running", DisplayName: workflowNodeDisplayName(node), StartedAt: now.UTC().Format(time.RFC3339Nano), Attempt: number}
		attempt = &Attempt{Caller: caller, ExecutorID: m.definition.Executors[node.ID], Prompt: rendered}
	}
	if id, ok := workflowRetryingAgentNode(m.states); ok {
		at, err := time.Parse(time.RFC3339Nano, m.states[id].RetryAt)
		if err != nil {
			m.status, m.failure = "failed", "invalid_retry_time"
			return nil
		}
		if now.Before(at) {
			return nil
		}
		node := m.graph.ByNode[id]
		pool := m.readStates(node)
		dispatch(node, pool)
	} else if node, ok := runningWorkflowLoop(m.graph, m.states); ok {
		advanceWorkflowLoop(dispatch, Object{"input": m.definition.Input}, node, m.states, m.ambient, m.graph)
	} else if node, ok := runningWorkflowIteration(m.graph, m.states); ok {
		advanceWorkflowIteration(dispatch, Object{"input": m.definition.Input}, node, m.states, m.ambient, m.graph)
	} else {
		outer := workflowOuterGraph(m.graph)
		projection := projectWorkflowNodes(outer, m.states)
		if projection.hasFailure() {
			m.status, m.failure = "failed", workflowRunFailureReason(outer, m.states)
		} else if node, ok := projection.readyNode(); ok {
			switch node.Kind {
			case "agent":
				dispatch(node, m.readStates(node))
			case "loop":
				advanceWorkflowLoop(dispatch, Object{"input": m.definition.Input}, node, m.states, m.ambient, m.graph)
			case "iteration":
				advanceWorkflowIteration(dispatch, Object{"input": m.definition.Input}, node, m.states, m.ambient, m.graph)
			default:
				m.states[node.ID] = executeWorkflowNode(node, m.definition.Input, m.states, m.ambient)
			}
		} else if projection.hasPending() {
			m.status, m.failure = "failed", "workflow_stalled"
		} else {
			workflowInactiveStates(outer, m.states)
			m.status = "succeeded"
		}
	}
	if reason := m.budgetFailure(); reason != "" {
		// Reject the whole transition, including oversized member outputs and archives.
		m.status, m.failure = "failed", reason
		m.states = workflowStatesFromJSON(previous.Nodes)
		m.stopActiveStates("failed", reason)
		attempt = nil
	}
	m.pending = attempt
	return attempt
}

func (m *Machine) readStates(node workflowNodeView) map[string]workflowNodeState {
	pool := m.states
	if node.Container != "" {
		owner := m.states[node.Container]
		if owner.Loop != nil {
			pool = workflowLoopReadStates(m.graph, node.Container, m.states, m.ambient)
		}
		if owner.Iteration != nil {
			pool = workflowIterationReadStates(m.graph, node.Container, m.states, m.ambient, owner.Iteration.Items[owner.Iteration.Round], owner.Iteration.Round)
		}
	}
	return workflowConditionStates(workflowPredecessorStates(m.graph, node, pool), m.ambient)
}

// ApplyResult commits only an authoritative result for the active attempt. The
// caller must successfully End the session first, so the next node can own the workspace.
func (m *Machine) ApplyResult(caller execution.Caller, result execution.Result, now time.Time) error {
	if m.pending == nil || canonicalJSON(caller) != canonicalJSON(m.pending.Caller) {
		return execution.ErrConflict
	}
	node := m.graph.ByNode[caller.NodeID]
	state := m.states[node.ID]
	previous := state
	state.FinishedAt = now.UTC().Format(time.RFC3339Nano)
	state.SessionEndReason = result.StopReason
	switch {
	case result.Status == execution.StatusCompleted && result.AnswerComplete:
		state.Status = "succeeded"
		state.Output = Object{"output": result.Answer}
	case result.Status == execution.StatusCancelled:
		state.Status = "cancelled"
		m.status = "cancelled"
	case result.Status == execution.StatusInterrupted:
		state.Status = "failed"
		state.Error = "interrupted"
		m.status, m.failure = "failed", "interrupted"
	default:
		reason := result.FailureCode
		if reason == "" {
			reason = "execution_failed"
		}
		if result.Status == execution.StatusCompleted {
			reason = "answer_incomplete"
		}
		state.Status, state.Error = "failed", reason
		if wait, retry := workflowRetryDecision(workflowRetryPolicyOf(node.Data), state.Attempt, reason); retry {
			state.Status = "retrying"
			state.RetryAt = now.Add(wait).UTC().Format(time.RFC3339Nano)
		}
	}
	m.states[node.ID] = state
	if m.status == "cancelled" {
		m.stopActiveStates("cancelled", "")
	}
	if m.status == "failed" {
		m.stopActiveStates("failed", m.failure)
	}
	if reason := m.budgetFailure(); reason != "" {
		previous.Status, previous.Error, previous.FinishedAt = "failed", reason, now.UTC().Format(time.RFC3339Nano)
		previous.Output = nil
		m.states[node.ID] = previous
		m.status, m.failure = "failed", reason
		m.stopActiveStates("failed", reason)
	}
	m.pending = nil
	return nil
}

func (m *Machine) budgetFailure() string {
	if over := workflowStateBudget(m.graph, m.states); over != nil {
		return over.reason
	}
	// The original container budget also needs a ceiling for graphs with no containers.
	if len(jsonText(workflowEncodedStates(m.states))) > workflowStateLimit-workflowFailureReserve {
		return "workflow_state_size_exceeded"
	}
	return ""
}

// Interrupt records cancellation or interruption without creating another attempt.
func (m *Machine) Interrupt(cancelled bool) {
	if m.status != "running" {
		return
	}
	m.status, m.failure = "failed", "interrupted"
	nodeStatus := "failed"
	if cancelled {
		m.status, m.failure, nodeStatus = "cancelled", "", "cancelled"
	}
	m.stopActiveStates(nodeStatus, m.failure)
	m.pending = nil
}

func (m *Machine) stopActiveStates(status, reason string) {
	for id, state := range m.states {
		if state.Status == "running" || state.Status == "retrying" {
			state.Status, state.Error, state.FinishedAt = status, reason, nowRFC3339()
			state.RetryAt = ""
			m.states[id] = state
		}
	}
}
