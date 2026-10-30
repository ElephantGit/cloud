package server

import (
	"errors"
	"net/http"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// spaces returns the caller's workspace spaces in one page. v2 has exactly one per tenant, so
// pagination is skipped; the squad member picker reads the first space's plugins.
func (h *handler) spaces(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	if !squadUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := h.store.TenantSpaces(r.Context(), tenant, u.ID)
	if errors.Is(e, store.ErrNotFound) {
		fault(w, http.StatusForbidden, "forbidden")
		return
	}
	if e != nil {
		internal(w, "tenant spaces", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": ""})
}

// spacePlugins returns the agent plugins installed in one space. Only installed agents are
// listed, which is what a squad may add as a member.
func (h *handler) spacePlugins(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant, space := r.PathValue("tid"), r.PathValue("sid")
	if !squadUUID.MatchString(tenant) || !squadUUID.MatchString(space) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := h.store.SpaceAgentPlugins(r.Context(), tenant, space, u.ID)
	if errors.Is(e, store.ErrNotFound) {
		fault(w, http.StatusForbidden, "forbidden")
		return
	}
	if e != nil {
		internal(w, "space plugins", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
