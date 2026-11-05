package demo

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

func TestGraphs(t *testing.T) {
	for name, build := range map[string]func(WorkflowSeed) (string, error){"newcomer": NewcomerGraph, "lead": DecompositionGraph, "ops": OpsGraph} {
		t.Run(name, func(t *testing.T) {
			graph, err := build(WorkflowSeed{AgentCLI: "official/ora-space.opencode", ModelID: "configured"})
			if err != nil {
				t.Fatal(err)
			}
			analysis, err := workflow.AnalyzeWorkflow(graph)
			if err != nil || len(analysis.UnusedNodeIDs) != 0 {
				t.Fatalf("graph has unused nodes: %+v %v", analysis, err)
			}
			if _, err := build(WorkflowSeed{AgentCLI: "official/ora-space.opencode", ModelID: "fixture"}); err == nil {
				t.Fatal("fixture model admitted into real demo")
			}
		})
	}
	graph, err := DecompositionGraph(WorkflowSeed{AgentCLI: "official/ora-space.opencode", ModelID: "configured"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Nodes []demoNode `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(graph), &doc); err != nil {
		t.Fatal(err)
	}
	schema := doc.Nodes[1].Data["agentConfig"].(map[string]any)["outputContract"].(map[string]any)["schema"].(map[string]any)
	if _, err := workflow.ParseStructuredOutput(`{"tasks":[{"title":"增加筛选","description":"按负责人筛选；覆盖空列表测试"}]}`, schema); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.ParseStructuredOutput(`{"tasks":[{"title":"缺少验收条件"}]}`, schema); err == nil {
		t.Fatal("malformed child task accepted")
	}
}

func TestAllWorkflowOutputsResolveFromRuntimePool(t *testing.T) {
	tasks := []any{map[string]any{"title": "增加筛选", "description": "按负责人筛选；覆盖空列表测试"}}
	for _, tc := range []struct {
		name     string
		build    func(WorkflowSeed) (string, error)
		variable string
		value    any
		want     map[string]any
	}{
		{"newcomer", NewcomerGraph, "verify.output", "交付说明：测试通过", map[string]any{"delivery": "交付说明：测试通过"}},
		{"lead", DecompositionGraph, "decompose.structured_output", map[string]any{"tasks": tasks}, map[string]any{"tasks": tasks}},
		{"ops", legacyOpsGraph, "report.output", "巡检报告：服务正常", map[string]any{"report": "巡检报告：服务正常"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := tc.build(WorkflowSeed{AgentCLI: "official/ora-space.opencode", ModelID: "configured"})
			if err != nil {
				t.Fatal(err)
			}
			graph, err := workflow.ParseGraph(source)
			if err != nil {
				t.Fatal(err)
			}
			pool := workflow.NewPool(graph)
			writer, _, _ := strings.Cut(tc.variable, ".")
			if err := pool.Set(tc.variable, writer, tc.value); err != nil {
				t.Fatal(err)
			}
			output, ok := graph.Node("output")
			if !ok {
				t.Fatal("workflow has no output node")
			}
			got := map[string]any{}
			for _, binding := range output.OutputBindings() {
				value, assigned, err := pool.Resolve(binding.Selector)
				if err != nil || !assigned {
					t.Fatalf("output %s selector %v cannot resolve: assigned=%v err=%v", binding.Name, binding.Selector, assigned, err)
				}
				got[binding.Name] = value
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("workflow result = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestSeedsComposeAndHistoryIsOwnedByOps(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "demo_test_" + strings.ToLower(rand.Text())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	s, err := store.OpenConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	pool, err := pgxpool.NewWithConfig(ctx, config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	seeds := map[string]WorkflowSeed{}
	for index, name := range []string{"ops", "lead", "newcomer"} {
		user, err := s.UpsertUser(ctx, store.PresetUser{Email: name + "@demo.test", DisplayName: name, Role: store.RoleOwner})
		if err != nil {
			t.Fatal(err)
		}
		tenants, err := s.Tenants(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		scope := execution.Scope{TenantID: tenants[0].ID, UserID: user}
		projectID := fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
		commit := strings.Repeat("a", 40)
		project, err := s.CreateProject(ctx, scope, store.Project{ID: projectID, Name: name, RepositoryURL: "local-demo", Commit: commit, SnapshotRef: projectrepos.SnapshotRef(projectID, commit), MirrorPath: "/prepared/" + name})
		if err != nil {
			t.Fatal(err)
		}
		seeds[name] = WorkflowSeed{Store: s, Pool: pool, Scope: scope, ProjectID: project.ID, AgentCLI: "official/ora-space.opencode", ModelID: "configured", Now: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	}
	// C first, then B and A: no scene relies on another's initialization order.
	ops, err := SeedOps(ctx, seeds["ops"])
	if err != nil {
		t.Fatal(err)
	}
	if ops.Workflow.PublishedSnapshotID == nil || *ops.Workflow.PublishedSnapshotID != ops.Snapshot.ID {
		t.Fatal("seed returned stale publication metadata")
	}
	if _, err := SeedDecomposition(ctx, seeds["lead"]); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedNewcomer(ctx, seeds["newcomer"]); err != nil {
		t.Fatal(err)
	}
	bad := seeds["newcomer"]
	bad.ProjectID = "not-a-uuid"
	commit := strings.Repeat("a", 40)
	if _, err := s.CreateProject(ctx, bad.Scope, store.Project{ID: bad.ProjectID, Name: "invalid fixture", RepositoryURL: "local-demo", Commit: commit, SnapshotRef: projectrepos.SnapshotRef(bad.ProjectID, commit), MirrorPath: "/prepared/invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedNewcomer(ctx, bad); err == nil || !strings.Contains(err.Error(), "UUID") {
		t.Fatalf("invalid Node snapshot identity was not rejected before publishing: %v", err)
	}
	tenant := seeds["ops"].Scope.TenantID
	workflows, err := s.Workflows(ctx, tenant)
	if err != nil || len(workflows) != 3 {
		t.Fatalf("composed workflows: %v %v", workflows, err)
	}
	configs, err := s.ScheduleConfigs(ctx, tenant, seeds["ops"].Scope.UserID)
	if err != nil || len(configs) != 1 {
		t.Fatalf("ops schedule: %v %v", configs, err)
	}
	history, err := s.ScheduleHistory(ctx, tenant, seeds["ops"].Scope.UserID, configs[0].ID, store.OccurrenceCursor{}, 20)
	if err != nil || len(history) != 3 {
		t.Fatalf("ops history: %v %v", history, err)
	}
	for i, item := range history {
		want := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC).AddDate(0, 0, -i)
		// 02:00 Shanghai of Nov 1 / Oct 31 / Oct 30 is 18:00 UTC on the previous day.

		if !item.ScheduledAt.Equal(want) {
			t.Fatal("history date changed")
		}
		runID, ok := scheduler.WorkflowRunOf(item.ExecutionRef)
		if !ok {
			t.Fatal("missing run reference")
		}
		run, err := s.WorkflowRun(ctx, tenant, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.ProjectID != seeds["ops"].ProjectID {
			t.Fatal("wrong project")
		}
		if !strings.Contains(strings.ReplaceAll(string(run.RuntimePayload), " ", ""), `"demoSeed":true`) {
			t.Fatal("missing seed marker")
		}
		if i == 0 {
			if run.Status != "awaitingInput" || item.Released || !item.WaitingForInput {
				t.Fatalf("waiting history: %+v %+v", run, item)
			}
			form, err := s.WorkflowForm(ctx, tenant, seeds["ops"].Scope.UserID, runID)
			if err != nil || form.Status != "open" {
				t.Fatalf("waiting form: %+v %v", form, err)
			}
		} else {
			if run.Status != "succeeded" || !item.Released {
				t.Fatalf("finished history: %+v", run)
			}
			if i == 1 {
				form, err := s.WorkflowForm(ctx, tenant, seeds["ops"].Scope.UserID, runID)
				if err != nil || string(form.Results) == "[]" {
					t.Fatalf("submitted history: %+v %v", form, err)
				}
			}
		}
		if (i < 2) != (run.SnapshotID == ops.Snapshot.ID) {
			t.Fatal("history snapshot mismatch")
		}

	}
	for _, name := range []string{"lead", "newcomer"} {
		other, err := s.ScheduleConfigs(ctx, tenant, seeds[name].Scope.UserID)
		if err != nil || len(other) != 0 {
			t.Fatalf("ops schedule leaked to %s: %v %v", name, other, err)
		}
		if _, err := s.ScheduleHistory(ctx, tenant, seeds[name].Scope.UserID, configs[0].ID, store.OccurrenceCursor{}, 20); err == nil {
			t.Fatal("foreign history readable")
		}
	}
}
