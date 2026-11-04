package workflowhost

import (
	"context"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestWorkflowFreezesSelectedSkillAndClearsItForNextNode(t *testing.T) {
	f := newFixture(t)
	_, err := f.s.CreateSkill(context.Background(), f.scope.TenantID, f.scope.UserID, store.SkillInput{Name: "selected-review", Description: "Review", Content: "UNIQUE_REVIEW_INSTRUCTION"})
	if err != nil {
		t.Fatal(err)
	}
	graph := strings.Replace(twoAgents, `"roleId":"reviewer"`, `"skills":[{"skillId":"selected-review","enabled":true},{"skillId":"missing-disabled","enabled":false}]`, 1)
	runID := f.run(t, graph, str("review"))
	if _, err := f.host.StartRun(context.Background(), f.scope, runID); err != nil {
		t.Fatal(err)
	}
	a := f.exec.awaitStart(t)
	request := f.exec.request(a)
	if request.Skills == nil || len(request.Skills.Documents) != 1 || request.Skills.Documents[0].Content != "UNIQUE_REVIEW_INSTRUCTION" {
		t.Fatalf("not frozen: %+v", request.Skills)
	}
	if !strings.Contains(promptText(t, request.Prompt), "<required_skills>") {
		t.Fatal("missing required skill instructions")
	}
	f.exec.finish(a, completed("reviewed"))
	b := f.exec.awaitStart(t)
	next := f.exec.request(b)
	if next.Skills == nil || len(next.Skills.Documents) != 0 || len(next.Skills.Plugins) != 0 {
		t.Fatalf("next node inherited skills: %+v", next.Skills)
	}
	f.exec.finish(b, completed("done"))
	f.await(t, runID, terminal)
}

func TestWorkflowRejectsMissingSkillAndUnsupportedMCPBeforeExecution(t *testing.T) {
	for _, config := range []string{`"skills":[{"skillId":"missing-skill","enabled":true}]`, `"mcps":[{"mcpId":"official/tavily","enabled":true}]`} {
		t.Run(config, func(t *testing.T) {
			f := newFixture(t)
			graph := strings.Replace(twoAgents, `"roleId":"reviewer"`, config, 1)
			runID := f.run(t, graph, str("check"))
			if _, err := f.host.StartRun(context.Background(), f.scope, runID); err != nil {
				t.Fatal(err)
			}
			run, nodes := f.await(t, runID, terminal)
			if run.Status != store.WorkflowStatusFailed || errorKind(t, node(nodes, "a")) != "missing_agent_config" {
				t.Fatalf("not refused: %+v", run)
			}
			f.exec.mu.Lock()
			defer f.exec.mu.Unlock()
			if len(f.exec.started) != 0 {
				t.Fatal("started despite missing dependency")
			}
		})
	}
}
