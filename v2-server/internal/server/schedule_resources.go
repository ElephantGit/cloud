package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
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
type scheduleAgentView struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Version string              `json:"version"`
	Models  []scheduleModelView `json:"models"`
}
type scheduleWorkflowView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	SnapshotID string `json:"snapshotId"`
}
type scheduleResourceView struct {
	Projects  []scheduleProjectView  `json:"projects"`
	Models    []scheduleModelView    `json:"models"`
	Agents    []scheduleAgentView    `json:"agents"`
	Workflows []scheduleWorkflowView `json:"workflows"`
	Available struct {
		Projects  bool `json:"projects"`
		Models    bool `json:"models"`
		Agents    bool `json:"agents"`
		Workflows bool `json:"workflows"`
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
	out := scheduleResourceView{Projects: []scheduleProjectView{}, Models: []scheduleModelView{}, Agents: []scheduleAgentView{}, Workflows: []scheduleWorkflowView{}}
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
	out.Available.Agents = err == nil
	if err == nil {
		index := map[string]int{}
		modelIDs := map[string]map[string]bool{}
		for _, e := range executors {
			i, ok := index[e.AgentID]
			if !ok {
				i = len(out.Agents)
				index[e.AgentID] = i
				out.Agents = append(out.Agents, scheduleAgentView{ID: e.AgentID, Name: e.AgentName, Version: e.PluginVersion, Models: []scheduleModelView{}})
				modelIDs[e.AgentID] = map[string]bool{}
			}
			if !modelIDs[e.AgentID][e.Model.ID] {
				out.Agents[i].Models = append(out.Agents[i].Models, scheduleModelView{ID: e.Model.ID, Name: e.Model.Name})
				modelIDs[e.AgentID][e.Model.ID] = true
			}
		}
	}
	if workflows, err := h.scheduleWorkflows(r.Context(), tid); err != nil {
		internal(w, "schedule workflow resources", err)
		return
	} else if workflows != nil {
		out.Workflows, out.Available.Workflows = workflows, true
	}
	writeJSON(w, 200, out)
}

// scheduleWorkflows lists the published workflows a schedule can run, each with the snapshot it
// would freeze; nil when this process cannot run workflows at all.
func (h *handler) scheduleWorkflows(ctx context.Context, tenant string) ([]scheduleWorkflowView, error) {
	workflows, ok := h.store.(WorkflowStore)
	if _, runs := h.store.(scheduleWorkflowStore); !ok || !runs || h.opts.WorkflowRuns == nil {
		return nil, nil
	}
	summaries, err := workflows.Workflows(ctx, tenant)
	if err != nil {
		return nil, err
	}
	out := []scheduleWorkflowView{}
	for _, summary := range summaries {
		if summary.PublishedVersion == nil {
			continue
		}
		// The library row names the version only; the snapshot is what a schedule freezes.
		w, err := workflows.Workflow(ctx, tenant, summary.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if w.PublishedSnapshotID == nil {
			continue
		}
		out = append(out, scheduleWorkflowView{ID: w.ID, Name: w.Name, Version: *summary.PublishedVersion, SnapshotID: *w.PublishedSnapshotID})
	}
	return out, nil
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
	if in.ProjectID == "" && target.Model == "" && target.AgentID == "" && target.ExecutorID == "" {
		return nil
	}
	scope := execution.Scope{TenantID: tenant, UserID: user}
	if in.ProjectID == "" {
		if target.AgentID != "" || target.ExecutorID != "" {
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
		if e.Model.ID != target.Model {
			continue
		}
		if target.AgentID != "" && e.AgentID == target.AgentID && (target.ExecutorID == "" || e.ID == target.ExecutorID) {
			return nil
		}
		if target.AgentID == "" && e.ID == target.ExecutorID {
			return nil
		}
	}
	return store.ErrNotFound
}

// errScheduleWorkflowsUnavailable means this process cannot run workflows (no execution bridge
// or run store), so a workflow target is refused rather than saved to fail at every trigger.
var errScheduleWorkflowsUnavailable = errors.New("schedule workflow targets unavailable")

// resolveScheduleWorkflow checks a workflow target against what the owner can run now: the project
// is theirs, and the snapshot is a published (not draft) version of the workflow in this tenant.
// The schedule keeps that snapshot; a newer version applies only once the schedule is edited.
func (h *handler) resolveScheduleWorkflow(ctx context.Context, tenant, user string, in *store.ScheduleInput, target scheduler.WorkflowTarget) error {
	workflows, ok := h.store.(WorkflowStore)
	if !ok || h.opts.WorkflowRuns == nil {
		return errScheduleWorkflowsUnavailable
	}
	if _, ok := h.store.(scheduleWorkflowStore); !ok {
		return errScheduleWorkflowsUnavailable
	}
	projects, ok := h.store.(scheduleProjects)
	if !ok {
		return ErrScheduleResourcesUnavailable
	}
	scope := execution.Scope{TenantID: tenant, UserID: user}
	if _, err := projects.GetProject(ctx, scope, in.ProjectID); err != nil {
		return scheduleSelectionError(err)
	}
	snapshot, err := workflows.WorkflowSnapshot(ctx, tenant, target.WorkflowID, target.SnapshotID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrWorkflowSnapshotNotFound) {
		return store.ErrNotFound
	}
	if err != nil {
		return err
	}
	if snapshot.Version == store.WorkflowDraftVersion {
		return store.ErrNotFound
	}
	return nil
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
