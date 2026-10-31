package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

// These document and snapshot routes retain api/openapi.json's resource shapes.
// Like v2's other compatible routes, browser authentication uses ora_session.
const workflowBase = "/api/v1/tenants/{tid}/workflows"

type workflowStore interface {
	CheckExecutionScope(context.Context, execution.Scope) error
	GetWorkflow(context.Context, execution.Scope, string) (workflow.Document, error)
	ListWorkflows(context.Context, execution.Scope, string, int) (workflow.Page[workflow.Document], error)
	CreateWorkflow(context.Context, execution.Scope, string, workflow.CreateDocument) (workflow.Document, error)
	UpdateWorkflow(context.Context, execution.Scope, string, workflow.UpdateDocument) (workflow.Document, error)
	DeleteWorkflow(context.Context, execution.Scope, string, string, workflow.DocumentVersion) (workflow.Document, error)
	PublishWorkflow(context.Context, execution.Scope, string, string, workflow.PublishDocument) (workflow.Snapshot, error)
	GetWorkflowSnapshot(context.Context, execution.Scope, string, string) (workflow.Snapshot, error)
	ListWorkflowSnapshots(context.Context, execution.Scope, string, string, int) (workflow.Page[workflow.Snapshot], error)
	RestoreWorkflowSnapshot(context.Context, execution.Scope, string, string, workflow.DocumentVersion) (workflow.Document, error)
}

func (h *handler) workflowRoutes(mux *http.ServeMux) {
	for _, route := range []string{"GET " + workflowBase, "POST " + workflowBase, "GET " + workflowBase + "/{wfid}", "PUT " + workflowBase + "/{wfid}", "DELETE " + workflowBase + "/{wfid}"} {
		mux.HandleFunc(route, h.workflowAccess(h.workflowDocuments))
	}
	mux.HandleFunc("POST "+workflowBase+"/{wfid}/publish", h.workflowAccess(h.workflowPublish))
	mux.HandleFunc("GET "+workflowBase+"/{wfid}/snapshots", h.workflowAccess(h.workflowSnapshots))
	mux.HandleFunc("GET "+workflowBase+"/{wfid}/snapshots/{snapshotId}", h.workflowAccess(h.workflowSnapshots))
	mux.HandleFunc("PUT "+workflowBase+"/{wfid}/snapshots/{snapshotId}/restore", h.workflowAccess(h.workflowRestore))
}

type workflowRequest struct {
	scope execution.Scope
	store workflowStore
}

func (h *handler) workflowAccess(next func(http.ResponseWriter, *http.Request, workflowRequest)) http.HandlerFunc {
	return h.authed(func(w http.ResponseWriter, r *http.Request, u store.User) {
		s, ok := h.store.(workflowStore)
		if !ok {
			fault(w, http.StatusServiceUnavailable, "workflows_unavailable")
			return
		}
		for _, key := range []string{"tid", "wfid", "snapshotId"} {
			if id := r.PathValue(key); id != "" && !scheduleUUID.MatchString(id) {
				fault(w, http.StatusNotFound, "not_found")
				return
			}
		}
		scope := execution.Scope{TenantID: r.PathValue("tid"), UserID: u.ID}
		if err := s.CheckExecutionScope(r.Context(), scope); err != nil {
			workflowFault(w, err)
			return
		}
		next(w, r, workflowRequest{scope: scope, store: s})
	})
}

func (h *handler) workflowDocuments(w http.ResponseWriter, r *http.Request, c workflowRequest) {
	ctx, id := r.Context(), r.PathValue("wfid")
	switch r.Method {
	case http.MethodGet:
		if id != "" {
			out, err := c.store.GetWorkflow(ctx, c.scope, id)
			workflowReply(w, out, err, false)
			return
		}
		after, limit, ok := workflowPagination(w, r)
		if !ok {
			return
		}
		out, err := c.store.ListWorkflows(ctx, c.scope, after, limit)
		workflowReply(w, out, err, false)
	case http.MethodPost:
		key, ok := idempotencyKey(w, r)
		if !ok {
			return
		}
		var in workflow.CreateDocument
		if !decodeWorkflowDocument(w, r, &in) {
			return
		}
		out, err := c.store.CreateWorkflow(ctx, c.scope, key, in)
		workflowReply(w, out, err, true)
	case http.MethodPut:
		var in workflow.UpdateDocument
		if !decodeWorkflowDocument(w, r, &in) {
			return
		}
		out, err := c.store.UpdateWorkflow(ctx, c.scope, id, in)
		workflowReply(w, out, err, false)
	case http.MethodDelete:
		key, ok := idempotencyKey(w, r)
		if !ok {
			return
		}
		var in workflow.DocumentVersion
		if !decodeWorkflowDocument(w, r, &in) {
			return
		}
		out, err := c.store.DeleteWorkflow(ctx, c.scope, id, key, in)
		workflowReply(w, out, err, false)
	}
}

func (h *handler) workflowPublish(w http.ResponseWriter, r *http.Request, c workflowRequest) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in workflow.PublishDocument
	if !decodeWorkflowDocument(w, r, &in) {
		return
	}
	out, err := c.store.PublishWorkflow(r.Context(), c.scope, r.PathValue("wfid"), key, in)
	workflowReply(w, out, err, true)
}

func (h *handler) workflowSnapshots(w http.ResponseWriter, r *http.Request, c workflowRequest) {
	if snapshot := r.PathValue("snapshotId"); snapshot != "" {
		out, err := c.store.GetWorkflowSnapshot(r.Context(), c.scope, r.PathValue("wfid"), snapshot)
		workflowReply(w, out, err, false)
		return
	}
	after, limit, ok := workflowPagination(w, r)
	if !ok {
		return
	}
	out, err := c.store.ListWorkflowSnapshots(r.Context(), c.scope, r.PathValue("wfid"), after, limit)
	workflowReply(w, out, err, false)
}

func (h *handler) workflowRestore(w http.ResponseWriter, r *http.Request, c workflowRequest) {
	var in workflow.DocumentVersion
	if !decodeWorkflowDocument(w, r, &in) {
		return
	}
	out, err := c.store.RestoreWorkflowSnapshot(r.Context(), c.scope, r.PathValue("wfid"), r.PathValue("snapshotId"), in)
	workflowReply(w, out, err, false)
}

func workflowReply(w http.ResponseWriter, out any, err error, resource bool) {
	if err != nil {
		workflowFault(w, err)
		return
	}
	if resource {
		out = map[string]any{"resource": out}
	}
	writeJSON(w, http.StatusOK, out)
}

func workflowFault(w http.ResponseWriter, err error) {
	var graph *workflow.GraphError
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, execution.ErrNotFound):
		fault(w, http.StatusNotFound, "not_found")
	case errors.Is(err, workflow.ErrVersionRequired):
		fault(w, http.StatusPreconditionRequired, "version_required")
	case errors.Is(err, store.ErrConflict):
		fault(w, http.StatusConflict, "version_conflict")
	case errors.Is(err, store.ErrWorkflowNameConflict):
		fault(w, http.StatusConflict, "workflow_conflict")
	case errors.Is(err, store.ErrKeyReused):
		fault(w, http.StatusConflict, "idempotency_conflict")
	case errors.As(err, &graph):
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "workflow_graph_invalid", "params": map[string]any{"problems": graph.Problems}, "requestId": requestID()})
	case errors.Is(err, execution.ErrInvalidRequest):
		fault(w, http.StatusBadRequest, "invalid_input")
	default:
		internal(w, "workflow", err)
	}
}

func workflowPagination(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	after, limit := r.URL.Query().Get("after"), 50
	valid := after == "" || scheduleUUID.MatchString(after)
	if values, present := r.URL.Query()["limit"]; present {
		var err error
		limit, err = strconv.Atoi(values[0])
		valid = valid && len(values) == 1 && err == nil && limit >= 1 && limit <= 100
	}
	if !valid || len(r.URL.Query()["after"]) > 1 {
		fault(w, http.StatusBadRequest, "invalid_pagination")
		return "", 0, false
	}
	return after, limit, true
}

// Draft graphs are opaque objects, with a bounded envelope. Reject null fields,
// unknown fields and trailing JSON instead of silently changing update meaning.
func decodeWorkflowDocument(w http.ResponseWriter, r *http.Request, out any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workflow.MaxDocumentBytes))
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &fields) != nil || fields == nil {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	for _, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			fault(w, http.StatusBadRequest, "invalid_input")
			return false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}
