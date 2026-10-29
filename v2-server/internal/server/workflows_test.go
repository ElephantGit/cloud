package server_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func workflowClient(t *testing.T, s *store.Store) *client {
	t.Helper()
	srv := httptest.NewServer(server.New(s, server.Options{SessionTTL: time.Hour,
		DemoUser: store.Identity{Source: "dev", Subject: "workflow-user", DisplayName: "工作流用户"}}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

func loginWorkflow(t *testing.T, c *client) string {
	t.Helper()
	if status, _ := c.do("POST", "/auth/login", map[string]string{}); status != 200 {
		t.Fatal(status)
	}
	status, body := c.do("GET", "/api/v1/me/tenants", nil)
	if status != 200 {
		t.Fatal(status, body)
	}
	return "/api/v1/tenants/" + body["items"].([]any)[0].(map[string]any)["id"].(string) + "/workflows"
}

func draftGraph(title string) map[string]any {
	return map[string]any{
		"nodes": []any{map[string]any{"id": "start", "type": "workflow", "position": map[string]any{"x": 0, "y": 0},
			"data": map[string]any{"kind": "start", "title": title, "inputVariables": []any{}}}},
		"edges": []any{}, "viewport": map[string]any{"x": 0, "y": 0, "zoom": 1},
		"annotations": map[string]any{"futureField": []any{false, 0, nil, ""}},
	}
}

func createDraft(t *testing.T, c *client, path, name string) map[string]any {
	t.Helper()
	status, body := c.do("POST", path, map[string]any{"name": name, "graph": draftGraph("原始输入")})
	if status != 200 {
		t.Fatal(status, body)
	}
	return body["resource"].(map[string]any)
}

func TestWorkflowAssetLifecycleAndRestart(t *testing.T) {
	s, config := openStoreConfig(t)
	c := workflowClient(t, s)
	root := loginWorkflow(t, c)
	wf := createDraft(t, c, root, "示例工作流")
	path := root + "/" + wf["id"].(string)
	status, body := c.do("POST", path+"/publish", map[string]any{})
	if status != 200 {
		t.Fatal(status, body)
	}
	snapshot := body["resource"].(map[string]any)
	if snapshot["version"] != float64(1) || snapshot["name"] != wf["name"] {
		t.Fatal(snapshot)
	}
	status, updated := c.do("PUT", path, map[string]any{"name": "修改后", "description": "说明", "version": wf["version"], "graph": draftGraph("新版输入")})
	if status != 200 || updated["version"] != float64(2) {
		t.Fatal(status, updated)
	}
	status, read := c.do("GET", path+"/snapshots/"+snapshot["id"].(string), nil)
	if status != 200 || !reflect.DeepEqual(read["graph"], snapshot["graph"]) {
		t.Fatal("snapshot mutated", status, read)
	}
	if status, body := c.do("PUT", path, map[string]any{"name": "覆盖", "version": 1}); status != 409 || body["code"] != "version_conflict" {
		t.Fatal(status, body)
	}

	// Close the pool and open a new server against the same schema, not an in-memory substitute.
	s.Close()
	reopened, err := store.OpenConfig(context.Background(), config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	c2 := workflowClient(t, reopened)
	loginWorkflow(t, c2)
	if status, read := c2.do("GET", path, nil); status != 200 || !reflect.DeepEqual(read, updated) {
		t.Fatal("draft lost on restart", status, read)
	}
	if status, read := c2.do("GET", path+"/snapshots/"+snapshot["id"].(string), nil); status != 200 || !reflect.DeepEqual(read, snapshot) {
		t.Fatal("snapshot lost on restart", status, read)
	}

	if status, body := c2.do("POST", root, map[string]any{"name": "修改后"}); status != 409 || body["code"] != "workflow_conflict" {
		t.Fatal(status, body)
	}
	if status, body := c2.do("DELETE", path, map[string]any{"version": 1}); status != 409 {
		t.Fatal(status, body)
	}
	if status, body := c2.do("DELETE", path, map[string]any{"version": 2}); status != 200 || body["deletedAt"] == nil {
		t.Fatal(status, body)
	}
	for _, suffix := range []string{"", "/snapshots", "/snapshots/" + snapshot["id"].(string)} {
		if status, body := c2.do("GET", path+suffix, nil); status != 404 {
			t.Fatal(status, body)
		}
	}
	createDraft(t, c2, root, "修改后")
}

func TestWorkflowAuthorizationValidationAndPagination(t *testing.T) {
	s, config := openStoreConfig(t)
	c := workflowClient(t, s)
	root := loginWorkflow(t, c)
	first := createDraft(t, c, root, "第一个")
	second := createDraft(t, c, root, "第二个")
	path := root + "/" + first["id"].(string)
	status, page := c.do("GET", root+"?limit=1", nil)
	if status != 200 || len(page["items"].([]any)) != 1 || page["nextCursor"] == "" {
		t.Fatal(status, page)
	}
	status, last := c.do("GET", root+"?limit=1&after="+page["nextCursor"].(string), nil)
	if status != 200 || len(last["items"].([]any)) != 1 || last["nextCursor"] != "" {
		t.Fatal(status, last)
	}
	if page["items"].([]any)[0].(map[string]any)["id"] == last["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("duplicate cursor row")
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?limit=abc", "?after=bad"} {
		if status, body := c.do("GET", root+q, nil); status != 400 {
			t.Fatal(status, body)
		}
	}
	_, published := c.do("POST", path+"/publish", map[string]any{})
	sid := published["resource"].(map[string]any)["id"].(string)
	if status, body := c.do("GET", root+"/"+second["id"].(string)+"/snapshots/"+sid, nil); status != 404 {
		t.Fatal(status, body)
	}
	if status, body := c.do("GET", path+"/snapshots/bad", nil); status != 404 {
		t.Fatal(status, body)
	}
	if status, body := c.do("GET", "/api/v1/tenants/00000000-0000-4000-8000-000000000000/workflows", nil); status != 403 {
		t.Fatal(status, body)
	}

	for _, body := range []any{
		map[string]any{"name": " "}, map[string]any{"name": strings.Repeat("名", 201)},
		map[string]any{"name": "空", "graph": nil}, map[string]any{"name": "数组", "graph": []any{}},
		map[string]any{"name": "越权", "tenantId": "x"}, map[string]any{"name": "类型", "description": 1},
	} {
		if status, out := c.do("POST", root, body); status != 400 {
			t.Fatal(status, out)
		}
	}
	if status, body := c.do("PUT", path, map[string]any{"name": "缺版本"}); status != 428 {
		t.Fatal(status, body)
	}
	// Unfinished graphs are legitimate drafts, but cannot become structural snapshots.
	status, blank := c.do("POST", root, map[string]any{"name": "空白"})
	if status != 200 {
		t.Fatal(status, blank)
	}
	blankPath := root + "/" + blank["resource"].(map[string]any)["id"].(string)
	if status, body := c.do("POST", blankPath+"/publish", map[string]any{}); status != 400 || body["code"] != "workflow_graph_invalid" {
		t.Fatal(status, body)
	}
	req, _ := http.NewRequest("POST", c.base+root, strings.NewReader(`{"name":"first"} {"name":"second"}`))
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal(res.StatusCode)
	}

	// Membership changes are read from PostgreSQL on each request, including snapshot reads.
	admin, err := pgx.ConnectConfig(context.Background(), config.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })
	if _, err = admin.Exec(context.Background(), "DELETE FROM memberships"); err != nil {
		t.Fatal(err)
	}
	if status, body := c.do("GET", path+"/snapshots/"+sid, nil); status != 403 {
		t.Fatal(status, body)
	}
	if status, body := c.do("PUT", path, map[string]any{"name": "不能写", "version": 1}); status != 403 {
		t.Fatal(status, body)
	}
	c.do("POST", "/auth/logout", nil)
	if status, body := c.do("GET", root, nil); status != 401 {
		t.Fatal(status, body)
	}
}

func TestWorkflowConcurrentWritesAndPublishes(t *testing.T) {
	c := newClient(t)
	root := loginWorkflow(t, c)
	wf := createDraft(t, c, root, "并发")
	path := root + "/" + wf["id"].(string)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, _ := c.do("PUT", path, map[string]any{"name": fmt.Sprint("编辑", i), "version": 1})
			statuses <- status
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
	versions := make(chan float64, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := c.do("POST", path+"/publish", map[string]any{})
			if status != 200 {
				t.Error(status, body)
				return
			}
			versions <- body["resource"].(map[string]any)["version"].(float64)
		}()
	}
	wg.Wait()
	close(versions)
	seen := map[float64]bool{}
	for v := range versions {
		seen[v] = true
	}
	for _, v := range []float64{1, 2, 3, 4} {
		if !seen[v] {
			t.Fatal("non-unique snapshot sequence", seen)
		}
	}
	if status, body := c.do("GET", path+"/snapshots?limit=2", nil); status != 200 || len(body["items"].([]any)) != 2 || body["nextCursor"] == "" {
		t.Fatal(status, body)
	}
}

func TestPublishRejectsDraftChangedAfterValidation(t *testing.T) {
	s := openStore(t)
	c := workflowClient(t, s)
	root := loginWorkflow(t, c)
	wf := createDraft(t, c, root, "发布围栏")
	path := root + "/" + wf["id"].(string)
	if status, body := c.do("PUT", path, map[string]any{"name": "仍在编辑", "version": 1, "graph": map[string]any{}}); status != 200 {
		t.Fatal(status, body)
	}
	_, err := s.PublishWorkflow(context.Background(), wf["tenantId"].(string), wf["id"].(string), "", 1)
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("stale validated graph published: %v", err)
	}
	if status, body := c.do("GET", path+"/snapshots", nil); status != 200 || len(body["items"].([]any)) != 0 {
		t.Fatal("stale publish left a snapshot", status, body)
	}
}

func TestWorkflowScopeAcrossAuthorizedTenants(t *testing.T) {
	s, config := openStoreConfig(t)
	c := workflowClient(t, s)
	root := loginWorkflow(t, c)
	wf := createDraft(t, c, root, "团队私有")
	path := root + "/" + wf["id"].(string)
	_, published := c.do("POST", path+"/publish", map[string]any{})
	sid := published["resource"].(map[string]any)["id"].(string)
	admin, err := pgx.ConnectConfig(context.Background(), config.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var other string
	if err = admin.QueryRow(context.Background(), "INSERT INTO tenants(name) VALUES('另一个团队') RETURNING id").Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(context.Background(), "INSERT INTO memberships(tenant_id,user_id,role) SELECT $1,id,'member' FROM users", other); err != nil {
		t.Fatal(err)
	}
	foreignRoot := "/api/v1/tenants/" + other + "/workflows"
	if status, body := c.do("GET", foreignRoot, nil); status != 200 || len(body["items"].([]any)) != 0 {
		t.Fatal(status, body)
	}
	for _, suffix := range []string{"", "/snapshots", "/snapshots/" + sid} {
		if status, body := c.do("GET", foreignRoot+"/"+wf["id"].(string)+suffix, nil); status != 404 {
			t.Fatal(status, body)
		}
	}
	if status, body := c.do("PUT", foreignRoot+"/"+wf["id"].(string), map[string]any{"name": "不能覆盖", "version": 1}); status != 404 {
		t.Fatal(status, body)
	}
}
