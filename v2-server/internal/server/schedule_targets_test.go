package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type targetResolver func(context.Context, string, string, server.ScheduleTarget) (string, error)

func (f targetResolver) ResolveScheduleTarget(c context.Context, t, u string, v server.ScheduleTarget) (string, error) {
	return f(c, t, u, v)
}

func TestReferencedSchedulesPersistOnlyAfterResourceAuthorization(t *testing.T) {
	for _, kind := range []string{"workflow", "task"} {
		t.Run(kind, func(t *testing.T) {
			s := openStore(t)
			ctx := context.Background()
			token, err := s.Login(ctx, store.Identity{Source: "test", Subject: kind, DisplayName: kind}, time.Hour)
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
			project := "11111111-1111-4111-8111-111111111111"
			resource := "22222222-2222-4222-8222-222222222222"
			snapshot := "33333333-3333-4333-8333-333333333333"
			target := map[string]any{"kind": "task", "sourceTaskId": resource, "supplementalInstruction": "检查最新内容"}
			if kind == "workflow" {
				target = map[string]any{"kind": kind, "workflowId": resource, "snapshot": snapshot, "inputs": map[string]string{"branch": "main"}}
			}
			in := map[string]any{"name": "持久引用", "projectId": project, "target": target, "rule": map[string]any{"kind": "daily", "time": "09:00", "days": []int{1}, "hours": 6, "timezone": "Asia/Shanghai"}}
			call := func(h http.Handler, method, path string, input any) *httptest.ResponseRecorder {
				b, _ := json.Marshal(input)
				req := httptest.NewRequest(method, path, bytes.NewReader(b))
				req.AddCookie(&http.Cookie{Name: server.SessionCookie, Value: token})
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				return w
			}
			path := "/api/v2/tenants/" + tenant + "/schedule-configs"
			unavailable := server.New(s, server.Options{})
			if w := call(unavailable, "POST", path, in); w.Code != 503 {
				t.Fatalf("unwired: %d %s", w.Code, w.Body)
			}
			authorized := true
			resolver := targetResolver(func(_ context.Context, tid, uid string, target server.ScheduleTarget) (string, error) {
				if tid != tenant || uid != user.ID {
					t.Fatal("caller scope lost")
				}
				if !authorized {
					return "", store.ErrNotFound
				}
				if target.Kind != kind {
					t.Fatal("target kind lost")
				}
				if kind == "workflow" && (target.Snapshot != snapshot || target.Inputs["branch"] != "main") {
					t.Fatal("snapshot inputs lost")
				}
				return project, nil
			})
			h := server.New(s, server.Options{ScheduleTargets: resolver})
			w := call(h, "POST", path, in)
			if w.Code != 201 {
				t.Fatalf("create: %d %s", w.Code, w.Body)
			}
			var saved store.ScheduleConfig
			if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
				t.Fatal(err)
			}
			// A new handler reads the same PostgreSQL configuration without needing the resolver.
			w = call(unavailable, "GET", path, nil)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			var list struct {
				Items []store.ScheduleConfig `json:"items"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if len(list.Items) != 1 || list.Items[0].ID != saved.ID || !bytes.Equal(list.Items[0].Target, saved.Target) {
				t.Fatalf("lost persisted target: %s", w.Body)
			}
			authorized = false
			in["version"] = 1
			in["name"] = "禁止覆盖"
			if w = call(h, "PUT", path+"/"+saved.ID, in); w.Code != 404 {
				t.Fatalf("revoked resource: %d", w.Code)
			}
			configs, err := s.ScheduleConfigs(ctx, tenant, user.ID)
			if err != nil || configs[0].Name != "持久引用" {
				t.Fatal("unauthorized overwrite", err)
			}
		})
	}
}
