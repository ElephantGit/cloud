package workflowhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/workflow"
	"strings"
	"testing"
)

const formGraph = `{"nodes":[
 {"id":"start","data":{"kind":"start","inputVariables":[{"name":"findings","valueType":"array[object]","value":[{"key":"disk:root","title":"Disk usage","description":"Over threshold"}]}]}},
 {"id":"review","data":{"kind":"form","formConfig":{"title":"Review","submitLabel":"Submit","reportSelector":["start","input"],"itemsSelector":["start","findings"],"fields":[]}}},
 {"id":"tasks","data":{"kind":"createTasks","createTasksConfig":{"formNodeId":"review"}}},
 {"id":"output","data":{"kind":"output","outputs":[{"name":"tasks","variableSelector":["tasks","structured_output","tasks"]}]}}
 ],"edges":[{"source":"start","target":"review"},{"source":"review","target":"tasks"},{"source":"tasks","target":"output"}]}`

func TestFormSubmissionCreatesTasksAndReusesAcrossRuns(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	report := "Inspection report"
	for i := 0; i < 3; i++ {
		if i == 2 {
			tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
			if err != nil {
				t.Fatal(err)
			}
			task := tasks[0]
			for _, status := range []string{"running", "review", "done"} {
				task, err = f.s.SetTaskStatus(ctx, f.scope.TenantID, f.scope.UserID, task.ID, status, task.Version)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		id := f.run(t, formGraph, &report)
		if _, err := f.host.StartRun(ctx, f.scope, id); err != nil {
			t.Fatal(err)
		}
		f.await(t, id, func(_ store.WorkflowRun, nodes []store.WorkflowNodeRun) bool {
			return node(nodes, "review").Status == "pending"
		})
		form, err := f.host.ReadForm(ctx, f.scope, id)
		if err != nil {
			t.Fatal(err)
		}
		if !form.CanEdit || form.Report != report {
			t.Fatalf("form = %+v", form)
		}
		answers, _ := json.Marshal([]map[string]any{{"key": "disk:root", "decision": "confirmed", "assignee": map[string]string{"type": "human", "id": f.scope.UserID}, "values": map[string]string{}}})
		if i == 1 {
			answers = json.RawMessage(`[{"key":"disk:root","decision":"confirmed","values":{}}]`)
		}
		draft, err := f.host.SaveForm(ctx, f.scope, id, form.Version, "", answers, false)
		if err != nil {
			t.Fatal(err)
		}
		if draft.Status != "open" {
			t.Fatal("draft advanced the form")
		}
		if _, err := f.host.SaveForm(ctx, f.scope, id, draft.Version, "submit", answers, true); err != nil {
			t.Fatal(err)
		}
		f.await(t, id, func(r store.WorkflowRun, _ []store.WorkflowNodeRun) bool { return r.Status == "succeeded" })
		if _, err := f.host.SaveForm(ctx, f.scope, id, draft.Version, "submit", answers, true); err != nil {
			t.Fatal(err)
		}
		tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
		wantCount := 1
		if i == 2 {
			wantCount = 2
		}
		if err != nil || len(tasks) != wantCount {
			t.Fatalf("tasks=%v error=%v", tasks, err)
		}
		if i == 1 {
			current, err := f.s.Task(ctx, f.scope.TenantID, f.scope.UserID, tasks[0].ID)
			if err != nil || len(current.WorkflowSources) != 2 {
				t.Fatalf("missing report associations: %+v %v", current, err)
			}
			if _, err := f.s.UpdateTask(ctx, f.scope.TenantID, f.scope.UserID, current.ID, store.TaskInput{Title: current.Title, Description: "Edited task text", ProjectID: current.ProjectID, Version: current.Version}); err != nil {
				t.Fatal(err)
			}
			current, err = f.s.Task(ctx, f.scope.TenantID, f.scope.UserID, current.ID)
			if err != nil || len(current.WorkflowSources) != 2 {
				t.Fatalf("task edit lost report associations: %+v %v", current, err)
			}
		}
		form, err = f.host.ReadForm(ctx, f.scope, id)
		if err != nil {
			t.Fatal(err)
		}
		var results []store.FormTaskResult
		if err := json.Unmarshal(form.Results, &results); err != nil {
			t.Fatal(err)
		}
		want := "created"
		if i == 1 {
			want = "reused"
		}
		if len(results) != 1 || results[0].Action != want {
			t.Fatalf("results=%+v", results)
		}
	}
}

func TestFormWaitSurvivesRestartAndCancelFencesSubmit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	report := "Evidence"
	id := f.run(t, formGraph, &report)
	if _, err := f.host.StartRun(ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	f.await(t, id, func(_ store.WorkflowRun, nodes []store.WorkflowNodeRun) bool {
		return node(nodes, "review").Status == "pending"
	})
	original, err := f.host.ReadForm(ctx, f.scope, id)
	if err != nil {
		t.Fatal(err)
	}
	answers := json.RawMessage(`[{"key":"disk:root","decision":"rejected","values":{}}]`)
	saved, err := f.host.SaveForm(ctx, f.scope, id, original.Version, "", answers, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.host.SaveForm(ctx, f.scope, id, original.Version, "", answers, false); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale draft: %v", err)
	}
	f.host.Close()
	f.host = f.newHost(t)
	if err := f.host.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := f.host.ReadForm(ctx, f.scope, id)
	if err != nil || restored.Version != saved.Version || restored.Status != "open" {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	if _, err := f.host.CancelRun(ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.host.SaveForm(ctx, f.scope, id, restored.Version, "cancelled-submit", answers, true); !errors.Is(err, store.ErrWorkflowRunState) {
		t.Fatalf("cancelled submission = %v", err)
	}
	tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("cancelled run created tasks: %v %v", tasks, err)
	}
}

func TestFormOnlyReviewerOrAdministratorCanSubmit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	report := "Evidence"
	id := f.run(t, formGraph, &report)
	if _, err := f.host.StartRun(ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	f.await(t, id, func(_ store.WorkflowRun, nodes []store.WorkflowNodeRun) bool {
		return node(nodes, "review").Status == "pending"
	})
	if _, err := f.s.UpsertUser(ctx, store.PresetUser{Email: "form-reader@demo.test", DisplayName: "Reader", Role: store.RoleMember}); err != nil {
		t.Fatal(err)
	}
	u, err := f.s.UserByEmail(ctx, "form-reader@demo.test")
	if err != nil {
		t.Fatal(err)
	}
	reader := f.scope
	reader.UserID = u.ID
	form, err := f.host.ReadForm(ctx, reader, id)
	if err != nil || form.CanEdit {
		t.Fatalf("reader form=%+v %v", form, err)
	}
	answers := json.RawMessage(`[{"key":"disk:root","decision":"rejected","values":{}}]`)
	if _, err := f.host.SaveForm(ctx, reader, id, form.Version, "reader", answers, true); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("unauthorized submission: %v", err)
	}
	if _, err := f.host.SaveForm(ctx, f.scope, id, form.Version, "owner", answers, true); err != nil {
		t.Fatal(err)
	}
	f.await(t, id, func(r store.WorkflowRun, _ []store.WorkflowNodeRun) bool { return r.Status == "succeeded" })
	tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("all rejected created tasks: %v %v", tasks, err)
	}
}

func TestSubmittedFormRecoversBeforeAndDuringTaskCreation(t *testing.T) {
	for _, scenario := range []struct{ started, disable bool }{{false, false}, {true, false}, {true, true}} {
		started := scenario.started
		t.Run(fmt.Sprintf("%t-%t", started, scenario.disable), func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			report := "Evidence"
			id := f.run(t, formGraph, &report)
			if _, err := f.host.StartRun(ctx, f.scope, id); err != nil {
				t.Fatal(err)
			}
			f.await(t, id, func(_ store.WorkflowRun, nodes []store.WorkflowNodeRun) bool {
				return node(nodes, "review").Status == "pending"
			})
			form, err := f.host.ReadForm(ctx, f.scope, id)
			if err != nil {
				t.Fatal(err)
			}
			assignee := f.scope.UserID
			if scenario.disable {
				if _, err := f.s.UpsertUser(ctx, store.PresetUser{Email: "retired-reviewer-target@demo.test", DisplayName: "Target", Role: store.RoleMember}); err != nil {
					t.Fatal(err)
				}
				user, err := f.s.UserByEmail(ctx, "retired-reviewer-target@demo.test")
				if err != nil {
					t.Fatal(err)
				}
				assignee = user.ID
			}
			values := []any{map[string]any{"key": "disk:root", "decision": "confirmed", "assignee": map[string]string{"type": "human", "id": assignee}, "values": map[string]string{}}}
			answers, _ := json.Marshal(values)
			_, err = f.s.SaveWorkflowForm(ctx, f.scope.TenantID, f.scope.UserID, id, form.Version, "crash-gap", answers, true, func(store.WorkflowForm) error { return nil }, func(raw []byte, form store.WorkflowForm) ([]byte, error) {
				p, _, err := workflow.DecodeRunPayload(raw)
				if err != nil {
					return nil, err
				}
				text := string(answers)
				if err := p.Complete(form.NodeID, "form", nil, &text, map[string]any{"answers": values}); err != nil {
					return nil, err
				}
				return p.Encode()
			})
			if err != nil {
				t.Fatal(err)
			}
			if started {
				if _, err := f.s.StartWorkflowEngineNodes(ctx, id, []store.WorkflowEngineStart{{NodeID: "tasks", NodeType: "createTasks"}}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.disable {
				if err := f.s.RetireUsers(ctx, []store.RetiredUser{{Email: "retired-reviewer-target@demo.test"}}); err != nil {
					t.Fatal(err)
				}
			}
			f.host.Close()
			f.host = f.newHost(t)
			if err := f.host.Interrupt(ctx); err != nil {
				t.Fatal(err)
			}
			if scenario.disable {
				for range 2 {
					run, err := f.s.WorkflowRun(ctx, f.scope.TenantID, id)
					if err != nil || run.Status != "failed" {
						t.Fatalf("recovery failure stranded run: %+v %v", run, err)
					}
					if _, err := f.host.RetryForm(ctx, f.scope, id); err == nil {
						t.Fatal("invalid assignee retry unexpectedly succeeded")
					}
				}
				return
			}
			f.await(t, id, func(r store.WorkflowRun, _ []store.WorkflowNodeRun) bool { return r.Status == "succeeded" })
			if err := f.host.Interrupt(ctx); err != nil {
				t.Fatal(err)
			}
			tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
			if err != nil || len(tasks) != 1 {
				t.Fatalf("recovery tasks=%v err=%v", tasks, err)
			}
		})
	}
}

func TestExistingTaskClosedAfterSubmitUsesFrozenAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	report := "Evidence"
	start := func() (string, store.WorkflowForm) {
		t.Helper()
		id := f.run(t, formGraph, &report)
		if _, err := f.host.StartRun(ctx, f.scope, id); err != nil {
			t.Fatal(err)
		}
		f.await(t, id, func(_ store.WorkflowRun, nodes []store.WorkflowNodeRun) bool {
			return node(nodes, "review").Status == "pending"
		})
		form, err := f.host.ReadForm(ctx, f.scope, id)
		if err != nil {
			t.Fatal(err)
		}
		return id, form
	}
	id, form := start()
	answers, _ := json.Marshal([]map[string]any{{"key": "disk:root", "decision": "confirmed", "assignee": map[string]string{"type": "human", "id": f.scope.UserID}, "values": map[string]string{}}})
	if _, err := f.host.SaveForm(ctx, f.scope, id, form.Version, "first", answers, true); err != nil {
		t.Fatal(err)
	}
	f.await(t, id, func(r store.WorkflowRun, _ []store.WorkflowNodeRun) bool { return r.Status == "succeeded" })
	tasks, err := f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
	if err != nil {
		t.Fatal(err)
	}
	old := tasks[0]
	id, form = start()
	answers = json.RawMessage(`[{"key":"disk:root","decision":"confirmed","values":{}}]`)
	frozen, err := f.s.SaveWorkflowForm(ctx, f.scope.TenantID, f.scope.UserID, id, form.Version, "second", answers, true, func(store.WorkflowForm) error { return nil }, func(raw []byte, form store.WorkflowForm) ([]byte, error) {
		p, _, err := workflow.DecodeRunPayload(raw)
		if err != nil {
			return nil, err
		}
		var values any
		if err := json.Unmarshal(form.Answers, &values); err != nil {
			return nil, err
		}
		text := string(form.Answers)
		if err := p.Complete(form.NodeID, "form", nil, &text, map[string]any{"answers": values}); err != nil {
			return nil, err
		}
		return p.Encode()
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(frozen.Answers), f.scope.UserID) {
		t.Fatal("fallback assignee was not frozen")
	}
	for _, status := range []string{"running", "review", "done"} {
		old, err = f.s.SetTaskStatus(ctx, f.scope.TenantID, f.scope.UserID, old.ID, status, old.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.host.resumeReviewRun(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.await(t, id, func(r store.WorkflowRun, _ []store.WorkflowNodeRun) bool { return r.Status == "succeeded" })
	tasks, err = f.s.Tasks(ctx, f.scope.TenantID, f.scope.UserID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("new task after close: %+v %v", tasks, err)
	}
}
