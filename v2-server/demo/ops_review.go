package demo

// OpsGraph collects read-only evidence, freezes a review, and creates tasks only
// after the human response. The model never creates tasks or external tickets.
func OpsGraph(seed WorkflowSeed) (string, error) {
	report := agentStep("report", "生成巡检报告与问题列表", "根据巡检证据输出 JSON，包含 reportMarkdown 和 findings。每项问题必须原样保留脚本 findings 的 key，包含 title、description、evidence、severity、module；不创造新的 key，不创建任务、不修改文件。报告说明数据来自演示服务。没有问题时 findings 是空数组。\n证据：{{#inspect.output#}}")
	properties := map[string]any{}
	for _, key := range []string{"key", "title", "description", "evidence", "severity", "module"} {
		properties[key] = map[string]string{"type": "string"}
	}
	report.Data["agentConfig"].(map[string]any)["outputContract"] = map[string]any{"type": "structured", "schema": map[string]any{"type": "object", "required": []string{"reportMarkdown", "findings"}, "additionalProperties": false, "properties": map[string]any{"reportMarkdown": map[string]string{"type": "string"}, "findings": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"key", "title", "description", "evidence", "severity", "module"}, "additionalProperties": false, "properties": properties}}}}}
	return chainGraph(seed,
		demoNode{ID: "start", Data: map[string]any{"kind": "start", "title": "开始：每日巡检"}},
		agentStep("inspect", "检查服务与资源", "只读检查当前演示仓库，读取 README.md、ops/metrics.json、ops/service.log，运行 node scripts/inspect.mjs。完整保留输出中的 findings 和稳定 key。发现异常是巡检结果，不是执行失败；只有采集或命令故障才报告失败。不要修改文件、访问外网或执行修复。\n要求：{{#start.input#}}"),
		report,
		demoNode{ID: "review", Data: map[string]any{"kind": "form", "title": "确认巡检问题", "formConfig": map[string]any{"title": "巡检报告与问题确认", "submitLabel": "确认并生成任务", "reportSelector": []string{"report", "structured_output", "reportMarkdown"}, "itemsSelector": []string{"report", "structured_output", "findings"}, "fields": []any{map[string]any{"name": "notes", "label": "备注", "type": "textarea", "required": false}}}}},
		demoNode{ID: "tasks", Data: map[string]any{"kind": "createTasks", "title": "创建或关联协作任务", "createTasksConfig": map[string]string{"formNodeId": "review"}}},
		demoNode{ID: "output", Data: map[string]any{"kind": "output", "title": "输出处置结果", "outputs": []any{map[string]any{"name": "report", "variableSelector": []string{"report", "structured_output", "reportMarkdown"}}, map[string]any{"name": "tasks", "variableSelector": []string{"tasks", "structured_output", "tasks"}}}}},
	)
}
