//go:build linux

package agentruntime

import (
	"reflect"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

func TestProviderForCurrentObservedTurnAndPassthrough(t *testing.T) {
	binding := nodeservices.Binding{Model: nodeservices.Model{ID: "fixture"}, Provider: nodeservices.FixtureProvider{}}
	observed := nodeservices.MessagesFixtureProvider{}
	var refs []execution.Ref
	p := &projectRuntime{driver: &Driver{opts: Options{ObserveProvider: func(ref execution.Ref, actual nodeservices.Binding) nodeservices.Provider {
		if !reflect.DeepEqual(actual, binding) {
			t.Fatal("frozen binding changed")
		}
		refs = append(refs, ref)
		return observed
	}}}, agents: map[string]*agentSession{}}
	if p.providerFor("absent", binding) != nil {
		t.Fatal("unbound execution admitted")
	}
	a := &agentSession{observeRequests: true, frozen: execution.Session{ID: "session"}}
	p.agents["execution"] = a
	if p.providerFor("execution", binding) != nil {
		t.Fatal("idle observed session admitted")
	}
	for _, turn := range []string{"one", "two"} {
		a.active = &activeTurn{id: turn}
		if got := p.providerFor("execution", binding); !reflect.DeepEqual(got, observed) {
			t.Fatal("observer not wired")
		}
	}
	if !reflect.DeepEqual(refs, []execution.Ref{{SessionID: "session", TurnID: "one"}, {SessionID: "session", TurnID: "two"}}) {
		t.Fatalf("turns: %v", refs)
	}
	a.active = nil
	if p.providerFor("execution", binding) != nil {
		t.Fatal("ended turn still admitted")
	}
	a.observeRequests = false
	if got := p.providerFor("execution", binding); !reflect.DeepEqual(got, binding.Provider) {
		t.Fatal("unobserved caller changed provider")
	}
	if len(refs) != 2 {
		t.Fatal("unobserved caller recorded")
	}
}
