package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/requestmetrics"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type dashboardStore interface {
	DashboardSessions(context.Context, execution.Scope, store.DashboardFilter) ([]store.DashboardSession, string, error)
	DashboardMetadata(context.Context, execution.Scope, string) (store.DashboardSession, error)
	DashboardHistory(context.Context, execution.Scope, string) ([]json.RawMessage, bool, error)
	DashboardRequests(context.Context, execution.Scope, string, string) ([]requestmetrics.Attempt, string, error)
	DashboardStatistics(context.Context, execution.Scope, string) (map[string]any, error)
}

func (h *handler) dashboardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/tenants/{tid}/dashboard/sessions", h.authed(h.dashboard))
	mux.HandleFunc("GET /api/v2/tenants/{tid}/dashboard/sessions/{sid}", h.authed(h.dashboard))
	for _, part := range []string{"history", "statistics", "model-calls"} {
		mux.HandleFunc("GET /api/v2/tenants/{tid}/dashboard/sessions/{sid}/"+part, h.authed(h.dashboard))
	}
}

func (h *handler) dashboard(w http.ResponseWriter, r *http.Request, u store.User) {
	w.Header().Set("Cache-Control", "private, no-store")
	s, ok := h.store.(dashboardStore)
	if !ok {
		fault(w, 503, "dashboard_unavailable")
		return
	}
	scope := execution.Scope{TenantID: r.PathValue("tid"), UserID: u.ID}
	if !taskUUID.MatchString(scope.TenantID) {
		fault(w, 404, "not_found")
		return
	}
	q := r.URL.Query()
	id := r.PathValue("sid")
	part := ""
	if id != "" {
		part = r.URL.Path[len("/api/v2/tenants/"+scope.TenantID+"/dashboard/sessions/"+id):]
	}
	var result any
	var err error
	if id == "" {
		f := store.DashboardFilter{Project: q.Get("project"), Agent: q.Get("agent"), Source: q.Get("source"), Status: q.Get("status"), From: q.Get("from"), To: q.Get("to"), After: q.Get("after")}
		for key := range q {
			if len(q[key]) != 1 || (key != "project" && key != "agent" && key != "source" && key != "status" && key != "from" && key != "to" && key != "after") {
				fault(w, 400, "invalid_session_filter")
				return
			}
		}
		var from, to time.Time
		if f.Source != "" && f.Source != "task" && f.Source != "chat" {
			fault(w, 400, "invalid_session_filter")
			return
		}
		if f.From != "" {
			from, err = time.Parse(time.RFC3339Nano, f.From)
			if err != nil {
				fault(w, 400, "invalid_session_filter")
				return
			}
		}
		if f.To != "" {
			to, err = time.Parse(time.RFC3339Nano, f.To)
			if err != nil {
				fault(w, 400, "invalid_session_filter")
				return
			}
		}
		if (!from.IsZero() && !to.IsZero() && !from.Before(to)) || len(f.Agent) > 200 || len(f.Project) > 128 || len(f.After) > 4096 {
			fault(w, 400, "invalid_session_filter")
			return
		}
		switch f.Status {
		case "", "running", "completed", "failed", "cancelled", "interrupted":
		default:
			fault(w, 400, "invalid_session_filter")
			return
		}
		var items []store.DashboardSession
		var next string
		items, next, err = s.DashboardSessions(r.Context(), scope, f)
		result = map[string]any{"items": items, "nextCursor": next}
	} else {
		for key := range q {
			if part != "/model-calls" || key != "after" || len(q[key]) != 1 || len(q.Get(key)) > 128 {
				fault(w, 400, "invalid_pagination")
				return
			}
		}
		switch part {
		case "":
			result, err = s.DashboardMetadata(r.Context(), scope, id)
		case "/history":
			var records []json.RawMessage
			var incomplete bool
			records, incomplete, err = s.DashboardHistory(r.Context(), scope, id)
			result = map[string]any{"source": "persisted_snapshot", "records": records, "incomplete": incomplete}
		case "/statistics":
			result, err = s.DashboardStatistics(r.Context(), scope, id)
		case "/model-calls":
			var items []requestmetrics.Attempt
			var next string
			items, next, err = s.DashboardRequests(r.Context(), scope, id, q.Get("after"))
			result = map[string]any{"items": items, "nextCursor": next}
		}
	}
	if err != nil {
		chatFault(w, "dashboard read", err)
		return
	}
	writeJSON(w, 200, result)
}
