// Fixture retained from cloud/internal/core/workflow_iteration_scheduler_db_test.go.
package workflow

func iterationRunGraph(strategy string, maxIterations int, selectors ...[]any) Object {
	start := wfNode("s", "start", "开始")
	start["data"].(Object)["inputVariables"] = []any{Object{"name": "items", "valueType": "array"}}
	iteration := wfNode("it", "iteration", "迭代")
	iteration["data"].(Object)["iterationConfig"] = Object{
		"iteratorSelector": []any{"s", "items"},
		"collectSelector":  []any{"agg", "output"},
		"errorStrategy":    strategy,
		"maxIterations":    float64(maxIterations),
	}
	member := wfNode("agg", "aggregator", "聚合")
	member["data"].(Object)["containerId"] = "it"
	choices := make([]any, 0, len(selectors))
	for _, selector := range selectors {
		choices = append(choices, selector)
	}
	member["data"].(Object)["aggregatorConfig"] = Object{"variables": choices}
	return wfGraph(
		[]Object{start, iteration, member, wfNode("o", "output", "输出")},
		[][3]string{{"s", "it", ""}, {"it", "o", ""}},
	)
}
