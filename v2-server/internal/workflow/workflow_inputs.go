// Adapted from cloud/internal/core/workflow_collaboration.go at 0d3299d5.
package workflow

import "strings"

func workflowStartNode(graph Object) Object {
	nodes, _ := graph["nodes"].([]any)
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		data, ok := node["data"].(map[string]any)
		if !ok || data["kind"] != "start" || workflowContainerID(Object(node)) != "" {
			continue
		}
		return Object(data)
	}
	return nil
}

func workflowStartVariables(graph Object) []Object {
	data := workflowStartNode(graph)
	if data == nil {
		return nil
	}
	declared, _ := data["inputVariables"].([]any)
	out := []Object{}
	for _, item := range declared {
		if variable, ok := item.(map[string]any); ok {
			out = append(out, Object(variable))
		}
	}
	return out
}

func workflowStartPrompt(graph Object) string {
	data := workflowStartNode(graph)
	if data == nil {
		return ""
	}
	prompt, _ := data["input"].(string)
	return strings.TrimSpace(prompt)
}
