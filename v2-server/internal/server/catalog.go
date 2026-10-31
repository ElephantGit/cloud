package server

import (
	"errors"
	"net/http"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// Public catalog fields are explicit so provider bindings and paths cannot reach HTTP.
type modelView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BindingVersion string `json:"bindingVersion"`
}

type executorView struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	AgentID       string    `json:"agentId"`
	AgentName     string    `json:"agentName"`
	PluginVersion string    `json:"pluginVersion"`
	Model         modelView `json:"model"`
}

func (h *handler) catalogScope(w http.ResponseWriter, r *http.Request, user store.User) (execution.Scope, bool) {
	tenant := r.PathValue("tid")
	if !scheduleUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return execution.Scope{}, false
	}
	if h.opts.Catalog == nil {
		fault(w, http.StatusServiceUnavailable, "execution_catalog_unavailable")
		return execution.Scope{}, false
	}
	return execution.Scope{TenantID: tenant, UserID: user.ID}, true
}

func (h *handler) models(w http.ResponseWriter, r *http.Request, user store.User) {
	scope, ok := h.catalogScope(w, r, user)
	if !ok {
		return
	}
	models, err := h.opts.Catalog.ListModels(r.Context(), scope)
	if err != nil {
		catalogFault(w, err)
		return
	}
	items := make([]modelView, 0, len(models))
	for _, model := range models {
		items = append(items, modelView{model.ID, model.Name, model.BindingVersion})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) executors(w http.ResponseWriter, r *http.Request, user store.User) {
	scope, ok := h.catalogScope(w, r, user)
	if !ok {
		return
	}
	executors, err := h.opts.Catalog.ListExecutors(r.Context(), scope)
	if err != nil {
		catalogFault(w, err)
		return
	}
	items := make([]executorView, 0, len(executors))
	for _, executor := range executors {
		items = append(items, executorView{executor.ID, executor.Name, executor.AgentID, executor.AgentName, executor.PluginVersion,
			modelView{executor.Model.ID, executor.Model.Name, executor.Model.BindingVersion}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func catalogFault(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, execution.ErrNotFound):
		fault(w, http.StatusNotFound, "not_found")
	case errors.Is(err, execution.ErrUnavailable):
		fault(w, http.StatusServiceUnavailable, "execution_catalog_unavailable")
	default:
		internal(w, "execution catalog", err)
	}
}
