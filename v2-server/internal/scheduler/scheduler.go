// Package scheduler coordinates saved schedules with an injected execution adapter.
// Host connects the coordinator to a process-owned ticker and authenticated commands.
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
	"github.com/wanglongan587/cloud/v2-server/internal/schedule"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ErrRecoveryRequired prevents scheduling before durable startup recovery has run.
var ErrRecoveryRequired = errors.New("schedule recovery required")

// CoordinatorStore supplies task-domain state, never public Session/Turn persistence.
type CoordinatorStore interface {
	ScheduleConfigs(context.Context, string, string) ([]store.ScheduleConfig, error)
	RecordOccurrence(context.Context, string, string, string, store.Trigger, time.Time) (store.Occurrence, error)
	GetOccurrence(context.Context, string, string, string) (store.Occurrence, error)
	FindManualOccurrence(context.Context, string, string, string, string) (store.Occurrence, error)
	ActiveOccurrences(context.Context, string, string) ([]store.Occurrence, error)
	RecoverableOccurrences(context.Context, string, string) ([]store.Occurrence, error)
	ApplyOccurrenceEvidence(context.Context, string, string, string, int64, store.ExecutionEvidence) (store.Occurrence, error)
}

// Runner consumes one stable occurrence. Reconcile observes and cleans up; it never starts work.
type Runner interface {
	Dispatch(context.Context, execution.Scope, store.Occurrence) error
	Reconcile(context.Context, execution.Scope, store.Occurrence) error
	Cancel(context.Context, execution.Scope, store.Occurrence) error
}

var _ CoordinatorStore = (*store.Store)(nil)
var _ Runner = (*ConversationAdapter)(nil)

type scheduleCursor struct {
	version int64
	enabled bool
	next    time.Time
}

type scopeCursor struct {
	mu    sync.Mutex
	ready bool
	tasks map[string]scheduleCursor
}

// Coordinator has a caller-driven lifecycle: Recover once per scope, then Tick as needed.
// It owns no goroutines. Each scope serializes its calls; different scopes are independent.
// One Coordinator must own scheduling in this single-process deployment.
type Coordinator struct {
	records CoordinatorStore
	runner  Runner
	now     func() time.Time
	mu      sync.Mutex
	scopes  map[execution.Scope]*scopeCursor
}

// NewCoordinator injects persistence, execution and the service clock explicitly.
func NewCoordinator(records CoordinatorStore, runner Runner, now func() time.Time) (*Coordinator, error) {
	if records == nil || runner == nil || now == nil {
		return nil, errors.New("scheduler requires records, runner and clock")
	}
	return &Coordinator{records: records, runner: runner, now: now, scopes: make(map[execution.Scope]*scopeCursor)}, nil
}

func (c *Coordinator) scope(scope execution.Scope) (*scopeCursor, error) {
	if scope.TenantID == "" || scope.UserID == "" {
		return nil, errors.New("scheduler requires tenant and user")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.scopes[scope]
	if s == nil {
		s = &scopeCursor{tasks: make(map[string]scheduleCursor)}
		c.scopes[scope] = s
	}
	return s, nil
}

// Recover closes unsent old records and marks possibly admitted ones for reconciliation.
// Durable state errors keep the scope closed. Execution lookup errors are returned but do not
// block unrelated tasks: the unresolved occurrence retains its database overlap protection.
// Repeating Recover after success is harmless and does not reset live scheduling cursors.
func (c *Coordinator) Recover(ctx context.Context, scope execution.Scope) error {
	s, err := c.scope(scope)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	items, err := c.records.RecoverableOccurrences(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return err
	}
	var observedErrors []error
	for _, item := range items {
		updated, err := c.records.ApplyOccurrenceEvidence(ctx, scope.TenantID, scope.UserID, item.ID, item.Version, store.ExecutionEvidence{Event: "recover"})
		if err != nil {
			return fmt.Errorf("recover occurrence %s: %w", item.ID, err)
		}
		if !updated.Released {
			if err := c.runner.Reconcile(ctx, scope, updated); err != nil {
				observedErrors = append(observedErrors, fmt.Errorf("reconcile %s: %w", item.ID, err))
			}
		}
	}
	// Leave task cursors empty. The first observed configuration starts strictly in the future.
	s.ready = true
	return errors.Join(observedErrors...)
}

// Tick first reconciles active records, then scans current configuration once.
// Initial discovery and any version change schedule strictly after now. While this process
// stays alive, a late tick considers only the most recent due instant, never a backlog.
// A cursor advances before writes/dispatch: errors are reported, never automatically replayed.
func (c *Coordinator) Tick(ctx context.Context, scope execution.Scope) error {
	s, err := c.scope(scope)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return ErrRecoveryRequired
	}
	now := c.now().UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		return errors.New("scheduler clock returned zero")
	}
	queryCtx, cancelQuery := context.WithTimeout(ctx, 30*time.Second)
	configs, err := c.records.ScheduleConfigs(queryCtx, scope.TenantID, scope.UserID)
	if err != nil {
		cancelQuery()
		return err
	}
	pending, err := c.records.RecoverableOccurrences(queryCtx, scope.TenantID, scope.UserID)
	cancelQuery()
	if err != nil {
		return err
	}
	var failures []error
	reconcileCtx, cancelReconcile := context.WithTimeout(ctx, 30*time.Second)
	for _, item := range pending {
		if !item.Released {
			if err := c.runner.Reconcile(reconcileCtx, scope, item); err != nil {
				failures = append(failures, fmt.Errorf("reconcile %s: %w", item.ID, err))
			}
			continue
		}
		if item.Kind == "scheduled" && item.Status == "recorded" {
			if err := c.abandonScheduledRecord(reconcileCtx, scope, item.ID); err != nil {
				failures = append(failures, fmt.Errorf("abandon unsent %s: %w", item.ID, err))
			}
		}
	}
	cancelReconcile()
	seen := make(map[string]bool, len(configs))
	for _, config := range configs {
		seen[config.ID] = true
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if !config.Enabled {
			s.tasks[config.ID] = scheduleCursor{version: config.Version}
			continue
		}
		var rule schedule.Rule
		if err := json.Unmarshal(config.Rule, &rule); err != nil {
			failures = append(failures, fmt.Errorf("rule %s: %w", config.ID, err))
			continue
		}
		future, err := schedule.Next(rule, config.CreatedAt, now, 1)
		if err != nil {
			failures = append(failures, fmt.Errorf("rule %s: %w", config.ID, err))
			continue
		}
		cursor, found := s.tasks[config.ID]
		if !found || cursor.version != config.Version || !cursor.enabled {
			s.tasks[config.ID] = scheduleCursor{version: config.Version, enabled: true, next: future[0]}
			continue
		}
		if cursor.next.After(now) {
			continue
		}
		due, err := mostRecentDue(rule, config.CreatedAt, cursor.next, now)
		if err != nil {
			failures = append(failures, fmt.Errorf("due %s: %w", config.ID, err))
			continue
		}
		s.tasks[config.ID] = scheduleCursor{version: config.Version, enabled: true, next: future[0]}
		dispatchCtx, cancelDispatch := context.WithTimeout(ctx, 30*time.Second)
		item, err := c.records.RecordOccurrence(dispatchCtx, scope.TenantID, scope.UserID, config.ID, store.Trigger{Kind: "scheduled", At: due, Version: config.Version}, now)
		if err != nil {
			cancelDispatch()
			failures = append(failures, fmt.Errorf("record %s: %w", config.ID, err))
			continue
		}
		if item.Status == "recorded" {
			if err := c.runner.Dispatch(dispatchCtx, scope, item); err != nil {
				cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				abandonErr := c.abandonScheduledRecord(cleanupCtx, scope, item.ID)
				cancelCleanup()
				failures = append(failures, fmt.Errorf("dispatch %s: %w", item.ID, errors.Join(err, abandonErr)))
			}
		}
		cancelDispatch()
	}
	for id := range s.tasks {
		if !seen[id] {
			delete(s.tasks, id)
		}
	}
	return errors.Join(failures...)
}

// abandonScheduledRecord closes an automatic trigger whose cursor already advanced but whose
// Dispatch failed before atomic begin. Manual recorded triggers are left intact for same-key retries.
func (c *Coordinator) abandonScheduledRecord(ctx context.Context, scope execution.Scope, id string) error {
	o, err := c.records.GetOccurrence(ctx, scope.TenantID, scope.UserID, id)
	if err != nil {
		return err
	}
	if o.Kind != "scheduled" || o.Status != "recorded" {
		return nil
	}
	o, err = c.records.ApplyOccurrenceEvidence(ctx, scope.TenantID, scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: "begin"})
	if err != nil || o.Status != "starting" {
		return err
	}
	_, err = c.records.ApplyOccurrenceEvidence(ctx, scope.TenantID, scope.UserID, o.ID, o.Version, store.ExecutionEvidence{Event: "not_accepted"})
	return err
}

func mostRecentDue(rule schedule.Rule, created, first, now time.Time) (time.Time, error) {
	if rule.Kind == "hours" {
		step := time.Duration(rule.Hours) * time.Hour
		return created.Add((now.Sub(created) / step) * step).UTC(), nil
	}
	// Calendar rules recur at least weekly. Starting sixteen days back covers a skipped
	// DST occurrence without iterating years of missed work after a long suspended tick.
	after := now.Add(-16 * 24 * time.Hour)
	if first.Add(-time.Nanosecond).After(after) {
		after = first.Add(-time.Nanosecond)
	}
	latest := first
	for i := 0; i < 32; i++ {
		times, err := schedule.Next(rule, created, after, 1)
		if err != nil {
			return time.Time{}, err
		}
		if times[0].After(now) {
			return latest, nil
		}
		latest, after = times[0], times[0]
	}
	return time.Time{}, errors.New("calendar due search exceeded bound")
}

// Manual uses a stable caller request key. A retry finds the original time and occurrence,
// even if configuration changed meanwhile. A recorded occurrence may finish its first admission
// through Dispatch's atomic begin; every other state is only reconciled, never restarted.
// Paused tasks may be explicitly requested, but database overlap checks remain authoritative.
func (c *Coordinator) Manual(ctx context.Context, scope execution.Scope, scheduleID, key string) (store.Occurrence, error) {
	s, err := c.scope(scope)
	if err != nil {
		return store.Occurrence{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return store.Occurrence{}, ErrRecoveryRequired
	}
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return store.Occurrence{}, store.ErrIneligible
	}
	existing, err := c.records.FindManualOccurrence(ctx, scope.TenantID, scope.UserID, scheduleID, key)
	if err == nil {
		var advanceErr error
		if existing.Status == "recorded" {
			advanceErr = c.runner.Dispatch(ctx, scope, existing)
		} else {
			advanceErr = c.runner.Reconcile(ctx, scope, existing)
		}
		latest, readErr := c.records.GetOccurrence(ctx, scope.TenantID, scope.UserID, existing.ID)
		if readErr != nil {
			return existing, errors.Join(advanceErr, readErr)
		}
		return latest, advanceErr
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Occurrence{}, err
	}
	configs, err := c.records.ScheduleConfigs(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return store.Occurrence{}, err
	}
	for _, config := range configs {
		if config.ID != scheduleID {
			continue
		}
		now := c.now().UTC().Truncate(time.Microsecond)
		item, err := c.records.RecordOccurrence(ctx, scope.TenantID, scope.UserID, scheduleID, store.Trigger{Kind: "manual", Key: key, At: now, Version: config.Version}, now)
		if err != nil {
			return store.Occurrence{}, err
		}
		var dispatchErr error
		if item.Status == "recorded" {
			dispatchErr = c.runner.Dispatch(ctx, scope, item)
		}
		latest, readErr := c.records.GetOccurrence(ctx, scope.TenantID, scope.UserID, item.ID)
		if readErr != nil {
			return item, errors.Join(dispatchErr, readErr)
		}
		return latest, dispatchErr
	}
	return store.Occurrence{}, store.ErrNotFound
}

// Cancel delegates only the scoped persisted occurrence, not an arbitrary external execution ID.
func (c *Coordinator) Cancel(ctx context.Context, scope execution.Scope, occurrenceID string) error {
	if _, err := c.scope(scope); err != nil {
		return err
	}
	item, err := c.records.GetOccurrence(ctx, scope.TenantID, scope.UserID, occurrenceID)
	if err != nil {
		return err
	}
	return c.runner.Cancel(ctx, scope, item)
}
