package demo

import "context"

// DecompositionGraph emits a structured tasks array for B's child-task importer.
func DecompositionGraph(seed WorkflowSeed) (string, error) {
	step := agentStep("decompose", "拆分需求与验收条件", "阅读仓库 README.md，并将下面的大需求拆成 3 到 5 个可独立分配给人或 Agent 的子任务。每个 description 写清具体范围和可验证的验收条件。只输出 JSON 对象 {\"tasks\":[{\"title\":\"…\",\"description\":\"…\"}]}，不要 Markdown 围栏，不要修改代码。\n大需求：{{#start.input#}}")
	step.Data["agentConfig"].(map[string]any)["outputContract"] = map[string]any{
		"type": "structured",
		"schema": map[string]any{"type": "object", "required": []string{"tasks"}, "additionalProperties": false,
			"properties": map[string]any{"tasks": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"title", "description"}, "additionalProperties": false, "properties": map[string]any{"title": map[string]string{"type": "string"}, "description": map[string]string{"type": "string"}}}}}},
	}
	return chainGraph(seed, demoNode{ID: "start", Data: map[string]any{"kind": "start", "title": "开始：团队大需求"}}, step,
		demoNode{ID: "output", Data: map[string]any{"kind": "output", "title": "输出子任务", "outputs": []any{map[string]any{"name": "tasks", "variableSelector": []string{"decompose", "tasks"}}}}})
}

// SeedDecomposition installs B's workflow without assuming A or C ran first.
func SeedDecomposition(ctx context.Context, seed WorkflowSeed) (SeededWorkflow, error) {
	graph, err := DecompositionGraph(seed)
	if err != nil {
		return SeededWorkflow{}, err
	}
	return seedWorkflow(ctx, seed, "需求拆分", graph)
}
