package core

import (
	"context"
	"errors"
	"testing"
)

// stubDirectory is one collaboration source with fixed targets, or with a fixed failure so the
// chain's error handling can be observed without a database.
type stubDirectory struct {
	items []CollaborationTargetSummary
	fail  error
}

func (d stubDirectory) ListTargets(context.Context, string, string) ([]CollaborationTargetSummary, error) {
	if d.fail != nil {
		return nil, d.fail
	}
	return d.items, nil
}

func (d stubDirectory) ResolveTarget(_ context.Context, _, targetType, targetID string) (CollaborationTargetSummary, bool, error) {
	if d.fail != nil {
		return CollaborationTargetSummary{}, false, d.fail
	}
	for _, item := range d.items {
		if item.Type == targetType && item.ID == targetID {
			return item, true, nil
		}
	}
	return CollaborationTargetSummary{}, false, nil
}

// TestDirectoryChainListsInOrderAndResolvesFirstMatch pins the composition rule production relies on:
// listing concatenates sources in chain order, resolution stops at the first source that can answer
// (so a later duplicate never shadows the earlier one), and a target no source knows is not-found —
// the same 404 an unknown id produces, not an error.
func TestDirectoryChainListsInOrderAndResolvesFirstMatch(t *testing.T) {
	agent := CollaborationTargetSummary{Type: "agent", ID: "a-1", DisplayName: "Agent"}
	workflow := CollaborationTargetSummary{Type: "workflow", ID: "w-1", DisplayName: "Workflow"}
	chain := DirectoryChain{
		stubDirectory{items: []CollaborationTargetSummary{agent}},
		stubDirectory{items: []CollaborationTargetSummary{workflow, agent}},
	}
	items, err := chain.ListTargets(context.Background(), "t-1", "")
	if err != nil || len(items) != 3 || items[0].ID != "a-1" || items[1].ID != "w-1" || items[2].ID != "a-1" {
		t.Fatalf("chain listing must concatenate sources in order: %v %v", items, err)
	}
	resolved, ok, err := chain.ResolveTarget(context.Background(), "t-1", "agent", "a-1")
	if err != nil || !ok || resolved.DisplayName != "Agent" {
		t.Fatalf("the first source's answer must win: %v %v %v", resolved, ok, err)
	}
	if _, ok, err := chain.ResolveTarget(context.Background(), "t-1", "team", "x-1"); ok || err != nil {
		t.Fatalf("an unknown target is not-found, never an error: %v %v", ok, err)
	}
	empty := DirectoryChain{}
	if items, err := empty.ListTargets(context.Background(), "t-1", ""); err != nil || len(items) != 0 {
		t.Fatalf("an empty chain lists nothing: %v %v", items, err)
	}
	if _, ok, err := empty.ResolveTarget(context.Background(), "t-1", "agent", "a-1"); ok || err != nil {
		t.Fatalf("an empty chain resolves nothing: %v %v", ok, err)
	}
}

// TestDirectoryChainNeverHidesASourceFailure: a source failure must abort the chain and surface as-is.
// Swallowing it would turn a database outage into a silent 404, and falling through to the next
// source would let a partial answer be mistaken for the tenant's whole catalog.
func TestDirectoryChainNeverHidesASourceFailure(t *testing.T) {
	failure := errors.New("source unavailable")
	chain := DirectoryChain{
		stubDirectory{items: []CollaborationTargetSummary{{Type: "agent", ID: "a-1"}}},
		stubDirectory{fail: failure},
		stubDirectory{items: []CollaborationTargetSummary{{Type: "workflow", ID: "w-1"}}},
	}
	if _, err := chain.ListTargets(context.Background(), "t-1", ""); !errors.Is(err, failure) {
		t.Fatalf("listing must return the source error: %v", err)
	}
	if _, _, err := chain.ResolveTarget(context.Background(), "t-1", "workflow", "w-1"); !errors.Is(err, failure) {
		t.Fatalf("resolution must stop at the failing source instead of falling through: %v", err)
	}
}
