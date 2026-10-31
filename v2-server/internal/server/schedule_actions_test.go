package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/execution/executiontest"
	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

type actionFixture struct {
	db     *store.Store
	scope  execution.Scope
	token  string
	config store.ScheduleConfig
	now    atomic.Int64
}

func newActionFixture(t *testing.T) *actionFixture {
	t.Helper()
	f := &actionFixture{db: openStore(t)}
	ctx := context.Background()
	var err error
	f.token, err = f.db.Login(ctx, store.Identity{Source: "test", Subject: "actions", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.db.SessionUser(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := f.db.Tenants(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.scope = execution.Scope{TenantID: ts[0].ID, UserID: u.ID}
	f.config, err = f.db.CreateScheduleConfig(ctx, f.scope.TenantID, f.scope.UserID, store.ScheduleInput{Name: "actions", Target: json.RawMessage(`{"kind":"chat","prompt":"hello","model":"fixture"}`), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)})
	if err != nil {
		t.Fatal(err)
	}
	f.now.Store(f.config.CreatedAt.Add(time.Minute).UnixNano())
	return f
}

func (f *actionFixture) host(t *testing.T, stub *executiontest.Stub) (*scheduler.Host, chan time.Time, func()) {
	t.Helper()
	c, err := server.NewScheduleCoordinator(f.db, server.Options{Execution: stub}, func() time.Time { return time.Unix(0, f.now.Load()) })
	if err != nil {
		t.Fatal(err)
	}
	h, err := scheduler.NewHost(f.db, c, stub)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ticks := make(chan time.Time)
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx, ticks) }()
	select {
	case <-h.Ready():
	case <-time.After(10 * time.Second):
		t.Fatal("host startup timed out")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("host did not join")
			}
		})
	}
	t.Cleanup(stop)
	return h, ticks, stop
}

func actionClient(t *testing.T, f *actionFixture, h server.ScheduleActions) func(string, any, string) (int, map[string]any) {
	t.Helper()
	srv := httptest.NewServer(server.New(f.db, server.Options{Schedules: h}))
	t.Cleanup(srv.Close)
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schedule-actions.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return func(path string, in any, token string) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req, err := http.NewRequest("POST", srv.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		route, params, err := router.FindRoute(req)
		if err != nil {
			t.Fatal(err)
		}
		v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: res.StatusCode, Header: res.Header, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		v.SetBodyBytes(body)
		if err = openapi3filter.ValidateResponse(context.Background(), v); err != nil {
			t.Fatalf("%s: %v\n%s", path, err, body)
		}
		var out map[string]any
		if err = json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		if _, ok := out["executionRef"]; ok {
			t.Fatal("leaked execution ref")
		}
		if _, ok := out["snapshot"]; ok {
			t.Fatal("leaked prompt snapshot")
		}
		return res.StatusCode, out
	}
}

func TestScheduleActionsAndAutomaticScanShareCoordinator(t *testing.T) {
	f := newActionFixture(t)
	var mu sync.Mutex
	turns := map[string]execution.Turn{}
	var starts, cancels atomic.Int64
	var stopUnknown atomic.Bool
	stub := &executiontest.Stub{
		ListModelsFunc: func(context.Context, execution.Scope) ([]execution.Model, error) {
			return []execution.Model{{ID: "fixture"}}, nil
		},
		StartModelFunc: func(_ context.Context, scope execution.Scope, in execution.ModelStartRequest) (execution.Ref, error) {
			if scope != f.scope {
				return execution.Ref{}, execution.ErrNotFound
			}
			mu.Lock()
			defer mu.Unlock()
			starts.Add(1)
			ref := execution.Ref{SessionID: "session-" + in.Caller.ID, TurnID: "turn-" + in.Caller.ID}
			turns[in.Caller.ID] = execution.Turn{Ref: ref, Caller: in.Caller}
			return ref, nil
		},
		ReadAttemptFunc: func(_ context.Context, _ execution.Scope, c execution.Caller) (execution.Turn, error) {
			mu.Lock()
			defer mu.Unlock()
			v, ok := turns[c.ID]
			if !ok {
				return v, execution.ErrNotFound
			}
			return v, nil
		},
		CancelFunc: func(context.Context, execution.Scope, execution.Ref) (execution.Result, error) {
			cancels.Add(1)
			if stopUnknown.Load() {
				return execution.Result{}, execution.ErrStopUnconfirmed
			}
			return execution.Result{Status: execution.StatusCancelled}, nil
		},
		EndFunc: func(context.Context, execution.Scope, string) error { return nil },
	}
	h, ticks, stop := f.host(t, stub)
	call := actionClient(t, f, h)
	base := "/api/v2/tenants/" + f.scope.TenantID + "/schedule-configs/" + f.config.ID
	f.now.Store(f.config.CreatedAt.Add(time.Hour).UnixNano())
	// A second unbuffered tick can be received only after the first scan finishes.
	for i := 0; i < 2; i++ {
		select {
		case ticks <- time.Now():
		case <-time.After(10 * time.Second):
			t.Fatal("scan blocked")
		}
	}
	items, err := f.db.Occurrences(t.Context(), f.scope.TenantID, f.scope.UserID, f.config.ID, store.OccurrenceCursor{}, 10)
	if err != nil || len(items) != 1 || items[0].Kind != "scheduled" || items[0].Status != "running" {
		t.Fatalf("automatic: %+v %v", items, err)
	}
	first := items[0]
	code, out := call(base+"/run", map[string]string{"requestId": "overlap"}, f.token)
	if code != 200 || out["status"] != "skipped" || out["reason"] != "previous_unfinished" || starts.Load() != 1 {
		t.Fatalf("overlap: %d %v", code, out)
	}
	code, out = call(base+"/runs/"+first.ID+"/cancel", map[string]any{}, f.token)
	if code != 200 || out["status"] != "cancelled" || out["released"] != true {
		t.Fatalf("cancel: %d %v", code, out)
	}
	// A repeated stop returns the same terminal record without another public Cancel.
	code, _ = call(base+"/runs/"+first.ID+"/cancel", map[string]any{}, f.token)
	if code != 200 || cancels.Load() != 1 {
		t.Fatal("duplicate Cancel")
	}
	code, out = call(base+"/run", map[string]string{"requestId": "manual-1"}, f.token)
	if code != 200 || out["status"] != "running" {
		t.Fatalf("manual: %d %v", code, out)
	}
	id := out["id"].(string)
	code, retry := call(base+"/run", map[string]string{"requestId": "manual-1"}, f.token)
	if code != 200 || retry["id"] != id || starts.Load() != 2 {
		t.Fatalf("retry: %d %v", code, retry)
	}
	stopUnknown.Store(true)
	code, out = call(base+"/runs/"+id+"/cancel", map[string]any{}, f.token)
	if code != 503 || out["code"] != "stop_unconfirmed" {
		t.Fatalf("unknown stop: %d %v", code, out)
	}
	o, err := f.db.GetOccurrence(t.Context(), f.scope.TenantID, f.scope.UserID, id)
	if err != nil || o.Status != "cancelling" || o.Released {
		t.Fatalf("false cancellation: %+v %v", o, err)
	}
	stopUnknown.Store(false)
	code, out = call(base+"/runs/"+id+"/cancel", map[string]any{}, f.token)
	if code != 200 || out["released"] != true {
		t.Fatalf("confirmed stop: %d %v", code, out)
	}
	// Concurrent network retries share the stable manual key and one public admission.
	ids := make(chan string, 8)
	var retries sync.WaitGroup
	for i := 0; i < 8; i++ {
		retries.Add(1)
		go func() {
			defer retries.Done()
			code, out := call(base+"/run", map[string]string{"requestId": "concurrent"}, f.token)
			if code != 200 {
				t.Errorf("concurrent retry: %d %v", code, out)
				return
			}
			ids <- out["id"].(string)
		}()
	}
	retries.Wait()
	close(ids)
	concurrentID := ""
	count := 0
	for id := range ids {
		count++
		if concurrentID != "" && id != concurrentID {
			t.Fatal("duplicate HTTP admission")
		}
		concurrentID = id
	}
	if count != 8 || starts.Load() != 3 {
		t.Fatalf("retries=%d starts=%d", count, starts.Load())
	}
	if code, out := call(base+"/runs/"+concurrentID+"/cancel", map[string]any{}, f.token); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	stop()
	code, out = call(base+"/run", map[string]string{"requestId": "after-shutdown"}, f.token)
	if code != 503 || starts.Load() != 3 {
		t.Fatalf("shutdown admission: %d %v", code, out)
	}
}

func TestScheduleActionAuthorizationValidationAndUnavailable(t *testing.T) {
	f := newActionFixture(t)
	h, ticks, _ := f.host(t, &executiontest.Stub{})
	call := actionClient(t, f, h)
	base := "/api/v2/tenants/" + f.scope.TenantID + "/schedule-configs/" + f.config.ID
	for _, in := range []any{map[string]string{"requestId": ""}, map[string]string{"requestId": " x "}, map[string]string{"requestId": "x", "userId": "forged"}, nil} {
		if code, _ := call(base+"/run", in, f.token); code != 400 {
			t.Fatalf("invalid body: %d", code)
		}
	}
	if code, _ := call(base+"/run", map[string]string{"requestId": "x"}, ""); code != 401 {
		t.Fatal(code)
	}
	other, err := f.db.Login(t.Context(), store.Identity{Source: "test", Subject: "other", DisplayName: "Other"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := call(base+"/run", map[string]string{"requestId": "x"}, other); code != 404 {
		t.Fatal(code)
	}
	if code, out := call(base+"/run", map[string]string{"requestId": "x"}, f.token); code != 503 || out["code"] != "schedule_execution_unavailable" {
		t.Fatalf("%d %v", code, out)
	}
	f.now.Store(f.config.CreatedAt.Add(48 * time.Hour).UnixNano())
	for i := 0; i < 2; i++ {
		select {
		case ticks <- time.Now():
		case <-time.After(10 * time.Second):
			t.Fatal("scan blocked")
		}
	}
	items, err := f.db.Occurrences(t.Context(), f.scope.TenantID, f.scope.UserID, f.config.ID, store.OccurrenceCursor{}, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("unavailable created records: %+v %v", items, err)
	}
	// A valid occurrence cannot be cancelled through a different owned schedule URL.
	o, err := f.db.RecordOccurrence(t.Context(), f.scope.TenantID, f.scope.UserID, f.config.ID, store.Trigger{Kind: "manual", Key: "record", At: time.Now(), Version: 1}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []any{nil, map[string]string{"executionRef": "forged"}} {
		if code, _ := call(base+"/runs/"+o.ID+"/cancel", body, f.token); code != 400 {
			t.Fatalf("invalid cancel body: %d", code)
		}
	}
	otherConfig, err := f.db.CreateScheduleConfig(t.Context(), f.scope.TenantID, f.scope.UserID, store.ScheduleInput{Name: "other", Target: f.config.Target, Rule: f.config.Rule})
	if err != nil {
		t.Fatal(err)
	}
	wrong := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs/%s/cancel", f.scope.TenantID, otherConfig.ID, o.ID)
	if code, _ := call(wrong, map[string]any{}, f.token); code != 404 {
		t.Fatalf("mismatched parent: %d", code)
	}
	nilCall := actionClient(t, f, nil)
	if code, _ := nilCall(base+"/run", map[string]string{"requestId": "nil-host"}, f.token); code != 503 {
		t.Fatal(code)
	}

	// Missing model during StartModel marks the occurrence skipped and returns 409 with occurrenceId (not 404).
	missingModelStub := &executiontest.Stub{
		ListModelsFunc: func(context.Context, execution.Scope) ([]execution.Model, error) {
			return []execution.Model{{ID: "other"}}, nil
		},
		StartModelFunc: func(context.Context, execution.Scope, execution.ModelStartRequest) (execution.Ref, error) {
			return execution.Ref{}, execution.ErrNotFound
		},
	}
	mh, _, _ := f.host(t, missingModelStub)
	missingCall := actionClient(t, f, mh)
	code, out := missingCall(base+"/run", map[string]string{"requestId": "missing-model"}, f.token)
	params, _ := out["params"].(map[string]any)
	occID, _ := params["occurrenceId"].(string)
	if code != 409 || out["code"] != "execution_target_unavailable" || occID == "" {
		t.Fatalf("missing target error: %d %v", code, out)
	}
	persisted, err := f.db.GetOccurrence(t.Context(), f.scope.TenantID, f.scope.UserID, occID)
	if err != nil || persisted.Status != "skipped" || persisted.Reason != "execution_target_unavailable" || !persisted.Released {
		t.Fatalf("persisted skipped occurrence: %+v %v", persisted, err)
	}
}

func TestScheduleHostShutdownCancelsAndJoinsAdmission(t *testing.T) {
	f := newActionFixture(t)
	entered := make(chan struct{})
	stub := &executiontest.Stub{ListModelsFunc: func(context.Context, execution.Scope) ([]execution.Model, error) { return nil, nil }, StartModelFunc: func(ctx context.Context, _ execution.Scope, _ execution.ModelStartRequest) (execution.Ref, error) {
		close(entered)
		<-ctx.Done()
		return execution.Ref{}, ctx.Err()
	}}
	h, _, stop := f.host(t, stub)
	result := make(chan error, 1)
	go func() { _, err := h.Manual(context.Background(), f.scope, f.config.ID, "blocked"); result <- err }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("admission did not start")
	}
	stop()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("host returned before operation finished")
	}
}
