// Package executiontest supplies explicit callback doubles for page-service tests.
// It is not a fixture model, Node runtime, persistent history, or production service.
package executiontest

import (
	"context"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Stub delegates only the operations configured by a test. Unconfigured operations fail
// with ErrUnavailable so a missing backend can never masquerade as a successful Agent run.
// Configure callbacks before sharing a Stub; each callback owns its synchronization and semantics.
type Stub struct {
	StartFunc         func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error)
	StartModelFunc    func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error)
	AppendFunc        func(context.Context, execution.Scope, execution.AppendRequest) (execution.Ref, error)
	SubscribeFunc     func(context.Context, execution.Scope, string, uint64) (execution.Stream, error)
	CancelFunc        func(context.Context, execution.Scope, execution.Ref) (execution.Result, error)
	EndFunc           func(context.Context, execution.Scope, string) error
	ReadSessionFunc   func(context.Context, execution.Scope, string) (execution.Session, error)
	ReadTurnsFunc     func(context.Context, execution.Scope, string, string, int) (execution.TurnPage, error)
	ReadAttemptFunc   func(context.Context, execution.Scope, execution.Caller) (execution.Turn, error)
	ListExecutorsFunc func(context.Context, execution.Scope) ([]execution.Executor, error)
	ListModelsFunc    func(context.Context, execution.Scope) ([]execution.Model, error)
}

var _ execution.Service = (*Stub)(nil)
var _ execution.Catalog = (*Stub)(nil)

// StartModel delegates model-only admission to StartModelFunc.
func (s *Stub) StartModel(ctx context.Context, scope execution.Scope, req execution.ModelStartRequest) (execution.Ref, error) {
	if s.StartModelFunc == nil {
		return execution.Ref{}, execution.ErrUnavailable
	}
	return s.StartModelFunc(ctx, scope, req)
}

// ListExecutors delegates configured Agent choices to ListExecutorsFunc.
func (s *Stub) ListExecutors(ctx context.Context, scope execution.Scope) ([]execution.Executor, error) {
	if s.ListExecutorsFunc == nil {
		return nil, execution.ErrUnavailable
	}
	return s.ListExecutorsFunc(ctx, scope)
}

// ListModels delegates model-only choices to ListModelsFunc.
func (s *Stub) ListModels(ctx context.Context, scope execution.Scope) ([]execution.Model, error) {
	if s.ListModelsFunc == nil {
		return nil, execution.ErrUnavailable
	}
	return s.ListModelsFunc(ctx, scope)
}

// Start delegates admission to StartFunc.
func (s *Stub) Start(ctx context.Context, scope execution.Scope, req execution.StartRequest) (execution.Ref, error) {
	if s.StartFunc == nil {
		return execution.Ref{}, execution.ErrUnavailable
	}
	return s.StartFunc(ctx, scope, req)
}

// Append delegates admission to AppendFunc.
func (s *Stub) Append(ctx context.Context, scope execution.Scope, req execution.AppendRequest) (execution.Ref, error) {
	if s.AppendFunc == nil {
		return execution.Ref{}, execution.ErrUnavailable
	}
	return s.AppendFunc(ctx, scope, req)
}

// Subscribe delegates observation to SubscribeFunc.
func (s *Stub) Subscribe(ctx context.Context, scope execution.Scope, sessionID string, after uint64) (execution.Stream, error) {
	if s.SubscribeFunc == nil {
		return nil, execution.ErrUnavailable
	}
	return s.SubscribeFunc(ctx, scope, sessionID, after)
}

// Cancel delegates stop confirmation to CancelFunc.
func (s *Stub) Cancel(ctx context.Context, scope execution.Scope, ref execution.Ref) (execution.Result, error) {
	if s.CancelFunc == nil {
		return execution.Result{}, execution.ErrUnavailable
	}
	return s.CancelFunc(ctx, scope, ref)
}

// End delegates cleanup confirmation to EndFunc.
func (s *Stub) End(ctx context.Context, scope execution.Scope, sessionID string) error {
	if s.EndFunc == nil {
		return execution.ErrUnavailable
	}
	return s.EndFunc(ctx, scope, sessionID)
}

// ReadSession delegates metadata lookup to ReadSessionFunc.
func (s *Stub) ReadSession(ctx context.Context, scope execution.Scope, sessionID string) (execution.Session, error) {
	if s.ReadSessionFunc == nil {
		return execution.Session{}, execution.ErrUnavailable
	}
	return s.ReadSessionFunc(ctx, scope, sessionID)
}

// ReadTurns delegates history pagination to ReadTurnsFunc.
func (s *Stub) ReadTurns(ctx context.Context, scope execution.Scope, sessionID, cursor string, limit int) (execution.TurnPage, error) {
	if s.ReadTurnsFunc == nil {
		return execution.TurnPage{}, execution.ErrUnavailable
	}
	return s.ReadTurnsFunc(ctx, scope, sessionID, cursor, limit)
}

// ReadAttempt delegates history lookup to ReadAttemptFunc.
func (s *Stub) ReadAttempt(ctx context.Context, scope execution.Scope, caller execution.Caller) (execution.Turn, error) {
	if s.ReadAttemptFunc == nil {
		return execution.Turn{}, execution.ErrUnavailable
	}
	return s.ReadAttemptFunc(ctx, scope, caller)
}
