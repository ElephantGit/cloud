package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ScopeSource discovers current schedule owners, not HTTP users or login cookies.
type ScopeSource interface {
	ScheduleScopes(context.Context) ([]execution.Scope, error)
}

// Host shares one coordinator between automatic scans and HTTP commands. Run must
// start after the public execution service has completed its startup recovery.
// The owner cancels Run and joins it before closing execution or the database.
type Host struct {
	records     ScopeSource
	coordinator *Coordinator
	catalog     execution.Catalog
	ready       chan struct{}
	mu          sync.Mutex
	started     bool
	ctx         context.Context
	ops         sync.WaitGroup
}

// NewHost creates no goroutine. An unavailable Catalog prevents new occurrences
// and recovery mutations when no execution runtime is configured.
func NewHost(records ScopeSource, coordinator *Coordinator, catalog execution.Catalog) (*Host, error) {
	if records == nil || coordinator == nil || catalog == nil {
		return nil, errors.New("schedule host requires records, coordinator and catalog")
	}
	return &Host{records: records, coordinator: coordinator, catalog: catalog, ready: make(chan struct{})}, nil
}

// Ready closes after the initial scan. It indicates host startup, not execution availability.
func (h *Host) Ready() <-chan struct{} { return h.ready }

// Run owns the scan lifecycle; ticks are supplied by the process owner for deterministic tests.
// It may run only once. It waits for admitted HTTP operations before returning.
func (h *Host) Run(ctx context.Context, ticks <-chan time.Time) error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return errors.New("schedule host already started")
	}
	h.started, h.ctx = true, ctx
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.ctx = nil
		h.mu.Unlock()
		h.ops.Wait()
	}()
	h.scan(ctx)
	close(h.ready)
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			h.scan(ctx)
		}
	}
}

// operation links requests to process shutdown as well as client cancellation.
func (h *Host) operation(ctx context.Context) (context.Context, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx == nil || h.ctx.Err() != nil {
		return nil, nil, execution.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(h.ctx, cancel)
	h.ops.Add(1)
	return op, func() { stop(); cancel(); h.ops.Done() }, nil
}

func (h *Host) prepare(ctx context.Context, scope execution.Scope) error {
	// This is only an availability barrier. Selection authorization remains the
	// execution service's responsibility for every Start; no catalog IDs are invented.
	if _, err := h.catalog.ListModels(ctx, scope); err != nil {
		return err
	}
	return h.coordinator.Recover(ctx, scope)
}

func (h *Host) scan(ctx context.Context) {
	query, cancel := context.WithTimeout(ctx, 30*time.Second)
	scopes, err := h.records.ScheduleScopes(query)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("schedule scope scan failed", "error", err)
		}
		return
	}
	for _, scope := range scopes {
		if ctx.Err() != nil {
			return
		}
		prepCtx, cancelPrep := context.WithTimeout(ctx, 30*time.Second)
		err := h.prepare(prepCtx, scope)
		cancelPrep()
		if err == nil {
			err = h.coordinator.Tick(ctx, scope)
		}
		if report := reportableScanError(err); report != nil && ctx.Err() == nil {
			slog.Warn("schedule scan failed", "tenant", scope.TenantID, "user", scope.UserID, "error", report)
		}
	}
}

// reportableScanError strips expected availability/concurrency/missing-attempt leaves from joined
// batch errors so one benign item never silences real persistence or dispatch failures in the same scan.
func reportableScanError(err error) error {
	if err == nil {
		return nil
	}
	var leaves []error
	collectScanErrors("", err, &leaves)
	return errors.Join(leaves...)
}

func collectScanErrors(prefix string, err error, dst *[]error) {
	if err == nil {
		return
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range multi.Unwrap() {
			collectScanErrors(prefix, child, dst)
		}
		return
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		inner := single.Unwrap()
		if _, isMulti := inner.(interface{ Unwrap() []error }); isMulti {
			p := strings.TrimSuffix(strings.TrimSuffix(err.Error(), inner.Error()), ": ")
			if prefix != "" && p != "" {
				p = prefix + ": " + p
			} else if prefix != "" {
				p = prefix
			}
			collectScanErrors(p, inner, dst)
			return
		}
	}
	if errors.Is(err, execution.ErrUnavailable) || errors.Is(err, execution.ErrNotFound) || errors.Is(err, store.ErrNotFound) || errors.Is(err, ErrOperationInProgress) {
		return
	}
	if prefix != "" {
		*dst = append(*dst, fmt.Errorf("%s: %w", prefix, err))
		return
	}
	*dst = append(*dst, err)
}

// Manual accepts a stable request key and never trusts caller-supplied execution references.
func (h *Host) Manual(ctx context.Context, scope execution.Scope, id, key string) (store.Occurrence, error) {
	op, finish, err := h.operation(ctx)
	if err != nil {
		return store.Occurrence{}, err
	}
	defer finish()
	if err := h.prepare(op, scope); err != nil {
		return store.Occurrence{}, err
	}
	return h.coordinator.Manual(op, scope, id, key)
}

// Cancel requests a stop; only the execution result and End confirmation release the slot.
func (h *Host) Cancel(ctx context.Context, scope execution.Scope, id string) error {
	op, finish, err := h.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if err := h.prepare(op, scope); err != nil {
		return err
	}
	return h.coordinator.Cancel(op, scope, id)
}
