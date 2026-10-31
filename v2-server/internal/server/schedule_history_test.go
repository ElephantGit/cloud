package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestScheduleHistoryHTTP(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	token, err := s.Login(ctx, store.Identity{Source: "test", Subject: "history-owner", DisplayName: "Owner"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.SessionUser(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tenants, err := s.Tenants(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	tenant := tenants[0].ID
	input := store.ScheduleInput{Name: "history", Target: json.RawMessage(`{"kind":"chat","prompt":"original"}`), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)}
	cfg, err := s.CreateScheduleConfig(ctx, tenant, user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.CreateScheduleConfig(ctx, tenant, user.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.New(s, server.Options{}))
	defer func() { srv.Close() }()
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
	call := func(path, auth string, want int) map[string]any {
		t.Helper()
		req, err := http.NewRequest("GET", srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if auth != "" {
			req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: auth})
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s: status %d want %d body %s", path, res.StatusCode, want, raw)
		}
		route, params, err := router.FindRoute(req)
		if err != nil {
			t.Fatal(err)
		}
		v := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: res.StatusCode, Header: res.Header, Options: &openapi3filter.Options{IncludeResponseStatus: true}}
		v.SetBodyBytes(raw)
		if err = openapi3filter.ValidateResponse(ctx, v); err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		if err = json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	path := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs", tenant, cfg.ID)
	emptyPath := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs", tenant, empty.ID)
	if out := call(emptyPath, token, 200); len(out["items"].([]any)) != 0 || out["nextCursor"] != "" {
		t.Fatal(out)
	}
	call(path, "", 401)
	call(path, "invalid", 401)
	expected := map[string]store.Occurrence{}
	// Each transition is written through the actual domain API. GET must not reconcile it.
	scenarios := [][]string{{}, {"begin"}, {"begin", "reconcile"}, {"begin", "reconcile", "recover"}, {"begin", "accepted"}, {"begin", "accepted", "cancel_requested"}, {"begin", "accepted", "completed"}, {"begin", "accepted", "failed"}, {"begin", "accepted", "cancelled"}, {"begin", "accepted", "interrupted"}, {"begin", "rejected"}, {"begin", "reconcile", "not_accepted"}, {"begin", "accepted", "completed", "released"}}
	for i, events := range scenarios {
		// Separate schedules allow all occupied states to coexist; use the main schedule
		// for pagination records below and each dedicated schedule for state assertions.
		config, e := s.CreateScheduleConfig(ctx, tenant, user.ID, input)
		if e != nil {
			t.Fatal(e)
		}
		at := config.CreatedAt.Add(time.Hour)
		o, e := s.RecordOccurrence(ctx, tenant, user.ID, config.ID, store.Trigger{Kind: "manual", Key: fmt.Sprint(i), Version: 1, At: at}, at)
		if e != nil {
			t.Fatal(e)
		}
		for _, event := range events {
			ref := ""
			if event == "accepted" || event == "completed" || event == "failed" || event == "cancelled" || event == "interrupted" || event == "released" {
				ref = "stable-ref"
			}
			reason := ""
			if event == "rejected" {
				reason = "execution_busy"
			}
			o, e = s.ApplyOccurrenceEvidence(ctx, tenant, user.ID, o.ID, o.Version, store.ExecutionEvidence{Event: event, Ref: ref, Reason: reason})
			if e != nil {
				t.Fatal(e)
			}
		}
		detail := fmt.Sprintf("/api/v2/tenants/%s/schedule-configs/%s/runs/%s", tenant, config.ID, o.ID)
		raw := call(detail, token, 200)
		switch eventsLast(events) {
		case "reconcile":
			if raw["status"] != "reconciling" || raw["reason"] != "lookup_required" {
				t.Fatal(raw)
			}
		case "rejected":
			if raw["status"] != "skipped" || raw["reason"] != "execution_busy" {
				t.Fatal(raw)
			}
		case "not_accepted":
			if raw["status"] != "skipped" || raw["reason"] != "execution_not_started" {
				t.Fatal(raw)
			}
		}
		data, _ := json.Marshal(raw)
		var got store.Occurrence
		json.Unmarshal(data, &got)
		var wantSnapshot, gotSnapshot any
		json.Unmarshal(o.Snapshot, &wantSnapshot)
		json.Unmarshal(got.Snapshot, &gotSnapshot)
		if !reflect.DeepEqual(gotSnapshot, wantSnapshot) || got.Status != o.Status || got.Released != o.Released || got.Version != o.Version || got.Reason != o.Reason {
			t.Fatalf("GET changed state: %+v want %+v", got, o)
		}
		call(path+"/"+o.ID, token, 404)
	}
	base := cfg.CreatedAt.UTC().Truncate(time.Microsecond)
	specs := []struct {
		key        string
		recordedAt time.Time
		advance    bool
	}{
		{"0", base.Add(1 * time.Hour), false},
		{"1", base.Add(2 * time.Hour), false},
		{"2a", base.Add(3 * time.Hour), false},
		{"2b", base.Add(3 * time.Hour), true},
		{"2c", base.Add(3 * time.Hour), false},
		{"3", base.Add(3*time.Hour + 500*time.Millisecond), false},
		{"4", base.Add(4 * time.Hour), false},
	}
	for _, sp := range specs {
		o, e := s.RecordOccurrence(ctx, tenant, user.ID, cfg.ID, store.Trigger{Kind: "manual", Key: sp.key, Version: 1, At: base.Add(time.Minute)}, sp.recordedAt)
		if e != nil {
			t.Fatal(e)
		}
		if sp.advance {
			for _, ev := range []store.ExecutionEvidence{
				{Event: "begin"},
				{Event: "accepted", Ref: "stable-ref"},
				{Event: "completed", Ref: "stable-ref"},
				{Event: "released", Ref: "stable-ref"},
			} {
				o, e = s.ApplyOccurrenceEvidence(ctx, tenant, user.ID, o.ID, o.Version, ev)
				if e != nil {
					t.Fatal(e)
				}
			}
		}
		expected[o.ID] = o
	}
	first := call(path+"?limit=2", token, 200)
	cursor := first["nextCursor"].(string)
	if cursor == "" {
		t.Fatal("missing next cursor")
	}
	seen := map[string]bool{}
	var prevRecorded time.Time
	previous := ""
	page := first
	for {
		for _, item := range page["items"].([]any) {
			row := item.(map[string]any)
			id := row["id"].(string)
			rec, e := time.Parse(time.RFC3339Nano, row["recordedAt"].(string))
			if e != nil {
				t.Fatal(e)
			}
			if seen[id] || expected[id].ID == "" || !rec.Equal(expected[id].RecordedAt) {
				t.Fatalf("duplicate, foreign or mutated item: %v", row)
			}
			if previous != "" && (prevRecorded.Before(rec) || (prevRecorded.Equal(rec) && previous <= id)) {
				t.Fatalf("unordered item: prev=(%v,%s) curr=(%v,%s)", prevRecorded, previous, rec, id)
			}
			seen[id] = true
			prevRecorded = rec
			previous = id
		}
		next := page["nextCursor"].(string)
		if next == "" {
			break
		}
		page = call(path+"?limit=2&cursor="+url.QueryEscape(next), token, 200)
	}
	if len(seen) != len(specs) || !prevRecorded.Equal(specs[0].recordedAt) {
		t.Fatalf("seen=%d tail=%v", len(seen), prevRecorded)
	}
	call(emptyPath+"?cursor="+cursor, token, 400)
	encodeCursor := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	badCursors := []string{
		encodeCursor(map[string]any{"Tenant": tenant, "User": user.ID, "Schedule": cfg.ID, "Before": previous}),
		encodeCursor(map[string]any{"tenant": tenant, "user": user.ID, "schedule": cfg.ID, "id": previous}),
		encodeCursor(map[string]any{"tenant": tenant, "user": user.ID, "schedule": cfg.ID, "recordedAt": prevRecorded.Format(time.RFC3339Nano)}),
		encodeCursor(map[string]any{"tenant": tenant, "user": user.ID, "schedule": cfg.ID, "recordedAt": prevRecorded.Add(500 * time.Nanosecond).Format(time.RFC3339Nano), "id": previous}),
		encodeCursor(map[string]any{"tenant": tenant, "user": user.ID, "schedule": cfg.ID, "recordedAt": prevRecorded.Format(time.RFC3339Nano), "id": previous, "extra": 1}),
	}
	for _, bc := range badCursors {
		call(path+"?cursor="+url.QueryEscape(bc), token, 400)
	}
	for _, q := range []string{"limit=0", "limit=101", "limit=-1", "limit=x", "limit=1&limit=2", "cursor=bad", "cursor=a&cursor=b", "userId=forged", "limit="} {
		call(path+"?"+q, token, 400)
	}
	// A second active member cannot list or read the owner's history.
	other, e := s.Login(ctx, store.Identity{Source: "test", Subject: "other-history", DisplayName: "Other"}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	call(path, other, 404)
	call(path+"/"+previous, other, 404)
	call("/api/v2/tenants/00000000-0000-0000-0000-000000000001/schedule-configs/"+cfg.ID+"/runs", token, 404)
	call(path+"/00000000-0000-0000-0000-000000000001", token, 404)
	// Service replacement preserves the original cursor and frozen configuration.
	input.Name = "edited"
	input.Version = 1
	if _, e = s.UpdateScheduleConfig(ctx, tenant, user.ID, cfg.ID, input, nil); e != nil {
		t.Fatal(e)
	}
	srv.Close()
	srv = httptest.NewServer(server.New(s, server.Options{}))
	replay := call(path+"?limit=2&cursor="+url.QueryEscape(cursor), token, 200)
	if len(replay["items"].([]any)) != 2 {
		t.Fatal(replay)
	}
	detail := call(path+"/"+previous, token, 200)
	if detail["snapshot"].(map[string]any)["name"] != "history" {
		t.Fatal("history rewritten by edit")
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	call(path, token, 401)
}

func eventsLast(events []string) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1]
}
