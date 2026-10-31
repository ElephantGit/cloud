package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// This suite uses real occurrence SQL and the public callback Stub. It proves HTTP
// consumption and failure handling, not that a runtime executed or persisted the turn.
func TestScheduleResultHTTP(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	token, err := s.Login(ctx, store.Identity{Source: "test", Subject: "result-owner", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := s.Tenants(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := execution.Scope{TenantID: tenants[0].ID, UserID: u.ID}
	cfg, err := s.CreateScheduleConfig(ctx, scope.TenantID, u.ID, store.ScheduleInput{Name: "result", Target: []byte(`{"kind":"chat","prompt":"original"}`), Rule: []byte(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	at := cfg.CreatedAt.Add(time.Hour)
	o, err := s.RecordOccurrence(ctx, scope.TenantID, u.ID, cfg.ID, store.Trigger{Kind: "manual", Key: "result", At: at, Version: 1}, at)
	if err != nil {
		t.Fatal(err)
	}
	ref := execution.Ref{SessionID: "saved-session", TurnID: "saved-turn"}
	encoded, _ := json.Marshal(ref)
	for _, evidence := range []store.ExecutionEvidence{{Event: "begin"}, {Event: "accepted", Ref: string(encoded)}} {
		o, err = s.ApplyOccurrenceEvidence(ctx, scope.TenantID, u.ID, o.ID, o.Version, evidence)
		if err != nil {
			t.Fatal(err)
		}
	}
	caller := execution.Caller{Kind: execution.CallerSchedule, ID: o.ID, AttemptID: o.ID}
	base := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs", scope.TenantID, cfg.ID)
	path := base + "/" + o.ID + "/result"
	end := at.Add(time.Minute)
	full := strings.Repeat("完整回答🙂\n", 70000)
	result := execution.Result{Status: execution.StatusCompleted, Answer: full, AnswerComplete: true, StopReason: "end_turn", Usage: json.RawMessage(`{"inputTokens":4,"outputTokens":9}`)}
	var turn execution.Turn
	var session execution.Session
	var attemptErr, sessionErr error
	reads := 0
	stub := &executiontest.Stub{
		ReadAttemptFunc: func(_ context.Context, got execution.Scope, c execution.Caller) (execution.Turn, error) {
			reads++
			if got != scope || !reflect.DeepEqual(c, caller) {
				t.Errorf("untrusted scope/caller: %+v %+v", got, c)
			}
			return turn, attemptErr
		},
		ReadSessionFunc: func(_ context.Context, got execution.Scope, sid string) (execution.Session, error) {
			reads++
			if got != scope || sid != ref.SessionID {
				t.Errorf("untrusted session scope: %+v %s", got, sid)
			}
			return session, sessionErr
		},
	}
	// Every other public operation, including subscriptions, is forbidden on GET.
	forbidden := func() { t.Error("GET invoked a non-read execution operation") }
	stub.StartFunc = func(context.Context, execution.Scope, execution.StartRequest) (execution.Ref, error) {
		forbidden()
		return execution.Ref{}, execution.ErrUnavailable
	}
	stub.StartModelFunc = func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
		forbidden()
		return execution.Ref{}, execution.ErrUnavailable
	}
	stub.AppendFunc = func(context.Context, execution.Scope, execution.AppendRequest) (execution.Ref, error) {
		forbidden()
		return execution.Ref{}, execution.ErrUnavailable
	}
	stub.CancelFunc = func(context.Context, execution.Scope, execution.Ref) (execution.Result, error) {
		forbidden()
		return execution.Result{}, execution.ErrUnavailable
	}
	stub.EndFunc = func(context.Context, execution.Scope, string) error { forbidden(); return execution.ErrUnavailable }
	stub.SubscribeFunc = func(context.Context, execution.Scope, string, uint64) (execution.Stream, error) {
		forbidden()
		return nil, execution.ErrUnavailable
	}
	stub.ReadTurnsFunc = func(context.Context, execution.Scope, string, string, int) (execution.TurnPage, error) {
		forbidden()
		return execution.TurnPage{}, execution.ErrUnavailable
	}
	handler := server.New(s, server.Options{Execution: stub})
	doc, err := openapi3.NewLoader().LoadFromFile("../../schedule-history.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	call := func(h http.Handler, url, cookie string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest("GET", url, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: status %d want %d: %s", url, w.Code, want, w.Body)
		}
		if strings.Contains(w.Body.String(), "private backend detail") {
			t.Fatal("leaked error detail")
		}
		if cookie == token && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("history can be cached")
		}
		route, params, err := router.FindRoute(req)
		if err != nil {
			t.Fatal(err)
		}
		v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		v.SetBodyBytes(w.Body.Bytes())
		if err = openapi3filter.ValidateResponse(ctx, v); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	reset := func() {
		turn = execution.Turn{Ref: ref, Caller: caller, StartedAt: at, EndedAt: &end, Result: &result}
		session = execution.Session{ID: ref.SessionID, CreatedAt: at, EndedAt: &end}
		attemptErr, sessionErr = nil, nil
	}
	reset()
	t.Run("full answer survives session end and handler replacement", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			out := call(server.New(s, server.Options{Execution: stub}), path, token, 200)
			got := out["result"].(map[string]any)
			if got["answer"] != full || got["answerComplete"] != true || got["status"] != "completed" || got["stopReason"] != "end_turn" || got["failureCode"] != "" || got["usage"].(map[string]any)["outputTokens"] != float64(9) || out["sessionEndedAt"] == nil || out["turnId"] != ref.TurnID {
				t.Fatal("result fidelity lost")
			}
		}
	})
	for _, status := range []execution.Status{execution.StatusFailed, execution.StatusCancelled, execution.StatusInterrupted, execution.StatusCompleted} {
		t.Run("partial "+string(status), func(t *testing.T) {
			reset()
			partial := execution.Result{Status: status, Answer: "partial", StopReason: "stopped", FailureCode: "original_reason"}
			turn.Result = &partial
			out := call(handler, path, token, 200)["result"].(map[string]any)
			if out["status"] != string(status) || out["answer"] != partial.Answer || out["answerComplete"] != false || out["stopReason"] != partial.StopReason || out["failureCode"] != partial.FailureCode || out["usage"] != nil {
				t.Fatal(out)
			}
		})
	}
	t.Run("cancel distinguishes model and agent session lifetime", func(t *testing.T) {
		reset()
		cancelled := execution.Result{Status: execution.StatusCancelled, FailureCode: "cancelled"}
		turn.Result = &cancelled
		session.EndedAt = nil
		out := call(handler, path, token, 200)
		if out["mode"] != "model" || out["sessionEndedAt"] != nil {
			t.Fatal(out)
		}
		session.Target = &execution.Target{ProjectID: "project"}
		session.WorkspaceID, session.EndedAt = "retained-directory", &end
		out = call(handler, path, token, 200)
		if out["mode"] != "agent" || out["workspaceId"] != "retained-directory" || out["sessionEndedAt"] == nil {
			t.Fatal(out)
		}
	})
	t.Run("running has explicit null result", func(t *testing.T) {
		reset()
		turn.Result, turn.EndedAt, session.EndedAt = nil, nil, nil
		if out := call(handler, path, token, 200); out["result"] != nil {
			t.Fatal(out)
		}
	})
	for _, tc := range []struct {
		name   string
		change func()
		status int
		code   string
	}{
		{"wrong session ref", func() { turn.Ref.SessionID = "foreign" }, 502, "execution_reference_mismatch"},
		{"wrong turn ref", func() { turn.Ref.TurnID = "foreign" }, 502, "execution_reference_mismatch"},
		{"wrong caller kind", func() { turn.Caller.Kind = execution.CallerChat }, 502, "execution_reference_mismatch"},
		{"wrong caller id", func() { turn.Caller.ID = "foreign" }, 502, "execution_reference_mismatch"},
		{"wrong attempt", func() { turn.Caller.AttemptID = "foreign" }, 502, "execution_reference_mismatch"},
		{"extra node", func() { turn.Caller.NodeID = "node" }, 502, "execution_reference_mismatch"},
		{"extra rounds", func() { turn.Caller.Rounds = []execution.Round{{ContainerID: "loop"}} }, 502, "execution_reference_mismatch"},
		{"wrong session metadata", func() { session.ID = "foreign" }, 502, "execution_reference_mismatch"},
		{"missing ended result", func() { turn.Result = nil }, 502, "execution_result_incomplete"},
		{"invalid result", func() { invalid := result; invalid.Status = "unknown"; turn.Result = &invalid }, 502, "execution_result_incomplete"},
		{"attempt missing", func() { attemptErr = execution.ErrNotFound }, 404, "execution_result_not_found"},
		{"session missing", func() { sessionErr = execution.ErrNotFound }, 404, "execution_result_not_found"},
		{"attempt unavailable", func() { attemptErr = execution.ErrUnavailable }, 503, "execution_unavailable"},
		{"session unavailable", func() { sessionErr = execution.ErrUnavailable }, 503, "execution_unavailable"},
		{"attempt error with populated data", func() { attemptErr = errors.New("private backend detail") }, 502, "execution_read_failed"},
		{"session error with populated data", func() { sessionErr = errors.New("private backend detail") }, 502, "execution_read_failed"},
		{"context cancelled", func() { attemptErr = context.Canceled }, 502, "execution_read_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			tc.change()
			out := call(handler, path, token, tc.status)
			if out["code"] != tc.code {
				t.Fatal(out)
			}
			if _, ok := out["result"]; ok {
				t.Fatal("leaked untrusted result")
			}
		})
	}
	reset()
	call(server.New(s, server.Options{}), path, token, 503)
	call(server.New(s, server.Options{Execution: &executiontest.Stub{}}), path, token, 503)
	for _, tc := range []struct {
		ref    string
		status int
		code   string
	}{
		{"", 404, "execution_result_not_found"},
		{"not-json", 502, "execution_reference_mismatch"},
		{`{"SessionID":"saved-session"}`, 502, "execution_reference_mismatch"},
		{`{"TurnID":"saved-turn"}`, 502, "execution_reference_mismatch"},
	} {
		backend := resultRecordOverride{Store: s, change: func(item *store.Occurrence) { item.ExecutionRef = tc.ref }}
		before := reads
		out := call(server.New(backend, server.Options{Execution: stub}), path, token, tc.status)
		if out["code"] != tc.code || reads != before {
			t.Fatal("missing/malformed persisted Ref reached execution", out)
		}
	}
	turn.Result, turn.EndedAt, session.EndedAt = nil, nil, nil
	for _, status := range []string{"succeeded", "failed", "cancelled", "interrupted"} {
		backend := resultRecordOverride{Store: s, change: func(item *store.Occurrence) { item.Status = status }}
		if out := call(server.New(backend, server.Options{Execution: stub}), path, token, 502); out["code"] != "execution_result_incomplete" {
			t.Fatal(out)
		}
	}
	reset()
	before := reads
	call(handler, path, "", 401)
	for _, q := range []string{"sessionId=foreign", "turnId=foreign", "caller=foreign", "limit=1"} {
		call(handler, path+"?"+q, token, 400)
	}
	otherToken, err := s.Login(ctx, store.Identity{Source: "test", Subject: "result-other", DisplayName: "Other"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	call(handler, path, otherToken, 404)
	call(handler, strings.Replace(path, scope.TenantID, "00000000-0000-0000-0000-000000000001", 1), token, 404)
	call(handler, strings.Replace(path, cfg.ID, "00000000-0000-0000-0000-000000000001", 1), token, 404)
	call(handler, base+"?limit=1", token, 200)
	call(handler, base+"/"+o.ID, token, 200)
	if reads != before {
		t.Fatal("unauthorized or metadata GET called execution service")
	}
	got, err := s.ScheduleHistoryItem(ctx, scope.TenantID, u.ID, cfg.ID, o.ID)
	if err != nil || !reflect.DeepEqual(got, o) {
		t.Fatalf("GET changed occurrence: %+v %v", got, err)
	}
}

// Fault injection alters the returned durable record only after real SQL authorization.
type resultRecordOverride struct {
	*store.Store
	change func(*store.Occurrence)
}

func (s resultRecordOverride) ScheduleHistoryItem(ctx context.Context, tenant, user, schedule, id string) (store.Occurrence, error) {
	o, err := s.Store.ScheduleHistoryItem(ctx, tenant, user, schedule, id)
	if err == nil {
		s.change(&o)
	}
	return o, err
}
