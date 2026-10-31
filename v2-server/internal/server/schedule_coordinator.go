package server

import (
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/scheduler"
)

// NewScheduleCoordinator uses the same execution dependency as page handlers.
// The host owns one instance, calls Recover after public execution startup recovery,
// then drives Tick/Manual, or gives it to a scheduler.Host. Construction starts no timer.
func NewScheduleCoordinator(records scheduler.CoordinatorStore, o Options, now func() time.Time) (*scheduler.Coordinator, error) {
	adapter, err := scheduler.NewConversationAdapter(records, o.Execution)
	if err != nil {
		return nil, err
	}
	return scheduler.NewCoordinator(records, adapter, now)
}
