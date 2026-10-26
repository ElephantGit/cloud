package core

import "fmt"

const (
	workflowLocalStateLimit = 1024 * 1024
	workflowStateLimit      = 8 * 1024 * 1024
	workflowFailureReserve  = 64 * 1024
)

type workflowStateOverflow struct {
	node         workflowNodeView
	round        int64
	bytes, limit int
	reason       string
}

func workflowEncodedStates(states map[string]workflowNodeState) Object {
	out := Object{}
	for id := range states {
		state := states[id]
		out[id] = state.object()
	}
	return out
}

// workflowStateBudget measures the exact JSON representation used by persistence. Values copied
// into input, ledger, archive and final output are intentionally counted at every occurrence.
func workflowStateBudget(graph workflowGraphView, states map[string]workflowNodeState) *workflowStateOverflow {
	var active workflowNodeView
	var round int64
	failedBudget := false
	for _, node := range graph.Nodes {
		if node.Kind != "loop" && node.Kind != "iteration" {
			continue
		}
		if active.ID == "" || states[node.ID].Status == "running" {
			active = node
		}
		state := states[node.ID]
		if state.Status == "cancelled" || (state.Status == "failed" && (state.Output.S("reason") == "workflow_state_size_exceeded" || state.Output.S("reason") == node.Kind+"_state_size_exceeded")) {
			failedBudget = true
		}
		localReason := node.Kind + "_state_size_exceeded"
		check := func(value any, r int64) *workflowStateOverflow {
			n := len(jsonText(value))
			if n > workflowLocalStateLimit {
				return &workflowStateOverflow{node: node, round: r, bytes: n, limit: workflowLocalStateLimit, reason: localReason}
			}
			return nil
		}
		members := Object{}
		for _, member := range graph.Nodes {
			if member.Container == node.ID {
				if s, ok := states[member.ID]; ok {
					members[member.ID] = s.object()
				}
			}
		}
		if s := state.Iteration; s != nil {
			round = int64(s.Round)
			for _, item := range s.Items {
				if over := check(item, round); over != nil {
					return over
				}
			}
			for _, value := range s.Output {
				if over := check(value, round); over != nil {
					return over
				}
			}
			for _, raw := range s.Completed {
				archive, _ := asWorkflowObject(raw)
				if over := check(archive.O("states"), archive.N("index")); over != nil {
					return over
				}
			}
			if over := check(members, round); over != nil {
				return over
			}
		}
		if s := state.Loop; s != nil {
			round = s.Round
			for _, raw := range s.Completed {
				archive, _ := asWorkflowObject(raw)
				if over := check(Object{"carried": archive["carried"], "states": archive["states"]}, archive.N("index")); over != nil {
					return over
				}
			}
			if over := check(Object{"carried": s.Carried, "states": members}, round); over != nil {
				return over
			}
			if state.Status == "succeeded" {
				if over := check(state.Output, round); over != nil {
					return over
				}
			}
		}
	}
	if active.ID == "" {
		return nil
	}
	n := len(jsonText(workflowEncodedStates(states)))
	limit := workflowStateLimit - workflowFailureReserve
	if failedBudget {
		limit = workflowStateLimit
	}
	if n > limit {
		return &workflowStateOverflow{node: active, round: round, bytes: n, limit: limit, reason: "workflow_state_size_exceeded"}
	}
	return nil
}

// rejectWorkflowStateOverflow discards the whole candidate, not just its largest value. The
// rollback source is the version being replaced, so no uncommitted ledger or member can survive.
func rejectWorkflowStateOverflow(t *transaction, runID string, version int64, states map[string]workflowNodeState, over *workflowStateOverflow) bool {
	row := t.one("SELECT node_states FROM workflow_runs WHERE id=$1 AND version=$2", runID, version)
	if row == nil {
		return false
	}
	prior := workflowStatesFromJSON(row.O("nodeStates"))
	clear(states)
	for id := range prior {
		states[id] = prior[id]
	}
	detail := fmt.Sprintf("state budget exceeded: round=%d bytes=%d limit=%d", over.round, over.bytes, over.limit)
	if over.node.Kind == "iteration" {
		failWorkflowIteration(states, over.node, over.reason, detail)
	} else {
		failWorkflowLoop(states, over.node, over.reason, detail)
	}
	state := states[over.node.ID]
	// A graph-controlled display name must not consume the bounded failure reserve.
	state.DisplayName = prior[over.node.ID].DisplayName
	state.Output = Object{"reason": over.reason, "round": over.round, "bytes": over.bytes, "limit": over.limit}
	states[over.node.ID] = state
	return true
}
