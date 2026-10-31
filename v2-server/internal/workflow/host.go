package workflow

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Records is the workflow journal, separate from the execution service's history.
type Records interface {
	RecoverWorkflowRuns(context.Context) error
	GetWorkflowRun(context.Context, execution.Scope, string, string) (Record, error)
	BeginWorkflowRun(context.Context, execution.Scope, string, string) (Record, error)
	CommitWorkflowRun(context.Context, execution.Scope, string, string, int64, Checkpoint) (Record, error)
	RequestWorkflowRunCancel(context.Context, execution.Scope, string, string) (Record, error)
	InterruptWorkflowRun(context.Context, execution.Scope, string, string, int64) (Record, error)
}

// Host owns new-run workers; request cancellation stops admission/observation,
// never an admitted run. Construct it only AFTER execution startup recovery.
// Close joins its workers before the execution runtime or database can close.
type Host struct {
	records Records
	runtime execution.Service
	ctx     context.Context
	stop    context.CancelFunc
	mu      sync.Mutex
	closed  bool
	workers map[string]context.CancelCauseFunc
	ops     sync.WaitGroup
}

func NewHost(ctx context.Context, records Records, runtime execution.Service) (*Host, error) {
	if records == nil || runtime == nil {
		return nil, execution.ErrInvalidRequest
	}
	if err := records.RecoverWorkflowRuns(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Host{records: records, runtime: runtime, ctx: ctx, stop: cancel, workers: map[string]context.CancelCauseFunc{}}, nil
}

// Launch claims the durable pending row; a replayed HTTP admission response must
// never be passed directly to Run. Existing running/terminal rows are read-only.
func (h *Host) Launch(ctx context.Context, scope execution.Scope, workflowID, id string) (Record, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx.Err() != nil {
		return Record{}, execution.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(h.ctx, cancel)
	defer func() { stop(); cancel() }()
	r, err := h.records.GetWorkflowRun(op, scope, workflowID, id)
	if err != nil || r.Checkpoint.State.Status != "pending" {
		return r, err
	}
	r, err = h.records.BeginWorkflowRun(op, scope, workflowID, id)
	if err != nil {
		return r, err
	}
	worker, stopWorker := context.WithCancelCause(h.ctx)
	h.workers[id] = stopWorker
	h.ops.Go(func() {
		defer func() {
			stopWorker(nil)
			h.mu.Lock()
			delete(h.workers, id)
			h.mu.Unlock()
		}()
		revision := r.Revision
		_, err := Run(worker, h.runtime, r.Scope, r.Definition, func(ctx context.Context, c Checkpoint) error {
			next, err := h.records.CommitWorkflowRun(ctx, r.Scope, workflowID, id, revision, c)
			if err == nil {
				revision = next.Revision
				if next.CancelRequestedAt != nil {
					stopWorker(ErrUserCancelled)
				}
			}
			return err
		})
		if err != nil {
			// A cancelled Start response or failed receipt write can leave a live
			// attempt after Run returns. Resolve only that identity and confirm End;
			// never repair an unknown outcome by constructing another machine.
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(worker), 30*time.Second)
			_, settleErr := h.settleStopped(cleanup, r.Scope, workflowID, id)
			cancel()
			if settleErr != nil {
				// Unconfirmed stop/persistence retains the last durable fence.
				slog.Warn("workflow worker stopped before settlement", "run", id, "error", errors.Join(err, settleErr))
			}
		}
	})
	return r, nil
}

// Cancel records intent first. If an earlier worker already stopped on an error,
// reconcile that exact caller and confirm End instead of launching a replacement.
func (h *Host) Cancel(ctx context.Context, scope execution.Scope, workflowID, id string) (Record, error) {
	h.mu.Lock()
	if h.closed || h.ctx.Err() != nil {
		h.mu.Unlock()
		return Record{}, execution.ErrUnavailable
	}
	h.ops.Add(1)
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(h.ctx, cancel)
	defer func() { stop(); cancel(); h.ops.Done() }()
	r, err := h.records.RequestWorkflowRunCancel(op, scope, workflowID, id)
	worker := h.workers[id]
	if err == nil && worker != nil {
		worker(ErrUserCancelled)
	}
	h.mu.Unlock()
	if err != nil || worker != nil || r.FinishedAt != nil {
		return r, err
	}
	return h.settleStopped(op, scope, workflowID, id)
}

// Called only after this host's worker has stopped dispatching. Re-read because
// a failed write may have committed before its response was lost.
func (h *Host) settleStopped(ctx context.Context, scope execution.Scope, workflowID, id string) (Record, error) {
	r, err := h.records.GetWorkflowRun(ctx, scope, workflowID, id)
	if err != nil || r.FinishedAt != nil {
		return r, err
	}
	if r.Checkpoint.Active != nil {
		turn, err := h.runtime.ReadAttempt(ctx, scope, r.Checkpoint.Active.Caller)
		if err == nil {
			if turn.Ref.SessionID == "" || turn.Ref.TurnID == "" || (r.Checkpoint.Ref != (execution.Ref{}) && r.Checkpoint.Ref != turn.Ref) {
				return r, execution.ErrConflict
			}
			c := r.Checkpoint
			c.Ref = turn.Ref
			r, err = h.records.CommitWorkflowRun(ctx, scope, workflowID, id, r.Revision, c)
			if err != nil {
				return r, err
			}
			if err := h.runtime.End(ctx, scope, turn.Ref.SessionID); err != nil {
				return r, err
			}
		} else if !errors.Is(err, execution.ErrNotFound) || r.Checkpoint.Ref != (execution.Ref{}) {
			return r, err
		}
	}
	return h.records.InterruptWorkflowRun(ctx, scope, workflowID, id, r.Revision)
}

func (h *Host) Close() {
	h.mu.Lock()
	h.closed = true
	h.stop()
	h.mu.Unlock()
	h.ops.Wait()
}
