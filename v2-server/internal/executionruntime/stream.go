package executionruntime

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Subscribe reads directly from the durable cursor. A short poll avoids a replay/live handoff
// race without another in-memory event log; closing this observer never cancels a worker.
func (s *Service) Subscribe(ctx context.Context, scope execution.Scope, sid string, after uint64) (execution.Stream, error) {
	latest, err := s.db.ReadExecutionCursor(ctx, scope, sid)
	if err != nil {
		return nil, publicError(err)
	}
	if after > latest {
		return nil, execution.ErrInvalidRequest
	}
	owned, cancel := context.WithCancel(ctx)
	return &stream{s: s, scope: scope, sid: sid, cursor: after, ctx: owned, cancel: cancel}, nil
}

type stream struct {
	s      *Service
	scope  execution.Scope
	sid    string
	cursor uint64
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (st *stream) Close() error { st.once.Do(st.cancel); return nil }
func (st *stream) Next(ctx context.Context) (execution.Event, error) {
	query, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(st.ctx, cancel)
	defer stop()
	for {
		if err := query.Err(); err != nil {
			return execution.Event{}, err
		}
		events, err := st.s.db.ReadExecutionEvents(query, st.scope, st.sid, st.cursor, 1)
		if err != nil {
			return execution.Event{}, publicError(err)
		}
		if len(events) > 0 {
			st.cursor = events[0].Sequence
			return events[0], nil
		}
		session, err := st.s.ReadSession(query, st.scope, st.sid)
		if err != nil {
			return execution.Event{}, err
		}
		if session.EndedAt != nil {
			// End may commit between the empty event query and this session read. Recheck
			// after observing EndedAt so the terminal event cannot be skipped as EOF.
			events, err = st.s.db.ReadExecutionEvents(query, st.scope, st.sid, st.cursor, 1)
			if err != nil {
				return execution.Event{}, publicError(err)
			}
			if len(events) > 0 {
				st.cursor = events[0].Sequence
				return events[0], nil
			}
			return execution.Event{}, io.EOF
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-query.Done():
			timer.Stop()
			return execution.Event{}, query.Err()
		case <-timer.C:
		}
	}
}
