package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// ErrUserCancelled distinguishes an explicit stop from process shutdown. Pass it
// to a worker's CancelCauseFunc; neither cause permits replay after restart.
var ErrUserCancelled = errors.New("workflow cancelled by user")

// Checkpoint pairs graph state with the active attempt's execution receipt.
// Persistence must retain every nonempty receipt for node history, even after Active clears.
type Checkpoint struct {
	State  State
	Active *Attempt
	Ref    execution.Ref
}

// Commit must durably save a checkpoint before returning nil. A failure stops the
// runner, which never starts the next side effect on uncommitted graph state.
type Commit func(context.Context, Checkpoint) error

// Run executes a newly admitted definition in its caller-owned worker context.
// It reuses the application's one execution.Service, never constructs another runtime,
// and never starts an internal goroutine. It is not an HTTP handler or a recovery loop.
func Run(ctx context.Context, runtime execution.Service, scope execution.Scope, def Definition, commit Commit) (State, error) {
	if def.ID == "" || def.ProjectID == "" || scope.TenantID == "" || scope.UserID == "" {
		return State{}, execution.ErrInvalidRequest
	}
	m, err := NewMachine(def)
	if err != nil {
		return State{}, err
	}
	if runtime == nil || commit == nil {
		return State{}, execution.ErrInvalidRequest
	}
	for m.status == "running" {
		if ctx.Err() != nil {
			m.Interrupt(errors.Is(context.Cause(ctx), ErrUserCancelled))
			return commitTerminal(ctx, m, commit)
		}
		attempt := m.Step(time.Now())
		if err := commit(ctx, Checkpoint{State: m.Snapshot(), Active: attempt}); err != nil {
			return m.Snapshot(), err
		}
		if attempt == nil {
			if wait := m.retryWait(time.Now()); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
				case <-timer.C:
				}
				timer.Stop()
			}
			continue
		}
		// Cancellation may arrive while the durable intent is being written. Do
		// not call Start after that write if the worker was stopped meanwhile.
		if ctx.Err() != nil {
			m.Interrupt(errors.Is(context.Cause(ctx), ErrUserCancelled))
			return commitTerminal(ctx, m, commit)
		}
		ref, err := runtime.Start(ctx, scope, execution.StartRequest{Caller: attempt.Caller, Target: execution.Target{ProjectID: def.ProjectID}, ExecutorID: attempt.ExecutorID, Prompt: execution.TextPrompt(attempt.Prompt)})
		if err != nil {
			// Start can commit before its response is lost. Resolve that SAME identity;
			// never dispatch a replacement attempt to "repair" an uncertain response.
			lookup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			turn, readErr := runtime.ReadAttempt(lookup, scope, attempt.Caller)
			cancel()
			if readErr == nil {
				ref = turn.Ref
			} else {
				if !errors.Is(readErr, execution.ErrNotFound) {
					return m.Snapshot(), errors.Join(err, readErr)
				}
				_ = m.ApplyResult(attempt.Caller, execution.Result{Status: execution.StatusFailed, FailureCode: startFailureCode(err)}, time.Now())
				if ctx.Err() != nil {
					m.Interrupt(errors.Is(context.Cause(ctx), ErrUserCancelled))
					return commitTerminal(ctx, m, commit)
				}
				if err := commit(ctx, Checkpoint{State: m.Snapshot()}); err != nil {
					return m.Snapshot(), err
				}
				continue
			}
		}
		if err := commit(ctx, Checkpoint{State: m.Snapshot(), Active: attempt, Ref: ref}); err != nil {
			return m.Snapshot(), errors.Join(err, endSession(ctx, runtime, scope, ref.SessionID))
		}
		result, waitErr := waitResult(ctx, runtime, scope, attempt.Caller, ref)
		// A turn result alone does not release a workflow's shared checkout. End must
		// confirm shutdown before the result commits and another node may start.
		if err := endSession(ctx, runtime, scope, ref.SessionID); err != nil {
			return m.Snapshot(), err
		}
		if ctx.Err() != nil {
			m.Interrupt(errors.Is(context.Cause(ctx), ErrUserCancelled))
			return commitTerminal(ctx, m, commit)
		}
		if waitErr != nil {
			result = execution.Result{Status: execution.StatusInterrupted, FailureCode: "interrupted"}
		}
		if err := m.ApplyResult(attempt.Caller, result, time.Now()); err != nil {
			return m.Snapshot(), err
		}
		if err := commit(ctx, Checkpoint{State: m.Snapshot()}); err != nil {
			return m.Snapshot(), err
		}
	}
	return m.Snapshot(), nil
}

func commitTerminal(ctx context.Context, m *Machine, commit Commit) (State, error) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	state := m.Snapshot()
	return state, commit(cleanup, Checkpoint{State: state})
}

func endSession(ctx context.Context, runtime execution.Service, scope execution.Scope, id string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return runtime.End(cleanup, scope, id)
}

func waitResult(ctx context.Context, runtime execution.Service, scope execution.Scope, caller execution.Caller, ref execution.Ref) (execution.Result, error) {
	stream, err := runtime.Subscribe(ctx, scope, ref.SessionID, 0)
	if err == nil {
		defer stream.Close()
		for {
			event, e := stream.Next(ctx)
			if e != nil {
				err = e
				break
			}
			if event.Kind == execution.EventTurnEnded && event.Ref == ref && event.Result != nil {
				return *event.Result, nil
			}
		}
	}
	if ctx.Err() != nil {
		return execution.Result{}, ctx.Err()
	}
	// A dropped observer is not evidence that the Agent failed. Read the retained
	// authoritative result once; if still running, stop it rather than replaying it.
	turn, e := runtime.ReadAttempt(ctx, scope, caller)
	if e == nil && turn.Ref == ref && turn.Result != nil {
		return *turn.Result, nil
	}
	return execution.Result{}, errors.Join(err, e)
}

func (m *Machine) retryWait(now time.Time) time.Duration {
	if id, ok := workflowRetryingAgentNode(m.states); ok {
		at, err := time.Parse(time.RFC3339Nano, m.states[id].RetryAt)
		if err == nil && at.After(now) {
			return at.Sub(now)
		}
	}
	return 0
}

func startFailureCode(err error) string {
	switch {
	case errors.Is(err, execution.ErrUnavailable):
		return "execution_unavailable"
	case errors.Is(err, execution.ErrNotFound):
		return "execution_not_found"
	case errors.Is(err, execution.ErrWorkspaceBusy):
		return "workspace_busy"
	case errors.Is(err, execution.ErrConflict):
		return "execution_conflict"
	case errors.Is(err, execution.ErrUnsupported):
		return "execution_unsupported"
	default:
		return "execution_start_failed"
	}
}
