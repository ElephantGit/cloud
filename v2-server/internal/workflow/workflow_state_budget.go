// Adapted from cloud/internal/core/workflow_state_budget.go at 0d3299d5.
// Database and execution effects are provided by the v2 adapter.
package workflow

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
	for _, node := range graph.Nodes {
		if node.Kind != "loop" && node.Kind != "iteration" {
			continue
		}
		if active.ID == "" || states[node.ID].Status == "running" {
			active = node
		}
		state := states[node.ID]
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
	if n > limit {
		return &workflowStateOverflow{node: active, round: round, bytes: n, limit: limit, reason: "workflow_state_size_exceeded"}
	}
	return nil
}
