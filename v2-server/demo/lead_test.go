//go:build linux

package demo

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// seedSummary is what a scene's viewers see, independent of generated ids and clocks.
type seedSummary struct {
	projects, workflows, squads, tasks []string
}

func openSeeder(t *testing.T) *Seeder {
	t.Helper()
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
	schema := "demo_env_" + strings.ToLower(rand.Text())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
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
	projects, err := projectrepos.New(s, projectrepos.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return &Seeder{Store: s, Pool: pool, Projects: projects, Now: time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)}
}

// seed presets the scenes' accounts, as v2-server does at startup, then seeds them in order.
func seed(t *testing.T, s *Seeder, scenes ...string) seedSummary {
	t.Helper()
	ctx := context.Background()
	users, err := SceneUsers(scenes)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SeedUsers(ctx, users); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(ctx, scenes); err != nil {
		t.Fatal(err)
	}
	scope, err := s.Scope(ctx, users.Users[0].Email)
	if err != nil {
		t.Fatal(err)
	}
	var out seedSummary
	projects, err := s.Store.ListProjects(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		// Node clones the snapshot ref of a UUID project; the fixture must be committed under it.
		if !projectUUID.MatchString(p.ID) || p.SnapshotRef != projectrepos.SnapshotRef(p.ID, p.Commit) {
			t.Fatalf("project %s lacks its Node identity: %+v", p.Name, p)
		}
		if out, err := exec.Command("git", "--git-dir", p.MirrorPath, "cat-file", "-e", p.SnapshotRef+":README.md").CombinedOutput(); err != nil {
			t.Fatalf("project %s snapshot has no README.md: %v %s", p.Name, err, out)
		}
		out.projects = append(out.projects, p.Name)
	}
	workflows, err := s.Store.Workflows(ctx, scope.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range workflows {
		out.workflows = append(out.workflows, w.Name)
	}
	squads, err := s.Store.Squads(ctx, scope.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sq := range squads {
		detail, err := s.Store.Squad(ctx, scope.TenantID, sq.ID)
		if err != nil {
			t.Fatal(err)
		}
		entry := sq.Name + ":"
		for _, m := range detail.Members {
			entry += " " + m.DisplayName + "/" + m.Type + "/" + m.Role
		}
		out.squads = append(out.squads, entry)
	}
	tasks, err := s.Store.Tasks(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	for _, task := range tasks {
		out.tasks = append(out.tasks, names[task.ProjectID]+" | "+task.Title+" | "+task.Assignee.DisplayName+" | "+task.Status+" | "+task.CreatedByName)
	}
	for _, list := range [][]string{out.projects, out.workflows, out.squads, out.tasks} {
		slices.Sort(list)
	}
	return out
}

func TestScenesComposeInOneTenant(t *testing.T) {
	lead := seed(t, openSeeder(t), "lead")
	if !slices.Equal(lead.projects, []string{"协作看板服务"}) || !slices.Equal(lead.workflows, []string{"版本质量扫描", "需求拆分"}) {
		t.Fatalf("lead alone: %+v", lead)
	}
	if want := []string{"看板小队: 陈默/human/ 苏芮/human/前端 周衡/human/后端 OpenCode/agent/编码与测试 Claude Code/agent/代码评审"}; !slices.Equal(lead.squads, want) {
		t.Fatalf("squad: %q", lead.squads)
	}
	if want := []string{
		"协作看板服务 | 取消任务时保留原状态记录 | 周衡 | review | 陈默",
		"协作看板服务 | 状态推进只能按顺序 | 周衡 | done | 陈默",
		"协作看板服务 | 看板列显示任务数 | 苏芮 | running | 陈默",
		"协作看板服务 | 看板按状态分列 | 苏芮 | done | 陈默",
		"协作看板服务 | 预置分析样例 · 看板变更检查（非现场执行） | OpenCode | review | 陈默",
	}; !slices.Equal(lead.tasks, want) {
		t.Fatalf("board history: %q", lead.tasks)
	}

	// The combined environment seeds all three into one tenant in reverse order; B's board, squad
	// and the workflow names are exactly what B alone shows, so A and C never reach B's screens.
	all := seed(t, openSeeder(t), "ops", "lead", "newcomer")
	bTasks := []string{}
	for _, task := range all.tasks {
		if strings.HasPrefix(task, "协作看板服务 | ") {
			bTasks = append(bTasks, task)
		}
	}
	if len(all.tasks) != len(lead.tasks)+2 {
		t.Fatal("missing inspection task examples")
	}
	if !slices.Equal(bTasks, lead.tasks) || !slices.Equal(all.squads, lead.squads) {
		t.Fatalf("other scenes changed B's board or squad: %+v", all)
	}
	if want := []string{"协作看板服务", "团队任务清单", "巡检演示服务"}; !slices.Equal(all.projects, want) {
		t.Fatalf("projects: %q", all.projects)
	}
	if want := []string{"每日巡检", "版本质量扫描", "需求开发", "需求拆分"}; !slices.Equal(all.workflows, want) {
		t.Fatalf("workflows: %q", all.workflows)
	}

	// A rerun reseeds an emptied environment; it must land in the same state.
	if again := seed(t, openSeeder(t), "newcomer", "lead", "ops"); !slices.Equal(again.tasks, all.tasks) ||
		!slices.Equal(again.projects, all.projects) || !slices.Equal(again.workflows, all.workflows) || !slices.Equal(again.squads, all.squads) {
		t.Fatalf("rerun differs: %+v vs %+v", again, all)
	}
}
