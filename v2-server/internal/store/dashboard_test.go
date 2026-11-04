package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/requestmetrics"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func dashboardExecution(t *testing.T, f *executionFixture, id string) execution.Ref {
	t.Helper()
	return dashboardExecutionWithStartKey(t, f, id, id)
}

func dashboardExecutionWithStartKey(t *testing.T, f *executionFixture, id, startKey string) execution.Ref {
	t.Helper()
	ctx := t.Context()
	task, err := f.s.CreateTask(ctx, f.scope.TenantID, f.scope.UserID, store.TaskInput{ProjectID: "dashboard-project", Title: "分析 " + id})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.CreateTaskRun(ctx, store.NewTaskRun{TaskID: task.ID, TenantID: f.scope.TenantID, StartedBy: f.scope.UserID, StartKey: startKey, ExecutorID: "opencode:fixture", ExecutorName: "OpenCode"})
	if err != nil {
		t.Fatal(err)
	}
	ref := execution.Ref{SessionID: id, TurnID: id + "-turn"}
	turn := execution.Turn{Ref: ref, Caller: execution.Caller{Kind: execution.CallerTask, ID: run.ID, AttemptID: id}, Prompt: execution.TextPrompt("synthetic")}
	if err := f.s.CreateExecutionSession(ctx, f.scope, execution.Session{ID: id, Executor: &execution.Executor{AgentID: "opencode"}, Model: execution.Model{ID: "fixture"}}, turn); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordTaskTurn(ctx, run.ID, ref.SessionID, ref.TurnID); err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestDashboardExcludesScheduledTaskSessions(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	if _, err := f.s.CreateProject(ctx, f.scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, startKey string
		excluded     bool
	}{
		{"manual", "manual-key", false},
		{"manual-schedule-word", "schedule:manual-key", false},
		{"manual-literal-escape", `\nschedule:manual-key`, false},
		{"scheduled", store.ScheduledTaskStartPrefix + "occurrence", true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			ref := dashboardExecutionWithStartKey(t, f, tc.id, tc.startKey)
			if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: ref, Kind: execution.EventHistorySnapshot, History: &execution.HistorySnapshot{Records: []execution.HistoryRecord{{Sequence: 1, Data: json.RawMessage(`{"seq":1,"type":"turnEnded"}`)}}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.BeginModelRequest(ctx, ref, requestmetrics.Attempt{Protocol: "openai-completions", Usage: requestmetrics.Usage{InputSemantics: "unknown", Availability: "unavailable"}}); err != nil {
				t.Fatal(err)
			}
			checks := map[string]func() error{
				"metadata":   func() error { _, err := f.s.DashboardMetadata(ctx, f.scope, ref.SessionID); return err },
				"history":    func() error { _, _, err := f.s.DashboardHistory(ctx, f.scope, ref.SessionID); return err },
				"requests":   func() error { _, _, err := f.s.DashboardRequests(ctx, f.scope, ref.SessionID, ""); return err },
				"statistics": func() error { _, err := f.s.DashboardStatistics(ctx, f.scope, ref.SessionID); return err },
			}
			for name, check := range checks {
				err := check()
				if tc.excluded && !errors.Is(err, execution.ErrNotFound) || !tc.excluded && err != nil {
					t.Errorf("%s: excluded=%t, error=%v", name, tc.excluded, err)
				}
			}
		})
	}
	for _, filter := range []store.DashboardFilter{{}, {Project: "dashboard-project", Agent: "opencode", Status: "running"}} {
		page, next, err := f.s.DashboardSessions(ctx, f.scope, filter)
		if err != nil || len(page) != 3 || next != "" {
			t.Fatalf("manual sessions: %v, cursor=%q, error=%v", page, next, err)
		}
		for _, session := range page {
			if session.ID == "scheduled" {
				t.Fatal("scheduled task session appeared in the manual-task dashboard")
			}
		}
	}
}

func TestDashboardPaginationHistoryStatisticsAndPrivacy(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	if _, err := f.s.CreateProject(ctx, f.scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
		t.Fatal(err)
	}
	ref := dashboardExecution(t, f, "session-0")
	for i := 1; i < 52; i++ {
		dashboardExecution(t, f, fmt.Sprintf("session-%d", i))
	}
	page, next, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{})
	if err != nil || len(page) != 50 || next == "" {
		t.Fatalf("first page: %d %q %v", len(page), next, err)
	}
	tail, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{After: next})
	if err != nil || len(tail) != 2 {
		t.Fatalf("tail: %d %v", len(tail), err)
	}
	if _, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{Agent: "another", After: next}); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("cursor not filter-bound: %v", err)
	}
	private := executionTurn("chat-session", "chat-turn")
	createExecution(t, f, private.Ref.SessionID, private)
	if _, err := f.s.DashboardMetadata(ctx, f.scope, private.Ref.SessionID); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("chat in dashboard: %v", err)
	}
	record := json.RawMessage(`{"seq":1,"at":"2026-01-01T00:00:00Z","type":"update","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"synthetic"}}}`)
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: ref, Kind: execution.EventHistorySnapshot, History: &execution.HistorySnapshot{Records: []execution.HistoryRecord{{Sequence: 1, Data: record}}}}); err != nil {
		t.Fatal(err)
	}
	history, incomplete, err := f.s.DashboardHistory(ctx, f.scope, ref.SessionID)
	var wantRecord, gotRecord any
	_ = json.Unmarshal(record, &wantRecord)
	if len(history) > 0 {
		_ = json.Unmarshal(history[0], &gotRecord)
	}
	if err != nil || incomplete || len(history) != 1 || !reflect.DeepEqual(wantRecord, gotRecord) {
		t.Fatalf("history: %s %v %v", history, incomplete, err)
	}
	for i := 0; i < 27; i++ {
		a, err := f.s.BeginModelRequest(ctx, ref, requestmetrics.Attempt{Protocol: "openai-completions", ModelID: "fixture", Usage: requestmetrics.Usage{Availability: "unavailable", InputSemantics: "includes_cache"}})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			continue
		}
		now := time.Now()
		zero := int64(0)
		value := int64(10)
		a.FinishedAt = &now
		a.Outcome = "succeeded"
		a.TerminalObserved = true
		a.Usage = requestmetrics.Usage{Input: &zero, Output: &value, Availability: "complete", InputSemantics: "includes_cache"}
		if err := f.s.FinishModelRequest(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := f.s.FinishModelRequest(ctx, a); err != nil {
			t.Fatalf("not idempotent: %v", err)
		}
		a.Outcome = "failed"
		if err := f.s.FinishModelRequest(ctx, a); !errors.Is(err, execution.ErrConflict) {
			t.Fatalf("rewrote completion: %v", err)
		}
	}
	attempts, cursor, err := f.s.DashboardRequests(ctx, f.scope, ref.SessionID, "")
	if err != nil || len(attempts) != 25 || cursor == "" {
		t.Fatalf("attempts: %d %q %v", len(attempts), cursor, err)
	}
	attempts, _, err = f.s.DashboardRequests(ctx, f.scope, ref.SessionID, cursor)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempt tail: %d %v", len(attempts), err)
	}
	statistics, err := f.s.DashboardStatistics(ctx, f.scope, ref.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(statistics)
	var got struct {
		Counts struct {
			Total   int `json:"total"`
			Started int `json:"started"`
		}
		Usage struct {
			Input struct {
				Value        *float64
				KnownCount   int
				Availability string
			} `json:"inputTokens"`
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Counts.Total != 27 || got.Counts.Started != 1 || got.Usage.Input.Value == nil || *got.Usage.Input.Value != 0 || got.Usage.Input.KnownCount != 26 || got.Usage.Input.Availability != "partial" {
		t.Fatalf("statistics=%s", raw)
	}
	if _, err := f.admin.Exec(ctx, `DELETE FROM memberships WHERE user_id=$1`, f.scope.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DashboardStatistics(ctx, f.scope, ref.SessionID); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("revoked read: %v", err)
	}
}
