package server

import (
	"errors"
	"net/http"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// members returns the workspace roster in one page; v2 workspaces are small enough to skip
// pagination, and the squad picker filters the list client-side.
func (h *handler) members(w http.ResponseWriter, r *http.Request, u store.User) {
	tenant := r.PathValue("tid")
	if !squadUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := h.store.TenantMembers(r.Context(), tenant, u.ID)
	if errors.Is(e, store.ErrNotFound) {
		// Cloud reserves 403 for a caller without an active membership in the workspace.
		fault(w, http.StatusForbidden, "forbidden")
		return
	}
	if e != nil {
		internal(w, "tenant members", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": ""})
}
