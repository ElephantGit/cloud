//go:build linux

package demo

import (
	"slices"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestDashboardSeedAndTaskSessionShareAgentFilter(t *testing.T) {
	for _, scenes := range [][]string{{"lead"}, {"newcomer", "lead", "ops"}} {
		t.Run(scenes[0], func(t *testing.T) {
			s := openSeeder(t)
			seed(t, s, scenes...)
			ctx := t.Context()
			scope, err := s.Scope(ctx, leadEmail)
			if err != nil {
				t.Fatal(err)
			}
			items, _, err := s.Store.DashboardSessions(ctx, scope, store.DashboardFilter{})
			if err != nil || len(items) != 1 {
				t.Fatalf("seeded sessions = %v, error = %v", items, err)
			}
			seeded := items[0]
			if seeded.Agent != "official/ora-space.opencode" {
				t.Fatalf("seed Agent ID = %q, want the canonical runtime ID", seeded.Agent)
			}
			// Create the same persisted task/session relationship as a live configured execution.
			// The browser acceptance test separately exercises the real Node/Agent path.
			task, err := s.Store.CreateTask(ctx, scope.TenantID, scope.UserID, store.TaskInput{ProjectID: seeded.ProjectID, Title: "现场格式会话"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.Store.CreateTaskRun(ctx, store.NewTaskRun{TaskID: task.ID, TenantID: scope.TenantID, StartedBy: scope.UserID, StartKey: "live", ExecutorID: "opencode:configured", ExecutorName: "OpenCode"})
			if err != nil {
				t.Fatal(err)
			}
			ref := execution.Ref{SessionID: "live-dashboard", TurnID: "live-dashboard-turn"}
			model := execution.Model{ID: demoModelID}
			live := execution.Session{ID: ref.SessionID, Target: &execution.Target{ProjectID: seeded.ProjectID}, Executor: &execution.Executor{ID: "opencode:configured", AgentID: "official/ora-space.opencode", Model: model}, Model: model}
			turn := execution.Turn{Ref: ref, Caller: execution.Caller{Kind: execution.CallerTask, ID: run.ID, AttemptID: "live"}, Prompt: execution.TextPrompt("synthetic")}
			if err := s.Store.CreateExecutionSession(ctx, scope, live, turn); err != nil {
				t.Fatal(err)
			}
			if err := s.Store.RecordTaskTurn(ctx, run.ID, ref.SessionID, ref.TurnID); err != nil {
				t.Fatal(err)
			}
			items, cursor, err := s.Store.DashboardSessions(ctx, scope, store.DashboardFilter{Project: seeded.ProjectID, Agent: "official/ora-space.opencode"})
			if err != nil || len(items) != 2 || cursor != "" {
				t.Fatalf("canonical filter: items=%v, cursor=%q, error=%v", items, cursor, err)
			}
			ids := []string{items[0].ID, items[1].ID}
			if !slices.Contains(ids, seeded.ID) || !slices.Contains(ids, ref.SessionID) {
				t.Fatalf("canonical filter lost a source: %v", ids)
			}
			for _, agent := range []string{"opencode", "OpenCode", "official/ora-space.claude"} {
				items, _, err := s.Store.DashboardSessions(ctx, scope, store.DashboardFilter{Agent: agent})
				if err != nil || len(items) != 0 {
					t.Fatalf("nonmatching Agent %q: items=%v, error=%v", agent, items, err)
				}
			}
		})
	}
}
