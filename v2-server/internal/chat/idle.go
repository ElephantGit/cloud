package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// ReclaimIdle ends every live session whose latest turn finished more than IdleAfter ago, so an
// idle conversation stops holding its workspace. A running turn is never ended here. It returns
// how many sessions it ended and every failure; one failure does not stop the rest.
func (s *Service) ReclaimIdle(ctx context.Context) (int, error) {
	cutoff := s.now().Add(-s.idle)
	idle, e := s.store.IdleChatSessions(ctx, cutoff)
	if e != nil {
		return 0, e
	}
	ended := 0
	var errs []error
	for _, x := range idle {
		ok, e := s.reclaim(ctx, x, cutoff)
		if e != nil {
			errs = append(errs, fmt.Errorf("session %s: %w", x.Session.ID, e))
		}
		if ok {
			ended++
		}
	}
	return ended, errors.Join(errs...)
}

func (s *Service) reclaim(ctx context.Context, x store.IdleChatSession, cutoff time.Time) (bool, error) {
	defer s.lock(x.ConversationID)()
	scope := execution.Scope{TenantID: x.TenantID, UserID: x.UserID}
	// Re-read under the lock: a prompt may have arrived, or a stop ended the session.
	c, e := s.store.ChatConversation(ctx, x.TenantID, x.UserID, x.ConversationID)
	if errors.Is(e, store.ErrNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	live, ok := c.Live()
	if !ok || live.ID != x.Session.ID || !live.LastPromptAt.Before(cutoff) {
		return false, nil
	}
	turn, e := s.runtime.ReadAttempt(ctx, scope, caller(c.ID, live.LastAttemptID))
	if e != nil {
		return false, fmt.Errorf("read latest turn: %w", e)
	}
	if turn.Result == nil || turn.EndedAt == nil || !turn.EndedAt.Before(cutoff) {
		return false, nil
	}
	if e := s.ending(ctx, live.ID, store.EndIdle, func() error { return s.runtime.End(ctx, scope, live.ID) }); e != nil {
		return false, fmt.Errorf("end idle session: %w", e)
	}
	return true, s.store.EndChatSession(ctx, live.ID, store.EndIdle, s.now())
}

// RunIdleReclaim sweeps every interval until ctx ends. Failures are logged and the next sweep
// retries them.
func (s *Service) RunIdleReclaim(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if n, e := s.ReclaimIdle(ctx); e != nil && ctx.Err() == nil {
			slog.Error("chat idle reclaim", "ended", n, "error", e)
		}
	}
}
