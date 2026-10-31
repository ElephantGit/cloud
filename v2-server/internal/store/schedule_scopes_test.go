package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestScheduleScopesFollowCurrentAuthorizationAndIncludePaused(t *testing.T) {
	f := executionDatabase(t)
	ctx := context.Background()
	scope := f.scope
	input := store.ScheduleInput{Name: "one", Target: json.RawMessage(`{"kind":"chat","model":"fixture","prompt":"hello"}`), Rule: json.RawMessage(`{"kind":"hours","hours":1,"timezone":"UTC"}`)}
	one, err := f.s.CreateScheduleConfig(ctx, scope.TenantID, scope.UserID, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Name = "two"
	if _, err = f.s.CreateScheduleConfig(ctx, scope.TenantID, scope.UserID, input); err != nil {
		t.Fatal(err)
	}
	paused := false
	if _, err = f.s.UpdateScheduleConfig(ctx, scope.TenantID, scope.UserID, one.ID, store.ScheduleInput{Version: 1}, &paused); err != nil {
		t.Fatal(err)
	}
	assertCount := func(want int) {
		t.Helper()
		items, err := f.s.ScheduleScopes(ctx)
		if err != nil || len(items) != want {
			t.Fatalf("scopes=%+v err=%v want=%d", items, err, want)
		}
		if want == 1 && items[0] != scope {
			t.Fatal("wrong owner")
		}
	}
	assertCount(1)
	for _, mutation := range []struct{ hide, restore string }{
		{`UPDATE users SET status='disabled' WHERE id=$1`, `UPDATE users SET status='active' WHERE id=$1`},
		{`UPDATE users SET deleted_at=now() WHERE id=$1`, `UPDATE users SET deleted_at=NULL WHERE id=$1`},
	} {
		if _, err = f.admin.Exec(ctx, mutation.hide, scope.UserID); err != nil {
			t.Fatal(err)
		}
		assertCount(0)
		if _, err = f.s.GetScheduleConfig(ctx, scope.TenantID, scope.UserID, one.ID); err != store.ErrNotFound {
			t.Fatal(err)
		}
		if _, err = f.admin.Exec(ctx, mutation.restore, scope.UserID); err != nil {
			t.Fatal(err)
		}
		assertCount(1)
	}
	if _, err = f.admin.Exec(ctx, `UPDATE tenants SET status='paused' WHERE id=$1`, scope.TenantID); err != nil {
		t.Fatal(err)
	}
	assertCount(0)
	if _, err = f.admin.Exec(ctx, `UPDATE tenants SET status='active' WHERE id=$1`, scope.TenantID); err != nil {
		t.Fatal(err)
	}
	assertCount(1)
	if _, err = f.admin.Exec(ctx, `DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2`, scope.TenantID, scope.UserID); err != nil {
		t.Fatal(err)
	}
	assertCount(0)
	if _, err = f.s.GetScheduleConfig(ctx, scope.TenantID, scope.UserID, one.ID); err != store.ErrNotFound {
		t.Fatal(err)
	}
}
