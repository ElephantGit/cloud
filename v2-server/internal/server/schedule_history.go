package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type historyStore interface {
	ScheduleHistory(context.Context, string, string, string, string, int) ([]store.Occurrence, error)
	ScheduleHistoryItem(context.Context, string, string, string, string) (store.Occurrence, error)
}

// Cursors are scoped positions, not authorization tokens. Every query rechecks ownership.
type historyCursor struct{ Tenant, User, Schedule, Before string }

func (h *handler) scheduleHistory(w http.ResponseWriter, r *http.Request, u store.User) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, id, occ := r.PathValue("tid"), r.PathValue("id"), r.PathValue("oid")
	if !scheduleUUID.MatchString(tenant) || !scheduleUUID.MatchString(id) || (occ != "" && !scheduleUUID.MatchString(occ)) {
		fault(w, 404, "not_found")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fault(w, 400, "invalid_pagination")
		return
	}
	limit := 20
	before := ""
	for key, values := range query {
		if (key != "limit" && key != "cursor") || len(values) != 1 || occ != "" {
			fault(w, 400, "invalid_pagination")
			return
		}
	}
	if values, ok := query["limit"]; ok {
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 100 {
			fault(w, 400, "invalid_pagination")
			return
		}
	}
	if raw := query.Get("cursor"); raw != "" {
		if len(raw) > 2048 {
			fault(w, 400, "invalid_pagination")
			return
		}
		var cursor historyCursor
		data, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || json.Unmarshal(data, &cursor) != nil || cursor.Tenant != tenant || cursor.User != u.ID || cursor.Schedule != id || !scheduleUUID.MatchString(cursor.Before) {
			fault(w, 400, "invalid_pagination")
			return
		}
		before = cursor.Before
	}
	s, ok := h.store.(historyStore)
	if !ok {
		fault(w, 503, "schedule_history_unavailable")
		return
	}
	if occ != "" {
		item, e := s.ScheduleHistoryItem(r.Context(), tenant, u.ID, id, occ)
		if historyError(w, e) {
			return
		}
		writeJSON(w, 200, item)
		return
	}
	items, e := s.ScheduleHistory(r.Context(), tenant, u.ID, id, before, limit)
	if historyError(w, e) {
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		data, _ := json.Marshal(historyCursor{tenant, u.ID, id, items[len(items)-1].ID})
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

func historyError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		fault(w, 404, "not_found")
	} else {
		internal(w, "schedule history", err)
	}
	return true
}
