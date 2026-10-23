package integration

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wanglongan587/cloud/internal/core"
)

// scheduledTaskOtherMember seeds a second active tenant member (distinct from the task creator) with
// a verified identity, and returns the Claims the store resolves that member from.
func scheduledTaskOtherMember(t *testing.T, store *core.Store, tenantID string) *core.Claims {
	t.Helper()
	userID := uuid.NewString()
	_, err := store.Pool.Exec(`INSERT INTO users(id, display_name, status) VALUES($1, 'Other member', 'active')`, userID)
	must(t, err)
	_, err = store.Pool.Exec(`INSERT INTO tenant_memberships(tenant_id, user_id, role, status) VALUES($1, $2, 'member', 'active')`, tenantID, userID)
	must(t, err)
	subject := "scheduled-other-" + userID[:8]
	_, err = store.Pool.Exec(`INSERT INTO user_identities(user_id,source,subject) VALUES($1,'test-scheduled',$2)`, userID, subject)
	must(t, err)
	return &core.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: subject}, Source: "test-scheduled", Caller: "gateway", DisplayName: "Other member"}
}

// TestScheduledTaskManagementIsCreatorOnly pins D1 创建者边界：管理写操作（disable/DELETE/PATCH）只对
// 创建者开放，任何其他有效成员都得到与不存在一致的 `404 not_found`，不暴露任务是否存在。
func TestScheduledTaskManagementIsCreatorOnly(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Creator boundary", 65)
	other := scheduledTaskOtherMember(t, store, s.tenantID)
	base := "/api/v1/tenants/" + s.tenantID + "/scheduled-tasks/" + taskID

	_, _, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "POST", Key: "other-disable", Path: base + "/disable",
		TenantID: s.tenantID, ScheduledTaskID: taskID, Identity: other,
	})
	if fault := core.ErrorCode(err); fault.Status != 404 || fault.Code != "not_found" {
		t.Fatalf("non-creator disable must be a 404 not_found fault, got %v", err)
	}

	_, _, err = store.Public(context.Background(), &core.PublicRequest{
		Method: "DELETE", Key: "other-delete", Path: base,
		TenantID: s.tenantID, ScheduledTaskID: taskID, Identity: other,
	})
	if fault := core.ErrorCode(err); fault.Status != 404 || fault.Code != "not_found" {
		t.Fatalf("non-creator delete must be a 404 not_found fault, got %v", err)
	}

	// The creator still owns it: the management write they are entitled to must keep working.
	creator := scheduledTaskCaller(t, store, s.tenantID)
	_, status, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "POST", Key: "creator-disable", Path: base + "/disable",
		TenantID: s.tenantID, ScheduledTaskID: taskID, Identity: creator,
	})
	must(t, err)
	if status != 200 {
		t.Fatalf("creator disable must succeed with 200, got %d", status)
	}
}

// TestScheduledTaskMissingTaskIsNotFound pins the `404` 统一语义：不存在的任务 ID 在管理读与写上都
// 与无权限一样回答 `404 not_found`。
func TestScheduledTaskMissingTaskIsNotFound(t *testing.T) {
	store, s, _ := scheduledTaskStore(t)
	creator := scheduledTaskCaller(t, store, s.tenantID)
	missing := uuid.NewString()
	base := "/api/v1/tenants/" + s.tenantID + "/scheduled-tasks/" + missing

	_, _, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "PATCH", Key: "missing-patch", Path: base,
		TenantID: s.tenantID, ScheduledTaskID: missing,
		Body:     core.Object{"name": "Ghost", "version": int64(1)},
		Identity: creator,
	})
	if fault := core.ErrorCode(err); fault.Status != 404 || fault.Code != "not_found" {
		t.Fatalf("PATCH of a missing task must be a 404 not_found fault, got %v", err)
	}

	_, _, err = store.Public(context.Background(), &core.PublicRequest{
		Method: "GET", Key: "missing-get", Path: base,
		TenantID: s.tenantID, ScheduledTaskID: missing, Identity: creator,
	})
	if fault := core.ErrorCode(err); fault.Status != 404 || fault.Code != "not_found" {
		t.Fatalf("GET of a missing task must be a 404 not_found fault, got %v", err)
	}
}

// TestScheduledTaskVersionConflict pins the `version` 乐观锁：用过期 version 提交的编辑以
// `409 version_conflict` 拒绝，且不产生部分写入。
func TestScheduledTaskVersionConflict(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Version CAS", 65)
	creator := scheduledTaskCaller(t, store, s.tenantID)
	path := "/api/v1/tenants/" + s.tenantID + "/scheduled-tasks/" + taskID

	_, _, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "PATCH", Key: "stale-version", Path: path,
		TenantID: s.tenantID, ScheduledTaskID: taskID,
		Body:     core.Object{"name": "Stale edit", "version": int64(2)},
		Identity: creator,
	})
	if fault := core.ErrorCode(err); fault.Status != 409 || fault.Code != "version_conflict" {
		t.Fatalf("stale version must be a 409 version_conflict fault, got %v", err)
	}
	var version int64
	must(t, store.Pool.QueryRow(`SELECT version FROM scheduled_tasks WHERE id=$1`, taskID).Scan(&version))
	if version != 1 {
		t.Fatalf("a rejected edit must not bump version, got %d", version)
	}
}

// TestScheduledTaskRevokeConsentDisables pins D5 撤销同意自动停用：revoke-consent 后任务被禁用、原因
// 为 `consent_revoked`，且同意策略与同意时刻被清空，既有运行事实不改写。
func TestScheduledTaskRevokeConsentDisables(t *testing.T) {
	store, s, agentID := scheduledTaskStore(t)
	taskID := seedEnabledScheduledTask(t, store, s, agentID, "Revoke consent", 65)
	creator := scheduledTaskCaller(t, store, s.tenantID)
	path := "/api/v1/tenants/" + s.tenantID + "/scheduled-tasks/" + taskID + "/revoke-consent"

	_, status, err := store.Public(context.Background(), &core.PublicRequest{
		Method: "POST", Key: "revoke-consent", Path: path,
		TenantID: s.tenantID, ScheduledTaskID: taskID, Identity: creator,
	})
	must(t, err)
	if status != 200 {
		t.Fatalf("revoke-consent must succeed with 200, got %d", status)
	}

	var enabled bool
	var reason, policy string
	must(t, store.Pool.QueryRow(`SELECT enabled, COALESCE(disabled_reason,''), COALESCE(unattended_policy,'') FROM scheduled_tasks WHERE id=$1`, taskID).Scan(&enabled, &reason, &policy))
	if enabled || reason != "consent_revoked" || policy != "" {
		t.Fatalf("revoke-consent must disable the task and clear consent, got enabled=%v reason=%q policy=%q", enabled, reason, policy)
	}
}
