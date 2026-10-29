package executiontest_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
)

// A page wired to an empty double must not report an admitted or stopped Agent.
func TestUnconfiguredStubDoesNotClaimExecution(t *testing.T) {
	s := &executiontest.Stub{}
	ctx := context.Background()
	scope := execution.Scope{TenantID: "tenant", UserID: "user"}
	ref, startErr := s.Start(ctx, scope, execution.StartRequest{})
	result, cancelErr := s.Cancel(ctx, scope, ref)
	stream, subscribeErr := s.Subscribe(ctx, scope, ref.SessionID, 0)
	if !errors.Is(startErr, execution.ErrUnavailable) || ref != (execution.Ref{}) {
		t.Fatalf("unconfigured admission = %v, %v", ref, startErr)
	}
	if !errors.Is(cancelErr, execution.ErrUnavailable) || !reflect.DeepEqual(result, execution.Result{}) {
		t.Fatalf("unconfigured stop = %v, %v", result, cancelErr)
	}
	if !errors.Is(subscribeErr, execution.ErrUnavailable) || stream != nil {
		t.Fatalf("unconfigured subscription = %v, %v", stream, subscribeErr)
	}
	if err := s.End(ctx, scope, "session"); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatalf("unconfigured cleanup = %v", err)
	}
}

// This is a consumer-unit-test example, not a real Agent or fixture-model acceptance test.
func ExampleStub() {
	caller := execution.Caller{
		Kind: execution.CallerWorkflow, ID: "run-1", NodeID: "agent",
		Rounds: []execution.Round{{ContainerID: "loop", Index: 0}}, AttemptID: "attempt-1",
	}
	completed := execution.Result{
		Status: execution.StatusCompleted, Answer: "The complete answer.", StopReason: "end_turn",
	}
	svc := &executiontest.Stub{
		ReadAttemptFunc: func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
			return execution.Turn{Caller: c, Result: &completed}, nil
		},
	}
	turn, err := svc.ReadAttempt(context.Background(), execution.Scope{TenantID: "demo", UserID: "user"}, caller)
	if err != nil {
		panic(err)
	}
	// A workflow output uses Answer; StopReason is separate diagnostic information.
	if turn.Result != nil && turn.Result.Status == execution.StatusCompleted {
		fmt.Println(turn.Result.Answer)
		fmt.Println(turn.Result.StopReason)
	}
	// Output:
	// The complete answer.
	// end_turn
}
