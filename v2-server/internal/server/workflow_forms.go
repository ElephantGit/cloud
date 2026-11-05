package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflowforms"
	"net/http"
	"strings"
)

type workflowFormHost interface {
	RetryForm(context.Context, WorkflowRunScope, string) (store.WorkflowForm, error)
	ReadForm(context.Context, WorkflowRunScope, string) (store.WorkflowForm, error)
	SaveForm(context.Context, WorkflowRunScope, string, int64, string, json.RawMessage, bool) (store.WorkflowForm, error)
}

func (h *handler) workflowForm(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, ok := h.workflowRunHost(w, r, u)
	if !ok {
		return
	}
	id, ok := workflowRunID(w, r)
	if !ok {
		return
	}
	forms, ok := host.(workflowFormHost)
	if !ok {
		fault(w, 503, "workflow_form_unavailable")
		return
	}
	var f store.WorkflowForm
	var err error
	if strings.HasSuffix(r.URL.Path, "/retry") {
		f, err = forms.RetryForm(r.Context(), scope, id)
	} else if r.Method == http.MethodGet {
		f, err = forms.ReadForm(r.Context(), scope, id)
	} else {
		var in struct {
			Version int64           `json:"version"`
			Answers json.RawMessage `json:"answers"`
		}
		if !decodeWorkflow(w, r, &in) {
			return
		}
		if in.Version < 1 || len(in.Answers) == 0 {
			fault(w, 400, "invalid_form_submission")
			return
		}
		final := r.Method == http.MethodPost
		key := r.Header.Get("Idempotency-Key")
		if final && (key == "" || len(key) > 200) {
			fault(w, 400, "idempotency_key_required")
			return
		}
		f, err = forms.SaveForm(r.Context(), scope, id, in.Version, key, in.Answers, final)
	}
	if err != nil {
		var field *workflowforms.FieldError
		switch {
		case errors.Is(err, store.ErrNotFound):
			fault(w, 404, "workflow_form_not_found")
		case errors.Is(err, store.ErrForbidden):
			fault(w, 403, "forbidden")
		case errors.Is(err, store.ErrInvalidAssignee), errors.Is(err, store.ErrUserUnavailable), errors.Is(err, store.ErrAgentUnavailable):
			fault(w, 400, "invalid_assignee")
		case errors.Is(err, store.ErrConflict):
			fault(w, 409, "version_conflict")
		case errors.Is(err, store.ErrWorkflowRunState):
			fault(w, 409, "workflow_form_closed")
		case errors.As(err, &field):
			writeJSON(w, 400, map[string]any{"code": "invalid_form_answer", "key": field.Key, "field": field.Field, "reason": field.Reason})
		default:
			internal(w, "workflow form", err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, f)
}
