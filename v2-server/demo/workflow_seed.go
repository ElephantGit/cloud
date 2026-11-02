// Package demo supplies the composable workflow data for the three recording scenes.
// It never resets databases, starts servers, or selects the first account in a preset.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
)

// WorkflowSeed receives the scene's prepared user/project from the environment framework.
// Pool is needed only for historical rows, and must address the same database as Store.
type WorkflowSeed struct {
	Store     *store.Store
	Pool      *pgxpool.Pool
	Scope     execution.Scope
	ProjectID string
	AgentCLI  string
	ModelID   string
	Now       time.Time
}

// SeededWorkflow identifies the definition and the published version usable by another scene.
type SeededWorkflow struct {
	Workflow store.Workflow
	Snapshot store.WorkflowSnapshot
}

var projectUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type demoNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Position map[string]int `json:"position"`
	Data     map[string]any `json:"data"`
}

func chainGraph(seed WorkflowSeed, steps ...demoNode) (string, error) {
	if strings.TrimSpace(seed.AgentCLI) == "" || strings.TrimSpace(seed.ModelID) == "" || strings.HasSuffix(seed.ModelID, "fixture") {
		return "", fmt.Errorf("demo workflows require an explicit real Agent and model")
	}
	edges := make([]map[string]string, 0, len(steps)-1)
	for i := range steps {
		steps[i].Type = "workflow"
		steps[i].Position = map[string]int{"x": i * 340, "y": 100}
		steps[i].Data["description"] = ""
		if config, ok := steps[i].Data["agentConfig"].(map[string]any); ok {
			config["executor"] = map[string]string{"agentCli": seed.AgentCLI, "modelId": seed.ModelID}
			config["schemaVersion"] = 3
			config["roleId"] = ""
			config["skills"] = []any{}
			config["mcps"] = []any{}
			config["interactive"] = false
			config["retry"] = map[string]any{"enabled": false, "maxRetries": 0, "initialDelaySeconds": 0}
		}
		if i > 0 {
			edges = append(edges, map[string]string{"id": steps[i-1].ID + "-" + steps[i].ID, "type": "workflow", "source": steps[i-1].ID, "target": steps[i].ID})
		}
	}
	raw, err := json.Marshal(map[string]any{"nodes": steps, "edges": edges, "viewport": map[string]any{"x": 30, "y": 100, "zoom": 0.65}})
	if err != nil {
		return "", err
	}
	graph := string(raw)
	if err := workflow.ValidateWorkflow(graph); err != nil {
		return "", fmt.Errorf("validate demo graph: %w", err)
	}
	return graph, nil
}

func agentStep(id, title, prompt string) demoNode {
	return demoNode{ID: id, Data: map[string]any{"kind": "agent", "title": title, "agentConfig": map[string]any{"prompt": prompt}}}
}

func outputStep(name, source string) demoNode {
	return demoNode{ID: "output", Data: map[string]any{"kind": "output", "title": "输出", "outputs": []any{map[string]any{"name": name, "variableSelector": []string{source, "output"}}}}}
}

func seedWorkflow(ctx context.Context, seed WorkflowSeed, name, graph string) (SeededWorkflow, error) {
	if seed.Store == nil {
		return SeededWorkflow{}, fmt.Errorf("demo workflow requires a store")
	}
	if err := seed.Store.CheckExecutionScope(ctx, seed.Scope); err != nil {
		return SeededWorkflow{}, err
	}
	project, err := seed.Store.GetProject(ctx, seed.Scope, seed.ProjectID)
	if err != nil {
		return SeededWorkflow{}, err
	}
	// Node decodes snapshot refs as a typed UUID binding, although the temporary store uses text IDs.
	if !projectUUID.MatchString(project.ID) || project.SnapshotRef != projectrepos.SnapshotRef(project.ID, project.Commit) {
		return SeededWorkflow{}, fmt.Errorf("demo project requires a UUID identity and its prepared snapshot ref")
	}
	w, _, err := seed.Store.CreateWorkflow(ctx, seed.Scope.TenantID, name, graph)
	if err != nil {
		return SeededWorkflow{}, fmt.Errorf("create %s: %w", name, err)
	}
	snapshot, err := seed.Store.PublishWorkflow(ctx, seed.Scope.TenantID, w.ID, "v1", workflow.ValidateWorkflow)
	if err != nil {
		return SeededWorkflow{}, fmt.Errorf("publish %s: %w", name, err)
	}
	w.PublishedSnapshotID = &snapshot.ID
	w.UpdatedAt = snapshot.CreatedAt
	return SeededWorkflow{Workflow: w, Snapshot: snapshot}, nil
}
