package executionruntime

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
)

type revocationProvider struct {
	started, release         chan struct{}
	startedOnce, releaseOnce sync.Once
}

func (p *revocationProvider) resume() { p.releaseOnce.Do(func() { close(p.release) }) }
func (p *revocationProvider) Complete(w http.ResponseWriter, r *http.Request, model nodeservices.Model) {
	p.startedOnce.Do(func() { close(p.started) })
	select {
	case <-p.release:
		nodeservices.FixtureProvider{}.Complete(w, r, model)
	case <-r.Context().Done():
	}
}

func TestAcceptedModelTurnFinishesAfterPermissionRevocation(t *testing.T) {
	for _, tc := range []struct{ name, revoke, restore string }{
		{"membership_removed", "DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2", "INSERT INTO memberships(tenant_id,user_id,role) VALUES ($1,$2,'owner')"},
		{"user_disabled", "UPDATE users SET status='disabled' WHERE id=$2 AND $1::uuid IS NOT NULL", "UPDATE users SET status='active' WHERE id=$2 AND $1::uuid IS NOT NULL"},
		{"tenant_suspended", "UPDATE tenants SET status='suspended' WHERE id=$1 AND $2::uuid IS NOT NULL", "UPDATE tenants SET status='active' WHERE id=$1 AND $2::uuid IS NOT NULL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &revocationProvider{started: make(chan struct{}), release: make(chan struct{})}
			f := runtimeDatabase(t, provider)
			t.Cleanup(provider.resume)
			inspect, err := pgx.ConnectConfig(f.ctx, f.config.ConnConfig.Copy())
			if err != nil {
				t.Fatal(err)
			}
			defer inspect.Close(f.ctx)
			request := modelRequest("chat", "first", "complete after revocation")
			ref := startModel(t, f, request)
			select {
			case <-provider.started:
			case <-f.ctx.Done():
				t.Fatal(f.ctx.Err())
			}
			f.service.mu.Lock()
			worker := f.service.workers[ref.TurnID]
			f.service.mu.Unlock()
			if worker == nil {
				t.Fatal("worker finished before provider was released")
			}
			if _, err := inspect.Exec(f.ctx, tc.revoke, f.scope.TenantID, f.scope.UserID); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.ReadSession(f.ctx, f.scope, ref.SessionID); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked read: %v", err)
			}
			if _, err := f.service.StartModel(f.ctx, f.scope, modelRequest("new-chat", "new", "rejected")); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked admission: %v", err)
			}
			if _, err := f.service.Append(f.ctx, f.scope, execution.AppendRequest{SessionID: ref.SessionID, Caller: execution.Caller{Kind: execution.CallerChat, ID: "chat", AttemptID: "next"}, Prompt: execution.TextPrompt("rejected")}); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked Append: %v", err)
			}
			if _, err := f.service.Cancel(f.ctx, f.scope, ref); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked public Cancel: %v", err)
			}
			if err := f.service.End(f.ctx, f.scope, ref.SessionID); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked public End: %v", err)
			}
			provider.resume()
			select {
			case <-worker.done:
			case <-f.ctx.Done():
				t.Fatal("accepted turn remained running after provider completed")
			}
			if worker.err != nil {
				t.Fatalf("revocation prevented durable terminal publication: %v", worker.err)
			}
			if _, err := f.service.ReadAttempt(f.ctx, f.scope, request.Caller); !errors.Is(err, execution.ErrNotFound) {
				t.Fatalf("revoked history: %v", err)
			}
			if _, err := inspect.Exec(f.ctx, tc.restore, f.scope.TenantID, f.scope.UserID); err != nil {
				t.Fatal(err)
			}
			turn, err := f.service.ReadAttempt(f.ctx, f.scope, request.Caller)
			if err != nil || turn.Result == nil || turn.Result.Status != execution.StatusCompleted || !turn.Result.AnswerComplete || turn.Result.Answer != "V2 fixture response." {
				t.Fatalf("accepted result lost: %+v %v", turn, err)
			}
			if err := f.service.End(f.ctx, f.scope, ref.SessionID); err != nil {
				t.Fatal(err)
			}
			next := startModel(t, f, modelRequest("other-chat", "next", "runtime is still available"))
			result, _ := observeTurn(t, f, next, 0)
			if result.Status != execution.StatusCompleted {
				t.Fatalf("revocation poisoned the runtime: %+v", result)
			}
		})
	}
}
