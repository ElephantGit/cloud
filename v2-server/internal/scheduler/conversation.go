package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// RecordStore is the durable occurrence state consumed by ConversationAdapter.
type RecordStore interface {
	GetOccurrence(context.Context, string, string, string) (store.Occurrence, error)
	ApplyOccurrenceEvidence(context.Context, string, string, string, int64, store.ExecutionEvidence) (store.Occurrence, error)
}

// ConversationAdapter routes immutable chat occurrences to shared model or Agent execution.
// It owns no provider, execution history, polling goroutine, or automatic retry queue.
type ConversationAdapter struct {
	records  RecordStore
	executor execution.Service
	mu       sync.Mutex
	active   map[string]bool
}

// NewConversationAdapter requires explicit dependencies; an unconfigured execution Stub remains unavailable.
func NewConversationAdapter(records RecordStore, executor execution.Service) (*ConversationAdapter, error) {
	if records == nil || executor == nil {
		return nil, errors.New("schedule conversation adapter requires records and execution")
	}
	return &ConversationAdapter{records: records, executor: executor, active: make(map[string]bool)}, nil
}

// ErrOperationInProgress means another local operation owns this occurrence.
// The caller can observe again on the next tick; no operation is queued.
var ErrOperationInProgress = errors.New("schedule operation in progress")

// enter prevents a lookup from overtaking an in-process Start. Only one adapter may
// own these occurrences; recovery runs after the public service's startup barrier.
func (a *ConversationAdapter) enter(id string) (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[id] {
		return nil, ErrOperationInProgress
	}
	a.active[id] = true
	return func() { a.mu.Lock(); delete(a.active, id); a.mu.Unlock() }, nil
}

func occurrenceCaller(o store.Occurrence) execution.Caller {
	return execution.Caller{Kind: execution.CallerSchedule, ID: o.ID, AttemptID: o.ID}
}

func (a *ConversationAdapter) get(ctx context.Context, scope execution.Scope, id string) (store.Occurrence, error) {
	return a.records.GetOccurrence(ctx, scope.TenantID, scope.UserID, id)
}

func (a *ConversationAdapter) apply(ctx context.Context, scope execution.Scope, o store.Occurrence, event, ref, reason string) (store.Occurrence, error) {
	return a.records.ApplyOccurrenceEvidence(ctx, scope.TenantID, scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: ref, Reason: reason})
}

// Dispatch reserves a recorded occurrence before issuing one StartModel or Start call.
// Duplicate dispatch attempts never issue a second admission, including after a local persistence failure.
func (a *ConversationAdapter) Dispatch(ctx context.Context, scope execution.Scope, occurrence store.Occurrence) error {
	leave, err := a.enter(occurrence.ID)
	if err != nil {
		return err
	}
	defer leave()
	o, err := a.get(ctx, scope, occurrence.ID)
	if err != nil {
		return err
	}
	if o.Status != "recorded" {
		return nil
	}
	o, err = a.apply(ctx, scope, o, "begin", "", "")
	if err != nil {
		return err
	}
	if o.Status != "starting" {
		return nil
	}
	var snapshot store.ScheduleConfig
	_ = json.Unmarshal(o.Snapshot, &snapshot) // The request builders reject malformed snapshots.
	var start func(context.Context) (execution.Ref, error)
	var code string
	if snapshot.ProjectID == "" {
		var request execution.ModelStartRequest
		request, code = modelRequest(o)
		start = func(ctx context.Context) (execution.Ref, error) { return a.executor.StartModel(ctx, scope, request) }
	} else {
		var request execution.StartRequest
		request, code = agentRequest(o)
		start = func(ctx context.Context) (execution.Ref, error) { return a.executor.Start(ctx, scope, request) }
	}
	if code != "" {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err = a.apply(writeCtx, scope, o, "rejected", "", code)
		return err
	}
	ref, startErr := start(ctx)
	// Admission may win just before the request context ends. Persist the result with a
	// bounded independent context; otherwise a cancelled observer could lose the binding.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	o, err = a.get(writeCtx, scope, o.ID)
	if err != nil {
		return errors.Join(startErr, err)
	}
	if startErr != nil {
		if o.Released || o.ExecutionRef != "" {
			// Never replace a persisted execution binding or terminal record.
			return startErr
		}
		event, reason := admissionFailure(startErr)
		_, err = a.apply(writeCtx, scope, o, event, "", reason)
		return errors.Join(startErr, err)
	}
	if !validRef(ref) {
		_, err = a.apply(writeCtx, scope, o, "reconcile", "", "lookup_required")
		return errors.Join(execution.ErrConflict, err)
	}
	encoded, _ := json.Marshal(ref)
	if o.ExecutionRef == string(encoded) {
		// A durable binding is authoritative even if a previous local write reported an error.
		return nil
	}
	if o.ExecutionRef != "" {
		return execution.ErrConflict
	}
	_, err = a.apply(writeCtx, scope, o, "accepted", string(encoded), "")
	return err
}

func modelRequest(o store.Occurrence) (execution.ModelStartRequest, string) {
	var config store.ScheduleConfig
	if json.Unmarshal(o.Snapshot, &config) != nil {
		return execution.ModelStartRequest{}, "invalid_snapshot"
	}
	var target struct{ Kind, Prompt, Model, ExecutorID string }
	if json.Unmarshal(config.Target, &target) != nil || target.Kind != "chat" || config.ProjectID != "" {
		return execution.ModelStartRequest{}, "unsupported_schedule_target"
	}
	if strings.TrimSpace(target.Model) == "" {
		return execution.ModelStartRequest{}, "model_required"
	}
	if target.ExecutorID != "" {
		return execution.ModelStartRequest{}, "project_required"
	}
	prompt := execution.TextPrompt(target.Prompt)
	if _, err := prompt.Text(); err != nil {
		return execution.ModelStartRequest{}, "invalid_prompt"
	}
	return execution.ModelStartRequest{Caller: occurrenceCaller(o), ModelID: target.Model, Prompt: prompt}, ""
}

// agentRequest requires a catalog executor ID, never a guessed binding of display names.
// The shared service authorizes the selected project and creates an independent directory for this new session.
func agentRequest(o store.Occurrence) (execution.StartRequest, string) {
	var config store.ScheduleConfig
	if json.Unmarshal(o.Snapshot, &config) != nil {
		return execution.StartRequest{}, "invalid_snapshot"
	}
	var target struct{ Kind, Prompt, ExecutorID string }
	if json.Unmarshal(config.Target, &target) != nil || target.Kind != "chat" || strings.TrimSpace(config.ProjectID) == "" {
		return execution.StartRequest{}, "unsupported_schedule_target"
	}
	if strings.TrimSpace(target.ExecutorID) == "" {
		return execution.StartRequest{}, "executor_required"
	}
	prompt := execution.TextPrompt(target.Prompt)
	if _, err := prompt.Text(); err != nil {
		return execution.StartRequest{}, "invalid_prompt"
	}
	return execution.StartRequest{Caller: occurrenceCaller(o), Target: execution.Target{ProjectID: config.ProjectID}, ExecutorID: target.ExecutorID, Prompt: prompt}, ""
}

func admissionFailure(err error) (string, string) {
	switch {
	case errors.Is(err, execution.ErrConflict):
		return "reconcile", "lookup_required"
	case errors.Is(err, context.Canceled):
		return "reconcile", "lookup_required"
	case errors.Is(err, context.DeadlineExceeded):
		return "reconcile", "lookup_required"
	case errors.Is(err, execution.ErrWorkspaceBusy):
		return "rejected", "execution_busy"
	case errors.Is(err, execution.ErrUnavailable):
		return "rejected", "execution_unavailable"
	case errors.Is(err, execution.ErrInvalidRequest):
		return "rejected", "invalid_execution_request"
	case errors.Is(err, execution.ErrUnsupported):
		return "rejected", "execution_unsupported"
	case errors.Is(err, execution.ErrNotFound):
		return "rejected", "execution_target_unavailable"
	default:
		return "reconcile", "lookup_required"
	}
}

func validRef(ref execution.Ref) bool { return ref.SessionID != "" && ref.TurnID != "" }
func storedRef(o store.Occurrence) (execution.Ref, error) {
	var ref execution.Ref
	if json.Unmarshal([]byte(o.ExecutionRef), &ref) != nil || !validRef(ref) {
		return ref, execution.ErrConflict
	}
	return ref, nil
}
func isResult(status string) bool {
	return status == "succeeded" || status == "failed" || status == "cancelled" || status == "interrupted"
}

// Reconcile follows the original caller identity without submitting another prompt.
// After Start has returned (or startup recovery), a missing unbound attempt can be closed.
// Read errors and missing previously bound turns retain the task slot.
// A terminal result still requires End confirmation before the task's slot is released.
func (a *ConversationAdapter) Reconcile(ctx context.Context, scope execution.Scope, occurrence store.Occurrence) error {
	leave, err := a.enter(occurrence.ID)
	if err != nil {
		return err
	}
	defer leave()
	o, err := a.get(ctx, scope, occurrence.ID)
	if err != nil {
		return err
	}
	if o.Released || o.Status == "recorded" {
		return nil
	}
	if isResult(o.Status) {
		return a.end(ctx, scope, o)
	}
	turn, err := a.executor.ReadAttempt(ctx, scope, occurrenceCaller(o))
	if errors.Is(err, execution.ErrNotFound) && o.ExecutionRef == "" {
		_, err = a.apply(ctx, scope, o, "not_accepted", "", "execution_not_started")
		return err
	}
	if err != nil {
		return err
	}
	c := turn.Caller
	if c.Kind != execution.CallerSchedule || c.ID != o.ID || c.AttemptID != o.ID || c.NodeID != "" || len(c.Rounds) != 0 || !validRef(turn.Ref) {
		return fmt.Errorf("execution returned a different attempt: %w", execution.ErrConflict)
	}
	encoded, _ := json.Marshal(turn.Ref)
	if o.ExecutionRef != "" && o.ExecutionRef != string(encoded) {
		return execution.ErrConflict
	}
	if o.Status == "starting" || o.Status == "reconciling" {
		o, err = a.apply(ctx, scope, o, "accepted", string(encoded), "")
		if err != nil {
			return err
		}
	}
	if turn.Result == nil {
		return nil
	}
	return a.finish(ctx, scope, o, turn.Ref, *turn.Result)
}

func resultEvent(r execution.Result) (string, string, error) {
	reason := r.FailureCode
	if len(reason) > 256 {
		reason = "execution_failed"
	}
	switch r.Status {
	case execution.StatusCompleted:
		if !r.AnswerComplete {
			return "failed", "result_incomplete", nil
		}
		return "completed", "", nil
	case execution.StatusFailed:
		return "failed", reason, nil
	case execution.StatusCancelled:
		return "cancelled", reason, nil
	case execution.StatusInterrupted:
		return "interrupted", reason, nil
	default:
		return "", "", execution.ErrInvalidRequest
	}
}

func (a *ConversationAdapter) finish(ctx context.Context, scope execution.Scope, previous store.Occurrence, ref execution.Ref, result execution.Result) error {
	event, reason, err := resultEvent(result)
	if err != nil {
		return err
	}
	o, err := a.get(ctx, scope, previous.ID)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(ref)
	if o.ExecutionRef != string(encoded) {
		return execution.ErrConflict
	}
	if isResult(o.Status) {
		return a.end(ctx, scope, o)
	}
	o, err = a.apply(ctx, scope, o, event, string(encoded), reason)
	if err != nil {
		return err
	}
	return a.end(ctx, scope, o)
}

func (a *ConversationAdapter) end(ctx context.Context, scope execution.Scope, o store.Occurrence) error {
	if o.Released {
		return nil
	}
	ref, err := storedRef(o)
	if err != nil {
		return err
	}
	if err = a.executor.End(ctx, scope, ref.SessionID); err != nil {
		return err
	}
	o, err = a.get(ctx, scope, o.ID)
	if err != nil {
		return err
	}
	if o.Released {
		return nil
	}
	if !isResult(o.Status) {
		return store.ErrConflict
	}
	encoded, _ := json.Marshal(ref)
	_, err = a.apply(ctx, scope, o, "released", string(encoded), "")
	return err
}

// Cancel follows the shared mode semantics: Agent ends its session, model-only stops its turn.
// A schedule always ends its dedicated session afterward; End preserves history and directory.
// Unbound occurrences must be reconciled first, never replaced.
func (a *ConversationAdapter) Cancel(ctx context.Context, scope execution.Scope, occurrence store.Occurrence) error {
	leave, err := a.enter(occurrence.ID)
	if err != nil {
		return err
	}
	defer leave()
	o, err := a.get(ctx, scope, occurrence.ID)
	if err != nil {
		return err
	}
	if o.Released {
		return nil
	}
	if isResult(o.Status) {
		return a.end(ctx, scope, o)
	}
	if o.Status != "running" && o.Status != "cancelling" {
		return execution.ErrConflict
	}
	ref, err := storedRef(o)
	if err != nil {
		return err
	}
	if o.Status == "running" {
		o, err = a.apply(ctx, scope, o, "cancel_requested", "", "")
		if err != nil {
			return err
		}
	}
	result, err := a.executor.Cancel(ctx, scope, ref)
	if err != nil {
		return err
	}
	return a.finish(ctx, scope, o, ref, result)
}
