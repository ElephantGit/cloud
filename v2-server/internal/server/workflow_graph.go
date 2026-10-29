package server

import "encoding/json"

// graphProblems is the asset stage's structural gate, not an execution validator. Node settings,
// scopes and runtime admission must be validated by the runner before any snapshot is executable.
func graphProblems(raw json.RawMessage) []map[string]string {
	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Data struct {
				Kind string `json:"kind"`
			} `json:"data"`
		} `json:"nodes"`
		Edges []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"edges"`
	}
	if json.Unmarshal(raw, &graph) != nil || graph.Nodes == nil || graph.Edges == nil {
		return []map[string]string{{"code": "graph_shape_invalid", "field": "nodes/edges"}}
	}
	problems := []map[string]string{}
	ids := map[string]bool{}
	starts := 0
	for _, node := range graph.Nodes {
		if node.ID == "" || ids[node.ID] {
			problems = append(problems, map[string]string{"code": "node_id_invalid", "nodeId": node.ID})
		}
		ids[node.ID] = true
		if node.Type != "workflow" {
			problems = append(problems, map[string]string{"code": "node_type_invalid", "nodeId": node.ID})
		}
		switch node.Data.Kind {
		case "start":
			starts++
		case "agent", "condition", "aggregator", "loop", "loopExit", "iteration", "output":
		default:
			problems = append(problems, map[string]string{"code": "node_kind_unsupported", "nodeId": node.ID, "kind": node.Data.Kind})
		}
	}
	if starts != 1 {
		problems = append(problems, map[string]string{"code": "start_count_invalid", "field": "nodes"})
	}
	edges := map[string]bool{}
	for _, edge := range graph.Edges {
		if edge.ID == "" || edges[edge.ID] || !ids[edge.Source] || !ids[edge.Target] || edge.Source == edge.Target {
			problems = append(problems, map[string]string{"code": "edge_invalid", "field": edge.ID})
		}
		edges[edge.ID] = true
	}
	return problems
}
