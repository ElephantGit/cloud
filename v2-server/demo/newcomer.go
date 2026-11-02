package demo

import "context"

// NewcomerGraph is the real requirement → plan → implementation → verification workflow.
// Each Agent runs in the same run workspace, so implementation and review see the same files.
func NewcomerGraph(seed WorkflowSeed) (string, error) {
	return chainGraph(seed,
		demoNode{ID: "start", Data: map[string]any{"kind": "start", "title": "开始：交给团队工作流"}},
		agentStep("plan", "理解需求与制定计划", "你是团队的开发负责人。先读取仓库 README.md 和现有代码，再把下面的需求整理成简短实现计划与验收清单；本步不改代码。只基于实际仓库，不访问网络。\n需求：{{#start.input#}}"),
		agentStep("implement", "实现需求", "按下面的计划在当前仓库实现需求，补上或更新自动化测试。直接修改文件，沿用现有技术栈，不安装依赖，不访问网络，不执行 git commit 或 push。\n原始需求：{{#start.input#}}\n计划：{{#plan.output#}}\n完成后列出实际修改的文件和关键实现。"),
		agentStep("verify", "运行测试与交付说明", "检查当前工作目录中上一步实际修改的代码，运行 README.md 中的测试命令，若失败请修复并重新测试。不要访问网络、安装依赖、commit 或 push。\n需求：{{#start.input#}}\n实现说明：{{#implement.output#}}\n输出简短中文交付报告：需求完成情况、文件路径与关键代码、实际测试命令与结果、使用示例和仍存在的限制。不得将未运行的测试说成已通过。"),
		outputStep("delivery", "verify"),
	)
}

// SeedNewcomer installs A's workflow after the framework prepares A's own account and project.
func SeedNewcomer(ctx context.Context, seed WorkflowSeed) (SeededWorkflow, error) {
	graph, err := NewcomerGraph(seed)
	if err != nil {
		return SeededWorkflow{}, err
	}
	return seedWorkflow(ctx, seed, "需求开发", graph)
}
