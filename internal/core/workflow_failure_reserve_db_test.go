package core

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowHistoricalFailureDoesNotUnlockReserve(t *testing.T) {
	for _, historical := range []string{"failed", "cancelled"} {
		for _, transition := range []string{"succeeded", "failed", "cancelled"} {
			t.Run(historical+"/"+transition, func(t *testing.T) {
				store := dispatcherDB(t)
				g := iterationRunGraph("continue", 10, []any{"it", "item"})
				id := seedWorkflowRunScene(t, store, g)
				states := map[string]workflowNodeState{
					"it":      {Status: "succeeded", Output: Object{"output": []any{}, "failed_count": 1}},
					"agg":     {Status: historical, Attempt: 1},
					"later":   {Status: "running", Attempt: 1},
					"padding": {Status: "succeeded", Output: Object{"value": ""}},
				}
				n := workflowStateLimit - workflowFailureReserve - 100 - len(jsonText(workflowEncodedStates(states)))
				states["padding"] = workflowNodeState{Status: "succeeded", Output: Object{"value": strings.Repeat("x", n)}}
				commitWorkflowRunMidpoint(t, store, id, workflowEncodedStates(states))
				_, _, _, version := readWorkflowRun(t, store, id)
				states["later"] = workflowNodeState{Status: transition, Attempt: 1, Error: strings.Repeat("e", 1024)}
				_, err := store.transact(context.Background(), func(tx *transaction) Object {
					settleWorkflowRunStates(tx, id, version, "running", "", states)
					return Object{}
				})
				if err != nil {
					t.Fatal(err)
				}
				status, detail, raw, _ := readWorkflowRun(t, store, id)
				got := workflowStatesFromJSON(raw)
				if transition == "succeeded" {
					if status != "failed" || detail != "workflow_state_size_exceeded" || got["later"].Status != "running" {
						t.Fatalf("historical %s unlocked ordinary write: %s %s", historical, status, detail)
					}
				} else if status != "running" || got["later"].Status != transition || got["it"].Status != "succeeded" || got["later"].Error != strings.Repeat("e", 1024) {
					t.Fatalf("new %s lost reserve or changed completed container: %s %s", transition, status, detail)
				}
			})
		}
	}
}
