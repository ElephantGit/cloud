package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ErrScheduleResourcesUnavailable indicates the shared execution catalog is not installed.
var ErrScheduleResourcesUnavailable = errors.New("schedule resources unavailable")

type scheduleProjects interface {
	ListProjects(context.Context, execution.Scope) ([]store.Project, error)
	GetProject(context.Context, execution.Scope, string) (store.Project, error)
}

type scheduleProjectView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type scheduleModelView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type scheduleExecutorView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentName string `json:"agentName"`
	ModelID   string `json:"modelId"`
}
type scheduleResourceView struct {
	Projects  []scheduleProjectView  `json:"projects"`
	Models    []scheduleModelView    `json:"models"`
	Executors []scheduleExecutorView `json:"executors"`
	Available struct {
		Projects  bool `json:"projects"`
		Models    bool `json:"models"`
		Executors bool `json:"executors"`
	} `json:"available"`
}

// scheduleResources projects the shared stores/catalog without exposing bindings,
// repository paths or credentials. Empty and unavailable catalogs are distinct.
func (h *handler) scheduleResources(w http.ResponseWriter, r *http.Request, u store.User) {
	w.Header().Set("Cache-Control", "no-store")
	tid := r.PathValue("tid")
	if !scheduleUUID.MatchString(tid) {
		fault(w, 404, "not_found")
		return
	}
	s, ok := h.store.(scheduleStore)
	if !ok {
		fault(w, 503, "schedule_resources_unavailable")
		return
	}
	member, err := s.ScheduleMember(r.Context(), tid, u.ID)
	if err != nil {
		internal(w, "schedule resource membership", err)
		return
	}
	if !member {
		fault(w, 404, "not_found")
		return
	}
	scope := execution.Scope{TenantID: tid, UserID: u.ID}
	out := scheduleResourceView{Projects: []scheduleProjectView{}, Models: []scheduleModelView{}, Executors: []scheduleExecutorView{}}
	if projects, ok := h.store.(scheduleProjects); ok {
		items, err := projects.ListProjects(r.Context(), scope)
		if scheduleResourceError(w, err) {
			return
		}
		out.Available.Projects = err == nil
		if err == nil {
			for _, p := range items {
				out.Projects = append(out.Projects, scheduleProjectView{p.ID, p.Name, ""})
			}
		}
	}
	models, err := h.opts.Catalog.ListModels(r.Context(), scope)
	if scheduleResourceError(w, err) {
		return
	}
	out.Available.Models = err == nil
	if err == nil {
		for _, m := range models {
			out.Models = append(out.Models, scheduleModelView{m.ID, m.Name})
		}
	}
	executors, err := h.opts.Catalog.ListExecutors(r.Context(), scope)
	if scheduleResourceError(w, err) {
		return
	}
	out.Available.Executors = err == nil
	if err == nil {
		for _, e := range executors {
			out.Executors = append(out.Executors, scheduleExecutorView{e.ID, e.Name, e.AgentName, e.Model.ID})
		}
	}
	writeJSON(w, 200, out)
}

func scheduleResourceError(w http.ResponseWriter, err error) bool {
	if err == nil || errors.Is(err, execution.ErrUnavailable) {
		return false
	}
	if errors.Is(err, execution.ErrNotFound) || errors.Is(err, store.ErrNotFound) {
		fault(w, 404, "not_found")
	} else {
		internal(w, "schedule resources", err)
	}
	return true
}

func (h *handler) resolveScheduleChat(ctx context.Context, tenant, user string, in *store.ScheduleInput, target ScheduleChatTarget) error {
	// Preserve unbound text-only configuration drafts; they cannot pass the runtime's
	// model_required gate. Any executable selection must resolve through the catalog.
	if in.ProjectID == "" && target.Model == "" && target.ExecutorID == "" {
		return nil
	}
	scope := execution.Scope{TenantID: tenant, UserID: user}
	if in.ProjectID == "" {
		if target.ExecutorID != "" {
			return store.ErrNotFound
		}
		models, err := h.opts.Catalog.ListModels(ctx, scope)
		if err != nil {
			return scheduleSelectionError(err)
		}
		for _, m := range models {
			if m.ID == target.Model {
				return nil
			}
		}
		return store.ErrNotFound
	}
	projects, ok := h.store.(scheduleProjects)
	if !ok {
		return ErrScheduleResourcesUnavailable
	}
	if _, err := projects.GetProject(ctx, scope, in.ProjectID); err != nil {
		return scheduleSelectionError(err)
	}
	executors, err := h.opts.Catalog.ListExecutors(ctx, scope)
	if err != nil {
		return scheduleSelectionError(err)
	}
	for _, e := range executors {
		if e.ID == target.ExecutorID && e.Model.ID == target.Model {
			return nil
		}
	}
	return store.ErrNotFound
}

func scheduleSelectionError(err error) error {
	if errors.Is(err, execution.ErrNotFound) {
		return store.ErrNotFound
	}
	if errors.Is(err, execution.ErrUnavailable) {
		return ErrScheduleResourcesUnavailable
	}
	return err
}
