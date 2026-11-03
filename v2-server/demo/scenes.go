//go:build linux

package demo

import "context"

// scene is one recording's seed. It uses only the accounts in users/<name>.json and its own project,
// so `task v2:demo` can run all three into one tenant in any order.
type scene struct {
	name string
	seed func(context.Context, *Seeder) error
}

// sceneSeeds lists the scenes in the order the combined command seeds them.
var sceneSeeds = []scene{
	{name: "newcomer", seed: seedNewcomerScene},
	{name: "ops", seed: seedOpsScene},
}

// seedNewcomerScene gives 林新 the 「团队任务清单」 project and the 「需求开发」 workflow.
func seedNewcomerScene(ctx context.Context, s *Seeder) error {
	scope, err := s.Scope(ctx, "lin.xin@demo.ora-space.dev")
	if err != nil {
		return err
	}
	project, err := s.Project(ctx, scope, "团队任务清单", "newcomer")
	if err != nil {
		return err
	}
	_, err = SeedNewcomer(ctx, s.WorkflowSeed(scope, project))
	return err
}

// seedOpsScene gives 许运 the 「巡检演示服务」 project, the 「每日巡检」 workflow, its 02:00 schedule and
// the past nights' history.
func seedOpsScene(ctx context.Context, s *Seeder) error {
	scope, err := s.Scope(ctx, "xu.yun@demo.ora-space.dev")
	if err != nil {
		return err
	}
	project, err := s.Project(ctx, scope, "巡检演示服务", "ops")
	if err != nil {
		return err
	}
	_, err = SeedOps(ctx, s.WorkflowSeed(scope, project))
	return err
}
