package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestScheduleCoordinatorUsesSharedOptionsExecution(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	token, err := s.Login(ctx, store.Identity{Source: "test", Subject: "scheduler", DisplayName: "Scheduler"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := s.Tenants(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
	config, err := s.CreateScheduleConfig(ctx, scope.TenantID, scope.UserID, store.ScheduleInput{Name: "shared service", Target: json.RawMessage(`{"kind":"chat","model":"fixture","prompt":"hello"}`), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return config.CreatedAt.Add(time.Minute) }
	if _, err = server.NewScheduleCoordinator(s, server.Options{}, clock); err == nil {
		t.Fatal("missing service silently accepted")
	}
	stub := &executiontest.Stub{}
	options := server.Options{Execution: stub, Catalog: stub}
	c, err := server.NewScheduleCoordinator(s, options, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Recover(ctx, scope); err != nil {
		t.Fatal(err)
	}
	o, err := c.Manual(ctx, scope, config.ID, "unconfigured")
	if !errors.Is(err, execution.ErrUnavailable) || o.Status != "skipped" || o.Reason != "execution_unavailable" {
		t.Fatalf("unconfigured: %+v %v", o, err)
	}
	var caller execution.Caller
	ref := execution.Ref{SessionID: "shared-session", TurnID: "shared-turn"}
	stub.StartModelFunc = func(_ context.Context, got execution.Scope, req execution.ModelStartRequest) (execution.Ref, error) {
		if got != scope {
			t.Fatal("wrong scope")
		}
		caller = req.Caller
		return ref, nil
	}
	stub.ReadAttemptFunc = func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error) {
		return execution.Turn{Caller: caller, Ref: ref, Result: &execution.Result{Status: execution.StatusCompleted, AnswerComplete: true}}, nil
	}
	ends := 0
	stub.EndFunc = func(context.Context, execution.Scope, string) error { ends++; return nil }
	o, err = c.Manual(ctx, scope, config.ID, "configured")
	if err != nil || o.Status != "running" || caller.ID != o.ID {
		t.Fatalf("shared dependency: %+v %v", o, err)
	}
	o, err = c.Manual(ctx, scope, config.ID, "configured")
	if err != nil || o.Status != "succeeded" || !o.Released || ends != 1 {
		t.Fatalf("shared cleanup: %+v %v ends=%d", o, err, ends)
	}
}
