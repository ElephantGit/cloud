package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/taskrun"
)

// TaskRecords finds the original run by the occurrence's stable start key.
type TaskRecords interface {
	TaskRunByKey(context.Context, string, string, string, string) (store.TaskRun, error)
}

type taskAdapter struct {
	records RecordStore
	runs    TaskRecords
	host    *taskrun.Host
	service execution.Service
	locks   sync.Map
}

// NewTaskAdapter dispatches only through the collaboration host and follows the original turn.
func NewTaskAdapter(records RecordStore, runs TaskRecords, host *taskrun.Host, service execution.Service) Runner {
	return &taskAdapter{records: records, runs: runs, host: host, service: service}
}

type taskRef struct {
	TaskID string `json:"taskId"`
	RunID  string `json:"taskRunId"`
}

// TaskRunOf identifies a collaboration reference for history routing.
func TaskRunOf(raw string) bool {
	var r taskRef
	return json.Unmarshal([]byte(raw), &r) == nil && r.TaskID != "" && r.RunID != ""
}
func (a *taskAdapter) handle(ctx context.Context, s execution.Scope, item store.Occurrence, action string) error {
	if _, busy := a.locks.LoadOrStore(item.ID, true); busy {
		return ErrOperationInProgress
	}
	defer a.locks.Delete(item.ID)
	o, err := a.records.GetOccurrence(ctx, s.TenantID, s.UserID, item.ID)
	if err != nil {
		return err
	}
	proof := func(event, ref, reason string) error {
		var e error
		o, e = a.records.ApplyOccurrenceEvidence(ctx, s.TenantID, s.UserID, item.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: ref, Reason: reason})
		return e
	}
	var cfg store.ScheduleConfig
	var t taskrun.ScheduledTarget
	if err = json.Unmarshal(o.Snapshot, &cfg); err != nil {
		return err
	}
	if err = json.Unmarshal(cfg.Target, &t); err != nil {
		return err
	}
	key := store.ScheduledTaskStartPrefix + item.ID
	if action == "dispatch" {
		if o.Status != "recorded" {
			return nil
		}
		if err = proof("begin", "", ""); err != nil || o.Status != "starting" {
			return err
		}
		if t.TaskID == "" || t.ExecutorID == "" {
			return proof("rejected", "", "unsupported_schedule_target")
		}
		r, e := a.host.StartScheduled(ctx, s, t, cfg.ProjectID, key)
		if e != nil {
			reason := ""
			switch {
			case errors.Is(e, taskrun.ErrScheduledStartUnknown): // Reconcile only this identity.
			case errors.Is(e, store.ErrNotFound), errors.Is(e, taskrun.ErrNotAssignee):
				reason = "task_assignment_changed"
			case errors.Is(e, taskrun.ErrScheduleTaskChanged), errors.Is(e, store.ErrConflict):
				reason = "task_no_longer_actionable"
			case errors.Is(e, store.ErrRunActive), errors.Is(e, execution.ErrWorkspaceBusy):
				reason = "execution_busy"
			case errors.Is(e, execution.ErrUnavailable):
				reason = "execution_unavailable"
			case errors.Is(e, taskrun.ErrNoExecutor):
				reason = "executor_required"
			}
			if reason != "" {
				return proof("rejected", "", reason)
			}
			return errors.Join(e, proof("reconcile", "", "lookup_required"))
		}
		ref, _ := json.Marshal(taskRef{t.TaskID, r.ID})
		return proof("accepted", string(ref), "")
	}
	if o.Released {
		return nil
	}
	if isResult(o.Status) {
		return proof("released", o.ExecutionRef, "")
	}
	r, e := a.runs.TaskRunByKey(ctx, s.TenantID, t.TaskID, s.UserID, key)
	if e == nil && r.SessionID == "" {
		r, e = a.host.RecoverScheduledStart(ctx, s, t.TaskID, key)
	}
	if errors.Is(e, store.ErrNotFound) && o.ExecutionRef == "" {
		return proof("not_accepted", "", "execution_not_started")
	}
	if e != nil {
		return e
	}
	ref, _ := json.Marshal(taskRef{t.TaskID, r.ID})
	if o.ExecutionRef != "" && o.ExecutionRef != string(ref) {
		return execution.ErrConflict
	}
	if o.Status == "starting" || o.Status == "reconciling" {
		if err := proof("accepted", string(ref), ""); err != nil {
			return err
		}
	}
	// Follow the first turn by caller identity: later manual follow-ups on the same
	// session cannot change this occurrence's outcome or be stopped by its cancel.
	turn, e := a.service.ReadAttempt(ctx, s, execution.Caller{Kind: execution.CallerTask, ID: r.ID, AttemptID: key})
	if errors.Is(e, execution.ErrNotFound) && r.EndReason == store.TaskRunInterrupted {
		if err := proof("interrupted", string(ref), "server_restarted"); err != nil {
			return err
		}
		return proof("released", string(ref), "")
	}
	if e != nil {
		return e
	}
	if turn.Result == nil {
		if action == "cancel" {
			if o.Status == "running" {
				if err := proof("cancel_requested", "", ""); err != nil {
					return err
				}
			}
			// Cancel names the exact turn, never a later follow-up on this task.
			return a.host.StopScheduledTurn(ctx, s, t.TaskID, r.ID, key, turn.Ref.TurnID)
		}
		return nil
	}
	event, reason := "completed", ""
	switch turn.Result.Status {
	case execution.StatusCompleted:
	case execution.StatusCancelled:
		event = "cancelled"
	case execution.StatusInterrupted:
		event = "interrupted"
		reason = "server_restarted"
	default:
		event = "failed"
		reason = "execution_failed"
	}
	if err := proof(event, string(ref), reason); err != nil {
		return err
	}
	return proof("released", string(ref), "")
}
func (a *taskAdapter) Dispatch(ctx context.Context, s execution.Scope, o store.Occurrence) error {
	return a.handle(ctx, s, o, "dispatch")
}
func (a *taskAdapter) Reconcile(ctx context.Context, s execution.Scope, o store.Occurrence) error {
	return a.handle(ctx, s, o, "reconcile")
}
func (a *taskAdapter) Cancel(ctx context.Context, s execution.Scope, o store.Occurrence) error {
	return a.handle(ctx, s, o, "cancel")
}
