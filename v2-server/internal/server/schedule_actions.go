package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ScheduleActions is the shared host used by both HTTP commands and periodic scans.
type ScheduleActions interface {
	Manual(context.Context, execution.Scope, string, string) (store.Occurrence, error)
	Cancel(context.Context, execution.Scope, string) error
}

type scheduleActionStore interface {
	GetScheduleConfig(context.Context, string, string, string) (store.ScheduleConfig, error)
	GetOccurrence(context.Context, string, string, string) (store.Occurrence, error)
}

// actionReceipt contains task-domain facts only, never public execution references or prompts.
type actionReceipt struct {
	ID          string    `json:"id"`
	ScheduleID  string    `json:"scheduleId"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	Released    bool      `json:"released"`
	Version     int64     `json:"version"`
	ScheduledAt time.Time `json:"scheduledAt"`
}

func receipt(o store.Occurrence) actionReceipt {
	return actionReceipt{o.ID, o.ScheduleID, o.Kind, o.Status, o.Reason, o.Released, o.Version, o.ScheduledAt}
}

func (h *handler) scheduleAction(w http.ResponseWriter, r *http.Request, u store.User) {
	s, ok := h.store.(scheduleActionStore)
	if !ok {
		fault(w, 503, "schedule_execution_unavailable")
		return
	}
	tid, id, oid := r.PathValue("tid"), r.PathValue("id"), r.PathValue("oid")
	if !scheduleUUID.MatchString(tid) || !scheduleUUID.MatchString(id) || (oid != "" && !scheduleUUID.MatchString(oid)) {
		fault(w, 404, "not_found")
		return
	}
	// Authorize the exact configuration before exposing execution readiness or attempting a stop.
	if _, err := s.GetScheduleConfig(r.Context(), tid, u.ID, id); err != nil {
		scheduleActionError(w, "schedule action config", err, "")
		return
	}
	var old store.Occurrence
	if oid != "" {
		var err error
		old, err = s.GetOccurrence(r.Context(), tid, u.ID, oid)
		if err != nil {
			scheduleActionError(w, "schedule action occurrence", err, "")
			return
		}
		if old.ScheduleID != id {
			fault(w, 404, "not_found")
			return
		}
	}
	var key string
	if oid == "" {
		var in struct {
			RequestID string `json:"requestId"`
		}
		if !decodeSchedule(w, r, &in) {
			return
		}
		key = in.RequestID
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) || len(key) > 200 {
			fault(w, 400, "invalid_request_id")
			return
		}
	} else {
		var in *struct{}
		if !decodeSchedule(w, r, &in) {
			return
		}
		if in == nil {
			fault(w, 400, "invalid_json")
			return
		}
	}
	if h.opts.Schedules == nil {
		fault(w, 503, "schedule_execution_unavailable")
		return
	}
	scope := execution.Scope{TenantID: tid, UserID: u.ID}
	if oid == "" {
		o, err := h.opts.Schedules.Manual(r.Context(), scope, id, key)
		if err != nil {
			scheduleActionError(w, "schedule manual run", err, o.ID)
			return
		}
		writeJSON(w, 200, receipt(o))
		return
	}
	if err := h.opts.Schedules.Cancel(r.Context(), scope, old.ID); err != nil {
		scheduleActionError(w, "schedule cancel run", err, old.ID)
		return
	}
	o, err := s.GetOccurrence(r.Context(), tid, u.ID, old.ID)
	if err != nil {
		scheduleActionError(w, "schedule reload occurrence", err, old.ID)
		return
	}
	writeJSON(w, 200, receipt(o))
}

func scheduleActionError(w http.ResponseWriter, op string, err error, id string) {
	status, code := 500, "internal"
	switch {
	case errors.Is(err, store.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, execution.ErrNotFound):
		if id != "" {
			status, code = 409, "execution_target_unavailable"
		} else {
			status, code = 404, "not_found"
		}
	case errors.Is(err, execution.ErrUnavailable), errors.Is(err, scheduler.ErrRecoveryRequired):
		status, code = 503, "schedule_execution_unavailable"
	case errors.Is(err, scheduler.ErrOperationInProgress):
		status, code = 409, "operation_in_progress"
	case errors.Is(err, execution.ErrStopUnconfirmed):
		status, code = 503, "stop_unconfirmed"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, code = 503, "operation_incomplete"
	case errors.Is(err, store.ErrConflict), errors.Is(err, execution.ErrConflict):
		status, code = 409, "execution_conflict"
	case errors.Is(err, execution.ErrWorkspaceBusy):
		status, code = 409, "execution_busy"
	case errors.Is(err, store.ErrIneligible), errors.Is(err, execution.ErrInvalidRequest):
		status, code = 400, "invalid_execution_request"
	case errors.Is(err, execution.ErrUnsupported):
		status, code = 400, "execution_unsupported"
	}
	if status == 500 {
		slog.Error(op, "error", err)
	}
	params := map[string]string{}
	if id != "" && status != 404 {
		params["occurrenceId"] = id
	}
	writeJSON(w, status, map[string]any{"code": code, "params": params, "requestId": requestID()})
}
