package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/chat"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// The chat routes are outside api/openapi.json; chat.openapi.json describes them.
const chatBase = "/api/v2/tenants/{tid}/chat"

type chatStore interface {
	Member(ctx context.Context, tenant, user string) (bool, error)
	ChatProjectOrder(ctx context.Context, tenant, user string) ([]string, error)
	SetChatProjectOrder(ctx context.Context, tenant, user string, ids []string) error
}

// heartbeat keeps idle event streams open through proxies.
const heartbeat = 15 * time.Second

func (h *handler) chatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+chatBase+"/executors", h.chat(h.chatExecutors))
	mux.HandleFunc("GET "+chatBase+"/conversations", h.chat(h.chatConversations))
	mux.HandleFunc("POST "+chatBase+"/conversations", h.chat(h.chatCreate))
	mux.HandleFunc("PATCH "+chatBase+"/conversations/{cid}", h.chat(h.chatRename))
	mux.HandleFunc("DELETE "+chatBase+"/conversations/{cid}", h.chat(h.chatDelete))
	mux.HandleFunc("POST "+chatBase+"/conversations/{cid}/messages", h.chat(h.chatSend))
	mux.HandleFunc("POST "+chatBase+"/conversations/{cid}/stop", h.chat(h.chatStop))
	mux.HandleFunc("POST "+chatBase+"/conversations/{cid}/end", h.chat(h.chatRelease))
	mux.HandleFunc("GET "+chatBase+"/conversations/{cid}/sessions/{sid}/events", h.chat(h.chatEvents))
	mux.HandleFunc("GET "+chatBase+"/project-order", h.chat(h.chatProjectOrder))
	mux.HandleFunc("PUT "+chatBase+"/project-order", h.chat(h.chatSetProjectOrder))
}

type chatRequest struct {
	scope execution.Scope
	svc   *chat.Service
	store chatStore
}

// chat authenticates, checks membership of the path's tenant and that chat can run at all.
// Every chat route answers 503 until v2-server is wired to an execution runtime.
func (h *handler) chat(next func(http.ResponseWriter, *http.Request, chatRequest)) http.HandlerFunc {
	return h.authed(func(w http.ResponseWriter, r *http.Request, u store.User) {
		s, ok := h.store.(chatStore)
		if !ok || h.opts.Chat == nil {
			fault(w, http.StatusServiceUnavailable, "execution_unavailable")
			return
		}
		tenant := r.PathValue("tid")
		if !scheduleUUID.MatchString(tenant) {
			fault(w, http.StatusNotFound, "not_found")
			return
		}
		member, e := s.Member(r.Context(), tenant, u.ID)
		if e != nil {
			internal(w, "chat membership", e)
			return
		}
		if !member {
			fault(w, http.StatusNotFound, "not_found")
			return
		}
		next(w, r, chatRequest{scope: execution.Scope{TenantID: tenant, UserID: u.ID}, svc: h.opts.Chat, store: s})
	})
}

type chatExecutor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AgentName string `json:"agentName"`
	ModelName string `json:"modelName"`
}

func (h *handler) chatExecutors(w http.ResponseWriter, r *http.Request, c chatRequest) {
	list, e := c.svc.Executors(r.Context(), c.scope)
	if e != nil {
		chatFault(w, "chat executors", e)
		return
	}
	items := make([]chatExecutor, 0, len(list))
	for _, x := range list {
		items = append(items, chatExecutor{ID: x.ID, Name: x.Name, AgentName: x.AgentName, ModelName: x.Model.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *handler) chatConversations(w http.ResponseWriter, r *http.Request, c chatRequest) {
	list, e := c.svc.Conversations(r.Context(), c.scope)
	if e != nil {
		chatFault(w, "chat conversations", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "nextCursor": ""})
}

type chatTurn struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
}

type chatAdmitted struct {
	Conversation store.ChatConversation `json:"conversation"`
	Turn         chatTurn               `json:"turn"`
}

func (h *handler) chatCreate(w http.ResponseWriter, r *http.Request, c chatRequest) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		ProjectID   string `json:"projectId"`
		WorkspaceID string `json:"workspaceId"`
		ExecutorID  string `json:"executorId"`
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	switch {
	case !boundedID(in.ProjectID):
		fault(w, http.StatusBadRequest, "invalid_project")
		return
	case in.WorkspaceID != "" && !boundedID(in.WorkspaceID):
		fault(w, http.StatusBadRequest, "invalid_workspace")
		return
	case !boundedID(in.ExecutorID):
		fault(w, http.StatusBadRequest, "invalid_executor")
		return
	case len(in.Title) > 200:
		fault(w, http.StatusBadRequest, "invalid_title")
		return
	case !validPrompt(in.Prompt):
		fault(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	conv, ref, e := c.svc.Create(r.Context(), c.scope, chat.CreateRequest{
		Key: key, ProjectID: in.ProjectID, WorkspaceID: in.WorkspaceID, ExecutorID: in.ExecutorID, Title: in.Title, Prompt: in.Prompt,
	})
	if e != nil {
		chatFault(w, "chat create", e)
		return
	}
	writeJSON(w, http.StatusCreated, chatAdmitted{Conversation: conv, Turn: chatTurn{ref.SessionID, ref.TurnID}})
}

func (h *handler) chatSend(w http.ResponseWriter, r *http.Request, c chatRequest) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		Prompt string `json:"prompt"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	if !validPrompt(in.Prompt) {
		fault(w, http.StatusBadRequest, "invalid_prompt")
		return
	}
	conv, ref, e := c.svc.Send(r.Context(), c.scope, r.PathValue("cid"), key, in.Prompt)
	if e != nil {
		chatFault(w, "chat send", e)
		return
	}
	writeJSON(w, http.StatusCreated, chatAdmitted{Conversation: conv, Turn: chatTurn{ref.SessionID, ref.TurnID}})
}

func (h *handler) chatRename(w http.ResponseWriter, r *http.Request, c chatRequest) {
	var in struct {
		Title string `json:"title"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		fault(w, http.StatusBadRequest, "invalid_title")
		return
	}
	conv, e := c.svc.Rename(r.Context(), c.scope, r.PathValue("cid"), title)
	if e != nil {
		chatFault(w, "chat rename", e)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

func (h *handler) chatDelete(w http.ResponseWriter, r *http.Request, c chatRequest) {
	if e := c.svc.Delete(r.Context(), c.scope, r.PathValue("cid")); e != nil {
		chatFault(w, "chat delete", e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type chatResult struct {
	Status         execution.Status `json:"status"`
	Answer         string           `json:"answer"`
	AnswerComplete bool             `json:"answerComplete"`
	StopReason     string           `json:"stopReason"`
	FailureCode    string           `json:"failureCode"`
	Usage          json.RawMessage  `json:"usage,omitempty"`
}

func resultOf(r execution.Result) chatResult {
	return chatResult{r.Status, r.Answer, r.AnswerComplete, r.StopReason, r.FailureCode, r.Usage}
}

func (h *handler) chatStop(w http.ResponseWriter, r *http.Request, c chatRequest) {
	result, e := c.svc.Stop(r.Context(), c.scope, r.PathValue("cid"))
	if e != nil {
		chatFault(w, "chat stop", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": resultOf(result)})
}

func (h *handler) chatRelease(w http.ResponseWriter, r *http.Request, c chatRequest) {
	conv, e := c.svc.Release(r.Context(), c.scope, r.PathValue("cid"))
	if e != nil {
		chatFault(w, "chat release", e)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

type chatEvent struct {
	Sequence   uint64                     `json:"sequence"`
	OccurredAt time.Time                  `json:"occurredAt"`
	Kind       execution.EventKind        `json:"kind"`
	TurnID     string                     `json:"turnId"`
	History    *execution.HistorySnapshot `json:"history,omitempty"`
	TextDelta  string                     `json:"textDelta,omitempty"`
	Result     *chatResult                `json:"result,omitempty"`
}

// chatEvents relays one session's events as Server-Sent Events. Each event's id is its session
// sequence, so a reconnecting EventSource resumes after the last one it saw. A stream that ends
// with the session sends an "end" event; a failure sends "failure" with a code instead of looking
// like a clean end. Closing the stream stops observation only, never the Agent.
func (h *handler) chatEvents(w http.ResponseWriter, r *http.Request, c chatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		internal(w, "chat events", errors.New("response writer cannot flush"))
		return
	}
	after, ok := eventCursor(r)
	if !ok {
		fault(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	ctx := r.Context()
	sid := r.PathValue("sid")
	if _, e := c.svc.Session(ctx, c.scope, r.PathValue("cid"), sid); e != nil {
		chatFault(w, "chat events", e)
		return
	}
	stream, e := c.svc.Subscribe(ctx, c.scope, sid, after)
	if e != nil {
		chatFault(w, "chat subscribe", e)
		return
	}

	type next struct {
		event execution.Event
		err   error
	}
	events := make(chan next)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			ev, e := stream.Next(ctx)
			select {
			case events <- next{ev, e}:
			case <-ctx.Done():
				return
			}
			if e != nil {
				return
			}
		}
	}()
	// Close unblocks Next; wait so the goroutine never outlives the request.
	defer wg.Wait()
	defer func() { _ = stream.Close() }()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_, _ = io.WriteString(w, ": ping\n\n")
		case n := <-events:
			switch {
			case errors.Is(n.err, io.EOF):
				_, _ = io.WriteString(w, "event: end\ndata: {}\n\n")
				flusher.Flush()
				return
			case n.err != nil:
				if ctx.Err() == nil {
					slog.Error("chat events", "session", sid, "error", n.err)
					writeEvent(w, "failure", "", map[string]string{"code": eventFailure(n.err)})
					flusher.Flush()
				}
				return
			}
			ev := n.event
			out := chatEvent{Sequence: ev.Sequence, OccurredAt: ev.OccurredAt, Kind: ev.Kind, TurnID: ev.Ref.TurnID, History: ev.History, TextDelta: ev.TextDelta}
			if ev.Result != nil {
				res := resultOf(*ev.Result)
				out.Result = &res
			}
			writeEvent(w, "", strconv.FormatUint(ev.Sequence, 10), out)
		}
		flusher.Flush()
	}
}

func writeEvent(w io.Writer, name, id string, v any) {
	data, e := json.Marshal(v)
	if e != nil {
		slog.Error("encode chat event", "error", e)
		return
	}
	if name != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", name)
	}
	if id != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", id)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

// eventCursor reads the resume point: Last-Event-ID from a reconnecting EventSource, else the
// after query parameter, else the start.
func eventCursor(r *http.Request) (uint64, bool) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw == "" {
		return 0, true
	}
	n, e := strconv.ParseUint(raw, 10, 64)
	return n, e == nil
}

func eventFailure(e error) string {
	if errors.Is(e, execution.ErrInvalidRequest) {
		return "invalid_cursor"
	}
	return "events_unavailable"
}

func (h *handler) chatProjectOrder(w http.ResponseWriter, r *http.Request, c chatRequest) {
	ids, e := c.store.ChatProjectOrder(r.Context(), c.scope.TenantID, c.scope.UserID)
	if e != nil {
		internal(w, "chat project order", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projectIds": ids})
}

func (h *handler) chatSetProjectOrder(w http.ResponseWriter, r *http.Request, c chatRequest) {
	var in struct {
		ProjectIDs []string `json:"projectIds"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	seen := map[string]bool{}
	for _, id := range in.ProjectIDs {
		if !boundedID(id) || seen[id] {
			fault(w, http.StatusBadRequest, "invalid_project_order")
			return
		}
		seen[id] = true
	}
	if in.ProjectIDs == nil || len(in.ProjectIDs) > 500 {
		fault(w, http.StatusBadRequest, "invalid_project_order")
		return
	}
	if e := c.store.SetChatProjectOrder(r.Context(), c.scope.TenantID, c.scope.UserID, in.ProjectIDs); e != nil {
		internal(w, "save chat project order", e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projectIds": in.ProjectIDs})
}

// chatFault maps the chat service's and the execution port's errors to stable codes.
func chatFault(w http.ResponseWriter, op string, e error) {
	var busy *chat.BusyError
	switch {
	case errors.As(e, &busy):
		params := map[string]any{}
		if o := busy.Occupant; o != nil {
			params["conversationId"] = o.ID
			params["title"] = o.Title
		}
		faultParams(w, http.StatusConflict, "workspace_busy", params)
	case errors.Is(e, store.ErrNotFound), errors.Is(e, execution.ErrNotFound):
		fault(w, http.StatusNotFound, "not_found")
	case errors.Is(e, store.ErrKeyReused), errors.Is(e, execution.ErrConflict):
		fault(w, http.StatusConflict, "idempotency_conflict")
	case errors.Is(e, chat.ErrNoActiveTurn):
		fault(w, http.StatusConflict, "no_active_turn")
	case errors.Is(e, execution.ErrUnsupported):
		fault(w, http.StatusBadRequest, "unsupported_content")
	case errors.Is(e, execution.ErrInvalidRequest):
		fault(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(e, execution.ErrAdmissionUnknown), errors.Is(e, context.DeadlineExceeded):
		// The same request, with the same Idempotency-Key, settles the outcome.
		fault(w, http.StatusServiceUnavailable, "admission_unknown")
	case errors.Is(e, execution.ErrStopUnconfirmed):
		fault(w, http.StatusServiceUnavailable, "stop_unconfirmed")
	case errors.Is(e, execution.ErrUnavailable):
		fault(w, http.StatusServiceUnavailable, "execution_unavailable")
	default:
		internal(w, op, e)
	}
}

// idempotencyKey reads the Idempotency-Key a POST that admits a turn must carry: it is the turn's
// attempt identity, so retrying with the same key can never run the prompt twice.
func idempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 200 {
		fault(w, http.StatusBadRequest, "idempotency_key_required")
		return "", false
	}
	return key, true
}

// decodeStrict decodes exactly one JSON object of at most 256 KiB with no unknown fields.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		fault(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func boundedID(id string) bool { return id != "" && len(id) <= 200 && strings.TrimSpace(id) == id }

func validPrompt(p string) bool { return strings.TrimSpace(p) != "" && len(p) <= 200<<10 }
