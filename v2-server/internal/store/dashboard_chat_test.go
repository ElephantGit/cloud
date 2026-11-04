package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/requestmetrics"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func dashboardChat(t *testing.T, f *executionFixture, scope execution.Scope, id string) (store.ChatConversation, execution.Ref) {
	t.Helper()
	ctx := t.Context()
	c, err := f.s.CreateChatConversation(ctx, store.NewChatConversation{TenantID: scope.TenantID, UserID: scope.UserID, ProjectID: "dashboard-project", ExecutorID: "opencode:fixture", CreateKey: id, Title: "对话 " + id})
	if err != nil {
		t.Fatal(err)
	}
	ref := execution.Ref{SessionID: id, TurnID: id + "-turn"}
	session := execution.Session{ID: id, Target: &execution.Target{ProjectID: c.ProjectID}, Executor: &execution.Executor{ID: c.ExecutorID, AgentID: "official/ora-space.opencode"}, Model: execution.Model{ID: "fixture"}}
	turn := execution.Turn{Ref: ref, Caller: execution.Caller{Kind: execution.CallerChat, ID: c.ID, AttemptID: id}, Prompt: execution.TextPrompt("hi")}
	if err := f.s.CreateExecutionSession(ctx, scope, session, turn); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordChatTurn(ctx, c.ID, id, id, ref.TurnID, time.Now()); err != nil {
		t.Fatal(err)
	}
	return c, ref
}

func TestDashboardChatSourceOwnershipHistoryAndDeletion(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	if _, err := f.s.CreateProject(ctx, f.scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
		t.Fatal(err)
	}
	other := executionMember(t, f.s, "other")
	if other.TenantID != f.scope.TenantID {
		t.Fatal("fixture must share a tenant")
	}
	c, ref := dashboardChat(t, f, f.scope, "my-chat")
	dashboardChat(t, f, other, "private-chat")
	dashboardExecution(t, f, "shared-task")
	raw := json.RawMessage(`{"seq":1,"type":"turnEnded"}`)
	if _, err := f.s.AppendExecutionEvent(ctx, f.scope, execution.Event{Ref: ref, Kind: execution.EventHistorySnapshot, History: &execution.HistorySnapshot{Records: []execution.HistoryRecord{{Sequence: 1, Data: raw}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.BeginModelRequest(ctx, ref, requestmetrics.Attempt{Protocol: "openai-completions", Usage: requestmetrics.Usage{Availability: "unavailable", InputSemantics: "unknown"}}); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []execution.Scope{f.scope, other} {
		items, _, err := f.s.DashboardSessions(ctx, scope, store.DashboardFilter{})
		if err != nil || len(items) != 2 {
			t.Fatalf("visible sessions: %v %v", items, err)
		}
		for _, item := range items {
			if item.Source == "chat" && ((scope == f.scope && item.ID != "my-chat") || (scope == other && item.ID != "private-chat")) {
				t.Fatalf("private chat leaked: %+v", item)
			}
		}
	}
	items, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{Source: "chat", Project: c.ProjectID, Agent: "official/ora-space.opencode"})
	if err != nil || len(items) != 1 || items[0].Href != "/chat?conversation="+c.ID {
		t.Fatalf("chat filter/link: %v %v", items, err)
	}
	history, incomplete, err := f.s.DashboardHistory(ctx, f.scope, ref.SessionID)
	if err != nil || incomplete || len(history) != 1 {
		t.Fatalf("history: %v %t %v", history, incomplete, err)
	}
	requests, _, err := f.s.DashboardRequests(ctx, f.scope, ref.SessionID, "")
	if err != nil || len(requests) != 1 {
		t.Fatalf("requests: %v %v", requests, err)
	}
	checkDenied := func(scope execution.Scope) {
		t.Helper()
		_, meta := f.s.DashboardMetadata(ctx, scope, ref.SessionID)
		_, _, history := f.s.DashboardHistory(ctx, scope, ref.SessionID)
		_, _, requests := f.s.DashboardRequests(ctx, scope, ref.SessionID, "")
		_, stats := f.s.DashboardStatistics(ctx, scope, ref.SessionID)
		for name, err := range map[string]error{"metadata": meta, "history": history, "requests": requests, "statistics": stats} {
			if !errors.Is(err, execution.ErrNotFound) {
				t.Errorf("%s private read: %v", name, err)
			}
		}
	}
	checkDenied(other)
	if err := f.s.DeleteChatConversation(ctx, f.scope.TenantID, f.scope.UserID, c.ID); err != nil {
		t.Fatal(err)
	}
	checkDenied(f.scope)
	if _, err := f.s.ReadExecutionSession(ctx, f.scope, ref.SessionID); err != nil {
		t.Fatalf("deletion must retain execution history: %v", err)
	}
}

func TestDashboardChatRejectsWrongParentAndOtherCallers(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE v2_execution_turns SET caller=jsonb_set(caller,'{ID}','"wrong-conversation"')`,
		`UPDATE v2_execution_turns SET caller=jsonb_set(caller,'{Kind}','"schedule"')`,
		`UPDATE v2_execution_turns SET caller=jsonb_set(caller,'{Kind}','"workflow"')`,
		`UPDATE v2_execution_sessions SET data=jsonb_set(data,'{Target,ProjectID}','"wrong-project"')`,
		`UPDATE v2_chat_conversations SET executor_id='another-executor'`,
		`UPDATE v2_execution_sessions SET user_id=(SELECT id FROM users WHERE subject='other@demo.test')`,
		`UPDATE v2_execution_sessions SET tenant_id=(SELECT id FROM tenants WHERE name='other')`,
	} {
		t.Run(mutation, func(t *testing.T) {
			f := executionDatabase(t)
			ctx := t.Context()
			if _, err := f.s.CreateProject(ctx, f.scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
				t.Fatal(err)
			}
			_, ref := dashboardChat(t, f, f.scope, "mismatched")
			executionMember(t, f.s, "other")
			if _, err := f.admin.Exec(ctx, `INSERT INTO tenants(name) VALUES('other')`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.admin.Exec(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			items, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{})
			if err != nil || len(items) != 0 {
				t.Fatalf("unbound chat visible: %v %v", items, err)
			}
			if _, err := f.s.DashboardMetadata(ctx, f.scope, ref.SessionID); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("unbound metadata: %v", err)
			}
		})
	}
}

func TestDashboardChatPaginationBindsSource(t *testing.T) {
	f := executionDatabase(t)
	ctx := t.Context()
	if _, err := f.s.CreateProject(ctx, f.scope, store.Project{ID: "dashboard-project", Name: "Dashboard", RepositoryURL: "fixture://demo", Commit: "abc", MirrorPath: "/tmp/dashboard"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 51; i++ {
		dashboardChat(t, f, f.scope, fmt.Sprintf("chat-%02d", i))
	}
	dashboardExecution(t, f, "shared-task")
	first, cursor, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{Source: "chat"})
	if err != nil || len(first) != 50 || cursor == "" {
		t.Fatalf("first page: %d %q %v", len(first), cursor, err)
	}
	tail, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{Source: "chat", After: cursor})
	if err != nil || len(tail) != 1 {
		t.Fatalf("tail: %v %v", tail, err)
	}
	for _, item := range first {
		if item.ID == tail[0].ID {
			t.Fatal("duplicate chat across pages")
		}
	}
	if _, _, err := f.s.DashboardSessions(ctx, f.scope, store.DashboardFilter{Source: "task", After: cursor}); !errors.Is(err, execution.ErrInvalidRequest) {
		t.Fatalf("source cursor accepted: %v", err)
	}
}
