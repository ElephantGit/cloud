package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/taskrun"
)

// taskRunStore is the part of the database the task timeline and the collaboration targets need.
type taskRunStore interface {
	TaskTimeline(ctx context.Context, tenant, caller, taskID string) ([]store.TaskRun, []store.TaskComment, error)
	CollaborationAgents(ctx context.Context, tenant, caller string) ([]store.TaskAssignee, error)
	PendingMentions(ctx context.Context, tenant, caller string) ([]store.PendingMention, error)
}

// maxCommentBody bounds a comment like a task description: a prompt-sized brief, not a document.
const maxCommentBody = 4000

func (h *handler) taskRunRoutes(mux *http.ServeMux) {
	base := "/api/v2/tenants/{tid}/tasks/{taskid}"
	mux.HandleFunc("GET "+base+"/timeline", h.authed(h.taskTimeline))
	mux.HandleFunc("GET "+base+"/executors", h.authed(h.taskExecutors))
	mux.HandleFunc("POST "+base+"/runs", h.authed(h.startTaskRun))
	mux.HandleFunc("POST "+base+"/runs/{rid}/stop", h.authed(h.stopTaskRun))
	mux.HandleFunc("GET "+base+"/runs/{rid}/events", h.authed(h.taskRunEvents))
	mux.HandleFunc("POST "+base+"/comments", h.authed(h.createTaskComment))
	mux.HandleFunc("GET /api/v1/tenants/{tid}/collaboration/targets", h.authed(h.collaborationTargets))
	mux.HandleFunc("GET /api/v2/tenants/{tid}/task-mentions", h.authed(h.pendingMentions))
}

// pendingMentions lists the caller's mentions still waiting on their reply, one per open task.
func (h *handler) pendingMentions(w http.ResponseWriter, r *http.Request, u store.User) {
	s, ok := h.store.(taskRunStore)
	tenant := r.PathValue("tid")
	if !ok {
		fault(w, http.StatusServiceUnavailable, "task_unavailable")
		return
	}
	if !taskUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	items, e := s.PendingMentions(r.Context(), tenant, u.ID)
	if errors.Is(e, store.ErrNotFound) {
		fault(w, http.StatusForbidden, "forbidden")
		return
	}
	if e != nil {
		internal(w, "pending mentions", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// taskRunScope checks the path ids and that task runs are wired; the host checks membership.
func (h *handler) taskRunScope(w http.ResponseWriter, r *http.Request, u store.User) (*taskrun.Host, execution.Scope, string, bool) {
	if h.opts.TaskRuns == nil {
		fault(w, http.StatusServiceUnavailable, "execution_unavailable")
		return nil, execution.Scope{}, "", false
	}
	tenant, id, run := r.PathValue("tid"), r.PathValue("taskid"), r.PathValue("rid")
	if !taskUUID.MatchString(tenant) || !taskUUID.MatchString(id) || (run != "" && !taskUUID.MatchString(run)) {
		fault(w, http.StatusNotFound, "not_found")
		return nil, execution.Scope{}, "", false
	}
	return h.opts.TaskRuns, execution.Scope{TenantID: tenant, UserID: u.ID}, id, true
}

// taskRunFault maps the run host's, the task store's and the execution port's errors.
func taskRunFault(w http.ResponseWriter, op string, e error) {
	switch {
	case errors.Is(e, store.ErrRunActive):
		fault(w, http.StatusConflict, "run_active")
	case errors.Is(e, taskrun.ErrRunOwned):
		fault(w, http.StatusConflict, "run_owned")
	case errors.Is(e, taskrun.ErrNotAgentTask):
		fault(w, http.StatusConflict, "not_agent_task")
	case errors.Is(e, taskrun.ErrTaskClosed):
		fault(w, http.StatusConflict, "task_closed")
	case errors.Is(e, taskrun.ErrNoLiveRun):
		fault(w, http.StatusConflict, "no_live_run")
	case errors.Is(e, taskrun.ErrNoExecutor):
		fault(w, http.StatusBadRequest, "invalid_executor")
	case errors.Is(e, store.ErrInvalidMention):
		fault(w, http.StatusBadRequest, "invalid_mention")
	case errors.Is(e, store.ErrForbidden):
		fault(w, http.StatusForbidden, "forbidden")
	default:
		chatFault(w, op, e)
	}
}

func (h *handler) taskTimeline(w http.ResponseWriter, r *http.Request, u store.User) {
	s, ok := h.store.(taskRunStore)
	tenant, id := r.PathValue("tid"), r.PathValue("taskid")
	if !ok {
		fault(w, http.StatusServiceUnavailable, "task_unavailable")
		return
	}
	if !taskUUID.MatchString(tenant) || !taskUUID.MatchString(id) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	runs, comments, e := s.TaskTimeline(r.Context(), tenant, u.ID, id)
	if e != nil {
		taskRunFault(w, "task timeline", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "comments": comments})
}

func (h *handler) taskExecutors(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, id, ok := h.taskRunScope(w, r, u)
	if !ok {
		return
	}
	list, e := host.Executors(r.Context(), scope, id)
	if e != nil {
		taskRunFault(w, "task executors", e)
		return
	}
	items := make([]chatExecutor, 0, len(list))
	for _, x := range list {
		items = append(items, chatExecutor{ID: x.ID, Name: x.Name, AgentID: x.AgentID, AgentName: x.AgentName, ModelID: x.Model.ID, ModelName: x.Model.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) startTaskRun(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, id, ok := h.taskRunScope(w, r, u)
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		ExecutorID string `json:"executorId"`
		Note       string `json:"note"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	if len(in.ExecutorID) > 200 || utf8.RuneCountInString(in.Note) > maxCommentBody {
		fault(w, http.StatusBadRequest, "invalid_request")
		return
	}
	run, e := host.Start(r.Context(), scope, id, key, taskrun.StartInput{ExecutorID: in.ExecutorID, Note: in.Note})
	if e != nil {
		taskRunFault(w, "start task run", e)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *handler) stopTaskRun(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, id, ok := h.taskRunScope(w, r, u)
	if !ok {
		return
	}
	run, e := host.Stop(r.Context(), scope, id, r.PathValue("rid"))
	if e != nil {
		taskRunFault(w, "stop task run", e)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// taskRunEvents relays a run's session to any member of the workspace, in the chat stream's shape.
func (h *handler) taskRunEvents(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, id, ok := h.taskRunScope(w, r, u)
	if !ok {
		return
	}
	after, ok := eventCursor(r)
	if !ok {
		fault(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rid := r.PathValue("rid")
	stream, run, e := host.Events(r.Context(), scope, id, rid, after)
	if e != nil {
		taskRunFault(w, "task run subscribe", e)
		return
	}
	relaySessionEvents(w, r, stream, func(ctx context.Context, _, turnID string) (string, error) {
		return host.TurnPrompt(ctx, run, turnID)
	}, "task run", rid)
}

type taskMentionRequest struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (h *handler) createTaskComment(w http.ResponseWriter, r *http.Request, u store.User) {
	host, scope, id, ok := h.taskRunScope(w, r, u)
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		Body       string               `json:"body"`
		Mentions   []taskMentionRequest `json:"mentions"`
		ToAgent    bool                 `json:"toAgent"`
		ExecutorID string               `json:"executorId"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" || utf8.RuneCountInString(body) > maxCommentBody {
		fault(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if len(in.Mentions) > 20 || len(in.ExecutorID) > 200 {
		fault(w, http.StatusBadRequest, "invalid_request")
		return
	}
	mentions := make([]store.TaskMention, 0, len(in.Mentions))
	for _, m := range in.Mentions {
		if (m.Type != "user" && m.Type != "agent") || !taskUUID.MatchString(m.ID) {
			fault(w, http.StatusBadRequest, "invalid_mention")
			return
		}
		mentions = append(mentions, store.TaskMention{Type: m.Type, ID: m.ID})
	}
	comment, e := host.Comment(r.Context(), scope, id, key, taskrun.CommentInput{Body: body, Mentions: mentions, ToAgent: in.ToAgent, ExecutorID: in.ExecutorID})
	if e != nil {
		taskRunFault(w, "create task comment", e)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

// collaborationTarget is the contract's CollaborationTarget.
type collaborationTarget struct {
	Type                  string                `json:"type"`
	ID                    string                `json:"id"`
	DisplayName           string                `json:"displayName"`
	Description           string                `json:"description"`
	InteractionDescriptor interactionDescriptor `json:"interactionDescriptor"`
}

type interactionDescriptor struct {
	Mode         string  `json:"mode"`
	RequiresTask bool    `json:"requiresTask"`
	FormRef      *string `json:"formRef"`
}

// collaborationTargets implements the contract's @ directory for the subset v2 routes: workspace
// members are mentioned, installed Space Agents take the message as a task. Squads and workflows
// are not offered: v2 does not dispatch to a squad, and workflows run from their own page.
func (h *handler) collaborationTargets(w http.ResponseWriter, r *http.Request, u store.User) {
	s, ok := h.store.(taskRunStore)
	tenant := r.PathValue("tid")
	if !ok {
		fault(w, http.StatusServiceUnavailable, "task_unavailable")
		return
	}
	if !taskUUID.MatchString(tenant) {
		fault(w, http.StatusNotFound, "not_found")
		return
	}
	members, e := h.store.TenantMembers(r.Context(), tenant, u.ID)
	if errors.Is(e, store.ErrNotFound) {
		fault(w, http.StatusForbidden, "forbidden")
		return
	}
	if e != nil {
		internal(w, "collaboration members", e)
		return
	}
	agents, e := s.CollaborationAgents(r.Context(), tenant, u.ID)
	if e != nil {
		internal(w, "collaboration agents", e)
		return
	}
	items := make([]collaborationTarget, 0, len(members)+len(agents))
	for _, m := range members {
		items = append(items, collaborationTarget{Type: "user", ID: m.UserID, DisplayName: m.DisplayName, Description: "工作区成员",
			InteractionDescriptor: interactionDescriptor{Mode: "mention"}})
	}
	for _, a := range agents {
		items = append(items, collaborationTarget{Type: "agent", ID: a.ID, DisplayName: a.DisplayName, Description: a.Identifier,
			InteractionDescriptor: interactionDescriptor{Mode: "task", RequiresTask: true}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": ""})
}
