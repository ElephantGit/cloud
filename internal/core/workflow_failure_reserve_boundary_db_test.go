package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestWorkflowLastFailedIterationOutputCopyUsesBoundedReserve(t *testing.T) {
	ordinaryLimit := workflowStateLimit - workflowFailureReserve
	for _, size := range []int{ordinaryLimit, ordinaryLimit + 1, workflowStateLimit - 1, workflowStateLimit, workflowStateLimit + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			store := dispatcherDB(t)
			graph := readWorkflowGraph(iterationRunGraph("continue", 10, []any{"it", "item"}))
			id := seedWorkflowRunScene(t, store, iterationRunGraph("continue", 10, []any{"it", "item"}))
			item := strings.Repeat("i", 80*1024)
			states := workflowFailedIterationBoundaryStates([]any{item})

			// Size the expected wire document independently of the round engine. The final
			// ledger appears twice: in recovery progress and in the exposed output trio.
			expected := workflowStatesFromJSON(workflowEncodedStates(states))
			entry := Object{"item": item, "status": "failed", "output": nil, "error": "body_failed: failed member"}
			member := states["agg"]
			final := expected["it"]
			final.Status, final.FinishedAt = "succeeded", nowRFC3339()
			final.Iteration = &workflowIterationRunState{
				Items: []any{item}, Ledger: []any{entry}, Output: []any{}, Failed: 1, Strategy: "continue",
				Completed: []any{Object{"index": 0, "states": Object{"agg": member.object()}}},
			}
			final.Output = Object{"output": []any{}, "entries": []any{entry}, "failed_count": 1}
			expected["it"] = final
			padding := states["padding"]
			padding.Output = Object{"value": strings.Repeat("x", size-len(jsonText(workflowEncodedStates(expected))))}
			states["padding"], expected["padding"] = padding, padding
			if priorSize := len(jsonText(workflowEncodedStates(states))); priorSize > ordinaryLimit {
				t.Fatalf("fixture already consumes reserve: %d", priorSize)
			}
			commitWorkflowRunMidpoint(t, store, id, workflowEncodedStates(states))
			_, _, prior, version := readWorkflowRun(t, store, id)
			states = workflowStatesFromJSON(prior)
			_, err := store.transact(context.Background(), func(tx *transaction) Object {
				if !advanceWorkflowIteration(tx, Object{"id": id}, graph.ByNode["it"], version, states, nil, graph) {
					t.Fatal("failed round did not settle")
				}
				candidate := states["it"]
				if candidate.Status != "succeeded" || candidate.Iteration.Failed != 1 ||
					jsonText(candidate.Output) != jsonText(final.Output) ||
					jsonText(candidate.Iteration.object()) != jsonText(final.Iteration.object()) {
					t.Fatal("final failed round lost its ledger, archive or exposed output copy")
				}
				if got := len(jsonText(workflowEncodedStates(states))); got != size {
					t.Fatalf("candidate bytes=%d, want %d including final output copy", got, size)
				}
				withoutCopy := workflowStatesFromJSON(workflowEncodedStates(states))
				container := withoutCopy["it"]
				container.Output = nil
				withoutCopy["it"] = container
				if len(jsonText(workflowEncodedStates(withoutCopy))) > ordinaryLimit {
					t.Fatal("fixture must need reserve specifically because of the final output copy")
				}
				settleWorkflowRunStates(tx, id, version, "running", "", states)
				return Object{}
			})
			if err != nil {
				t.Fatal(err)
			}
			status, detail, raw, afterVersion := readWorkflowRun(t, store, id)
			got := workflowStatesFromJSON(raw)["it"]
			wantVersion := version + 1
			if size > workflowStateLimit {
				// The failed-state CAS and the no-Workspace release fact each increment version.
				wantVersion++
			}
			if afterVersion != wantVersion || len(jsonText(raw)) > workflowStateLimit {
				t.Fatalf("settlement version=%d prior=%d bytes=%d", afterVersion, version, len(jsonText(raw)))
			}
			if size <= workflowStateLimit {
				if status != "running" || detail != "" || got.Status != "succeeded" || len(jsonText(raw)) != size ||
					jsonText(got.Output) != jsonText(final.Output) || jsonText(got.Iteration.object()) != jsonText(final.Iteration.object()) {
					t.Fatalf("final failure and output copy rejected or changed: %s %s", status, detail)
				}
			} else if status != "failed" || detail != "workflow_state_size_exceeded" ||
				got.Output.N("limit") != int64(workflowStateLimit) || got.Output.N("bytes") != int64(size) ||
				jsonText(got.Iteration.object()) != jsonText(prior.O("it").O("iteration")) ||
				jsonText(raw.O("agg")) != jsonText(prior.O("agg")) {
				t.Fatalf("over-cap candidate was not discarded with its uncommitted ledger: %s %s", status, detail)
			}
		})
	}
}

func TestWorkflowContinueAfterReservedFailureRejectsOrdinaryGrowth(t *testing.T) {
	store := dispatcherDB(t)
	g := iterationRunGraph("continue", 10, []any{"it", "item"})
	graph := readWorkflowGraph(g)
	id := seedWorkflowRunScene(t, store, g)
	states := workflowFailedIterationBoundaryStates([]any{strings.Repeat("i", 1024), "next item"})
	padding := states["padding"]
	padding.Output = Object{"value": strings.Repeat("x", workflowStateLimit-workflowFailureReserve-len(jsonText(workflowEncodedStates(states))))}
	states["padding"] = padding
	commitWorkflowRunMidpoint(t, store, id, workflowEncodedStates(states))
	_, _, _, version := readWorkflowRun(t, store, id)
	// Commit the first failed round through the real CAS seam, then let a fresh scheduler
	// pass read it. Otherwise the appended ledger still belongs to the same transition.
	_, err := store.transact(context.Background(), func(tx *transaction) Object {
		if !advanceWorkflowIteration(tx, Object{"id": id}, graph.ByNode["it"], version, states, nil, graph) ||
			!settleWorkflowRunStates(tx, id, version, "running", "", states) {
			t.Fatal("first failure could not use reserve")
		}
		return Object{}
	})
	if err != nil {
		t.Fatal(err)
	}
	status, detail, reserved, reservedVersion := readWorkflowRun(t, store, id)
	progress := workflowStatesFromJSON(reserved)["it"].Iteration
	if size := len(jsonText(reserved)); status != "running" || detail != "" ||
		size <= workflowStateLimit-workflowFailureReserve || size > workflowStateLimit ||
		progress.Round != 1 || progress.Failed != 1 || len(progress.Ledger) != 1 || len(progress.Completed) != 1 ||
		reserved["agg"] != nil || reservedVersion != version+1 {
		t.Fatalf("first failure did not commit a bounded next-round checkpoint: %s %s bytes=%d round=%d failures=%d ledger=%d archive=%d version=%d prior=%d", status, detail, size, progress.Round, progress.Failed, len(progress.Ledger), len(progress.Completed), reservedVersion, version)
	}
	if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, detail, terminal, terminalVersion := readWorkflowRun(t, store, id)
	container := workflowStatesFromJSON(terminal)["it"]
	// Failure settlement and the no-Workspace release fact each increment version once.
	if status != "failed" || detail != "workflow_state_size_exceeded" || container.Status != "failed" ||
		container.Output.N("limit") != int64(workflowStateLimit-workflowFailureReserve) ||
		jsonText(container.Iteration.object()) != jsonText(progress.object()) || terminal["agg"] != nil ||
		len(jsonText(terminal)) > workflowStateLimit || terminalVersion != reservedVersion+2 {
		t.Fatalf("next ordinary write borrowed reserve or lost the committed failed round: %s %s", status, detail)
	}
	for range 2 {
		if err := store.AdvanceWorkflowRunsOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		afterStatus, afterDetail, after, afterVersion := readWorkflowRun(t, store, id)
		if afterStatus != status || afterDetail != detail || afterVersion != terminalVersion || jsonText(after) != jsonText(terminal) {
			t.Fatal("terminal replay changed the retained failure or retried the next item")
		}
	}
}

// workflowFailedIterationBoundaryStates is a valid first-round checkpoint whose member has
// already failed. Padding is outside the body so it exercises the total, not the local, limit.
func workflowFailedIterationBoundaryStates(items []any) map[string]workflowNodeState {
	return map[string]workflowNodeState{
		"s": {Status: "succeeded", Output: Object{"items": items}},
		"it": {Status: "running", Iteration: &workflowIterationRunState{
			Items: items, Ledger: []any{}, Output: []any{}, Completed: []any{}, Strategy: "continue",
		}},
		"agg":     {Status: "failed", Error: "failed member", Output: Object{"reason": "body_failed"}},
		"padding": {Status: "succeeded", Output: Object{"value": ""}},
	}
}
