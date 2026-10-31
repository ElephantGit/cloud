package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/strictjson"
)

type historyStore interface {
	ScheduleHistory(context.Context, string, string, string, store.OccurrenceCursor, int) ([]store.Occurrence, error)
	ScheduleHistoryItem(context.Context, string, string, string, string) (store.Occurrence, error)
}

// Cursors are scoped positions, not authorization tokens. Every query rechecks ownership.
type historyCursor struct {
	Tenant     string    `json:"tenant"`
	User       string    `json:"user"`
	Schedule   string    `json:"schedule"`
	RecordedAt time.Time `json:"recordedAt"`
	ID         string    `json:"id"`
}

func decodeHistoryCursor(raw, tenant, user, schedule string) (store.OccurrenceCursor, bool) {
	if len(raw) > 2048 {
		return store.OccurrenceCursor{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || !strictjson.Valid(data) {
		return store.OccurrenceCursor{}, false
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cursor historyCursor
	if dec.Decode(&cursor) != nil || dec.Decode(&struct{}{}) != io.EOF {
		return store.OccurrenceCursor{}, false
	}
	if cursor.Tenant != tenant || cursor.User != user || cursor.Schedule != schedule || cursor.RecordedAt.IsZero() || !scheduleUUID.MatchString(cursor.ID) {
		return store.OccurrenceCursor{}, false
	}
	recordedAt := cursor.RecordedAt.UTC()
	if !recordedAt.Equal(recordedAt.Truncate(time.Microsecond)) {
		return store.OccurrenceCursor{}, false
	}
	return store.OccurrenceCursor{RecordedAt: recordedAt, ID: strings.ToLower(cursor.ID)}, true
}

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
	var before store.OccurrenceCursor
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
		var ok bool
		before, ok = decodeHistoryCursor(raw, tenant, u.ID, id)
		if !ok {
			fault(w, 400, "invalid_pagination")
			return
		}
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
		last := items[len(items)-1]
		data, _ := json.Marshal(historyCursor{
			Tenant:     tenant,
			User:       u.ID,
			Schedule:   id,
			RecordedAt: last.RecordedAt.UTC().Truncate(time.Microsecond),
			ID:         strings.ToLower(last.ID),
		})
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

func historyError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		fault(w, 404, "not_found")
	case errors.Is(err, store.ErrIneligible):
		fault(w, 400, "invalid_pagination")
	default:
		internal(w, "schedule history", err)
	}
	return true
}
