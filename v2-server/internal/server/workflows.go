package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// WorkflowStore is the asset persistence boundary; the future runner consumes frozen snapshots.
type WorkflowStore interface {
	Workflows(context.Context, string, string, int) ([]store.Workflow, error)
	Workflow(context.Context, string, string) (store.Workflow, error)
	CreateWorkflow(context.Context, string, store.WorkflowEdit) (store.Workflow, error)
	UpdateWorkflow(context.Context, string, string, store.WorkflowEdit) (store.Workflow, error)
	ArchiveWorkflow(context.Context, string, string, int64) (store.Workflow, error)
	PublishWorkflow(context.Context, string, string, string, int64) (store.WorkflowSnapshot, error)
	WorkflowSnapshots(context.Context, string, string, string, int) ([]store.WorkflowSnapshot, error)
	WorkflowSnapshot(context.Context, string, string, string) (store.WorkflowSnapshot, error)
	RestoreWorkflowSnapshot(context.Context, string, string, string, int64) (store.Workflow, error)
}

func (h *handler) workflowRoutes(mux *http.ServeMux) {
	const root = "/api/v1/tenants/{tid}/workflows"
	for pattern, fn := range map[string]http.HandlerFunc{
		"GET " + root:                                            h.listWorkflows,
		"POST " + root:                                           h.createWorkflow,
		"GET " + root + "/{wfid}":                                h.getWorkflow,
		"PUT " + root + "/{wfid}":                                h.updateWorkflow,
		"DELETE " + root + "/{wfid}":                             h.archiveWorkflow,
		"POST " + root + "/{wfid}/publish":                       h.publishWorkflow,
		"GET " + root + "/{wfid}/snapshots":                      h.listSnapshots,
		"GET " + root + "/{wfid}/snapshots/{snapshotId}":         h.getSnapshot,
		"PUT " + root + "/{wfid}/snapshots/{snapshotId}/restore": h.restoreSnapshot,
	} {
		mux.HandleFunc(pattern, h.workflowMember(fn))
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (h *handler) workflowMember(next http.HandlerFunc) http.HandlerFunc {
	return h.authed(func(w http.ResponseWriter, r *http.Request, u store.User) {
		if u.Status != "active" {
			fault(w, 403, "forbidden")
			return
		}
		tenants, err := h.store.Tenants(r.Context(), u.ID)
		if err != nil {
			internal(w, "workflow membership", err)
			return
		}
		member := false
		for _, t := range tenants {
			if strings.EqualFold(t.ID, r.PathValue("tid")) && t.Status == "active" {
				member = true
				break
			}
		}
		if !member {
			fault(w, 403, "forbidden")
			return
		}
		for _, key := range []string{"wfid", "snapshotId"} {
			if id := r.PathValue(key); id != "" && !uuidPattern.MatchString(id) {
				fault(w, 404, "not_found")
				return
			}
		}
		next(w, r)
	})
}

func workflowResult(w http.ResponseWriter, value any, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		fault(w, 404, "not_found")
	case errors.Is(err, store.ErrVersionConflict):
		fault(w, 409, "version_conflict")
	case errors.Is(err, store.ErrWorkflowConflict):
		fault(w, 409, "workflow_conflict")
	case err != nil:
		internal(w, "workflow", err)
	default:
		writeJSON(w, 200, value)
	}
}

func pagination(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	after, limit := r.URL.Query().Get("after"), 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			fault(w, 400, "invalid_pagination")
			return "", 0, false
		}
		limit = n
	}
	if after != "" && !uuidPattern.MatchString(after) {
		fault(w, 400, "invalid_pagination")
		return "", 0, false
	}
	return after, limit, true
}

func (h *handler) listWorkflows(w http.ResponseWriter, r *http.Request) {
	after, limit, ok := pagination(w, r)
	if !ok {
		return
	}
	items, err := h.store.Workflows(r.Context(), r.PathValue("tid"), after, limit)
	cursor := ""
	if len(items) > limit {
		items = items[:limit]
		cursor = items[limit-1].ID
	}
	if items == nil {
		items = []store.Workflow{}
	}
	workflowResult(w, map[string]any{"items": items, "nextCursor": cursor}, err)
}

func (h *handler) getWorkflow(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.Workflow(r.Context(), r.PathValue("tid"), r.PathValue("wfid"))
	workflowResult(w, out, err)
}

// readWorkflowBody preserves arbitrary graph fields, but rejects unknown envelope fields,
// explicit nulls, oversized bodies and concatenated JSON documents.
func readWorkflowBody(w http.ResponseWriter, r *http.Request, allowed ...string) (map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	var body map[string]json.RawMessage
	if err := dec.Decode(&body); err != nil || body == nil {
		fault(w, 400, "invalid_json")
		return nil, false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		fault(w, 400, "invalid_json")
		return nil, false
	}
	for field, value := range body {
		known := false
		for _, key := range allowed {
			if key == field {
				known = true
				break
			}
		}
		if !known || string(value) == "null" {
			fault(w, 400, "invalid_input")
			return nil, false
		}
	}
	return body, true
}

func workflowEdit(w http.ResponseWriter, r *http.Request, update bool) (store.WorkflowEdit, bool) {
	allowed := []string{"name", "description", "graph"}
	if update {
		allowed = append(allowed, "version")
	}
	body, ok := readWorkflowBody(w, r, allowed...)
	if !ok {
		return store.WorkflowEdit{}, false
	}
	var edit store.WorkflowEdit
	if json.Unmarshal(body["name"], &edit.Name) != nil || !validWorkflowName(edit.Name) {
		fault(w, 400, "invalid_input")
		return edit, false
	}
	edit.Name = strings.TrimSpace(edit.Name)
	if raw, exists := body["description"]; exists {
		var description string
		if json.Unmarshal(raw, &description) != nil {
			fault(w, 400, "invalid_input")
			return edit, false
		}
		edit.Description = &description
	}
	if raw, exists := body["graph"]; exists {
		var graph map[string]json.RawMessage
		if json.Unmarshal(raw, &graph) != nil || graph == nil {
			fault(w, 400, "invalid_input")
			return edit, false
		}
		edit.Graph = raw
	}
	if update {
		version, ok := workflowVersion(w, body)
		if !ok {
			return edit, false
		}
		edit.Version = version
	}
	return edit, true
}

func validWorkflowName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && utf8.RuneCountInString(name) <= 200
}

func workflowVersion(w http.ResponseWriter, body map[string]json.RawMessage) (int64, bool) {
	var version int64
	if _, ok := body["version"]; !ok {
		fault(w, 428, "version_required")
		return 0, false
	}
	if json.Unmarshal(body["version"], &version) != nil || version < 0 {
		fault(w, 400, "invalid_input")
		return 0, false
	}
	if version == 0 {
		fault(w, 428, "version_required")
		return 0, false
	}
	return version, true
}

func (h *handler) createWorkflow(w http.ResponseWriter, r *http.Request) {
	edit, ok := workflowEdit(w, r, false)
	if !ok {
		return
	}
	out, err := h.store.CreateWorkflow(r.Context(), r.PathValue("tid"), edit)
	workflowResult(w, map[string]any{"resource": out}, err)
}

func (h *handler) updateWorkflow(w http.ResponseWriter, r *http.Request) {
	edit, ok := workflowEdit(w, r, true)
	if !ok {
		return
	}
	out, err := h.store.UpdateWorkflow(r.Context(), r.PathValue("tid"), r.PathValue("wfid"), edit)
	workflowResult(w, out, err)
}

func (h *handler) archiveWorkflow(w http.ResponseWriter, r *http.Request) {
	body, ok := readWorkflowBody(w, r, "version")
	if !ok {
		return
	}
	version, ok := workflowVersion(w, body)
	if !ok {
		return
	}
	out, err := h.store.ArchiveWorkflow(r.Context(), r.PathValue("tid"), r.PathValue("wfid"), version)
	workflowResult(w, out, err)
}

func (h *handler) publishWorkflow(w http.ResponseWriter, r *http.Request) {
	body, ok := readWorkflowBody(w, r, "name")
	if !ok {
		return
	}
	name := ""
	if raw, exists := body["name"]; exists {
		if json.Unmarshal(raw, &name) != nil || (name != "" && !validWorkflowName(name)) {
			fault(w, 400, "invalid_input")
			return
		}
		name = strings.TrimSpace(name)
	}
	draft, err := h.store.Workflow(r.Context(), r.PathValue("tid"), r.PathValue("wfid"))
	if err != nil {
		workflowResult(w, nil, err)
		return
	}
	if problems := graphProblems(draft.Graph); len(problems) > 0 {
		writeJSON(w, 400, map[string]any{"code": "workflow_graph_invalid", "params": map[string]any{"problems": problems}, "requestId": requestID()})
		return
	}
	out, err := h.store.PublishWorkflow(r.Context(), draft.TenantID, draft.ID, name, draft.Version)
	workflowResult(w, map[string]any{"resource": out}, err)
}

func (h *handler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	after, limit, ok := pagination(w, r)
	if !ok {
		return
	}
	items, err := h.store.WorkflowSnapshots(r.Context(), r.PathValue("tid"), r.PathValue("wfid"), after, limit)
	cursor := ""
	if len(items) > limit {
		items = items[:limit]
		cursor = items[limit-1].ID
	}
	if items == nil {
		items = []store.WorkflowSnapshot{}
	}
	workflowResult(w, map[string]any{"items": items, "nextCursor": cursor}, err)
}

func (h *handler) getSnapshot(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.WorkflowSnapshot(r.Context(), r.PathValue("tid"), r.PathValue("wfid"), r.PathValue("snapshotId"))
	workflowResult(w, out, err)
}

func (h *handler) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	body, ok := readWorkflowBody(w, r, "version")
	if !ok {
		return
	}
	version, ok := workflowVersion(w, body)
	if !ok {
		return
	}
	out, err := h.store.RestoreWorkflowSnapshot(r.Context(), r.PathValue("tid"), r.PathValue("wfid"), r.PathValue("snapshotId"), version)
	workflowResult(w, out, err)
}
