package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
)

func TestExecutionOptionsDefaultToUnavailable(t *testing.T) {
	h := newHandler(nil, Options{})
	service, ok := h.opts.Execution.(*executiontest.Stub)
	if !ok || h.opts.Catalog != service {
		t.Fatal("omitted execution and catalog must share the unavailable Stub")
	}
	ctx, scope := t.Context(), execution.Scope{TenantID: "tenant", UserID: "user"}
	if ref, err := h.opts.Execution.Start(ctx, scope, execution.StartRequest{}); !errors.Is(err, execution.ErrUnavailable) || ref != (execution.Ref{}) {
		t.Fatalf("unconfigured execution = %v, %v", ref, err)
	}
	if models, err := h.opts.Catalog.ListModels(ctx, scope); !errors.Is(err, execution.ErrUnavailable) || models != nil {
		t.Fatalf("unconfigured models = %v, %v", models, err)
	}
	if executors, err := h.opts.Catalog.ListExecutors(ctx, scope); !errors.Is(err, execution.ErrUnavailable) || executors != nil {
		t.Fatalf("unconfigured executors = %v, %v", executors, err)
	}
}

func TestExecutionOptionsKeepExplicitDependencies(t *testing.T) {
	ctx, scope := t.Context(), execution.Scope{TenantID: "tenant", UserID: "user"}
	wantRef := execution.Ref{SessionID: "session", TurnID: "turn"}
	wantModels := []execution.Model{{ID: "fixture", Name: "Fixture", BindingVersion: "fixture-v1"}}
	service := &executiontest.Stub{StartFunc: func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
		return wantRef, nil
	}}
	catalog := &executiontest.Stub{ListModelsFunc: func(context.Context, execution.Scope) ([]execution.Model, error) {
		return wantModels, nil
	}}
	h := newHandler(nil, Options{Execution: service, Catalog: catalog})
	if h.opts.Execution != service || h.opts.Catalog != catalog {
		t.Fatal("explicit execution or catalog was replaced")
	}
	if ref, err := h.opts.Execution.Start(ctx, scope, execution.StartRequest{}); err != nil || ref != wantRef {
		t.Fatalf("custom execution = %v, %v", ref, err)
	}
	if models, err := h.opts.Catalog.ListModels(ctx, scope); err != nil || !reflect.DeepEqual(models, wantModels) {
		t.Fatalf("custom catalog = %v, %v", models, err)
	}

	// A service may also implement Catalog; that does not configure the omitted field.
	h = newHandler(nil, Options{Execution: catalog})
	if h.opts.Execution != catalog || h.opts.Catalog == catalog {
		t.Fatal("execution must not implicitly configure catalog")
	}
	if _, err := h.opts.Catalog.ListModels(ctx, scope); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatalf("omitted catalog = %v", err)
	}
	h = newHandler(nil, Options{Catalog: service})
	if h.opts.Catalog != service || h.opts.Execution == service {
		t.Fatal("catalog must not implicitly configure execution")
	}
	if _, err := h.opts.Execution.Start(ctx, scope, execution.StartRequest{}); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatalf("omitted execution = %v", err)
	}
}
