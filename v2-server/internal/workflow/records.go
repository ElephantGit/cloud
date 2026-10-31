package workflow

import (
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// Admission contains bindings already resolved by the application against the
// shared execution catalog. It is an internal command, not an HTTP request DTO.
// The store loads the published graph itself; callers cannot substitute a draft.
type Admission struct {
	SnapshotID  string            `json:"snapshotId"`
	ProjectID   string            `json:"projectId"`
	Input       Object            `json:"input"`
	Executors   map[string]string `json:"executors"`
	RolePrompts map[string]string `json:"rolePrompts,omitempty"`
}

// Record owns workflow state only. Execution sessions, turns, events and final
// results remain in execution.Service. Revision fences concurrent worker writes.
type Record struct {
	ID                string          `json:"id"`
	Scope             execution.Scope `json:"scope"`
	SnapshotID        string          `json:"snapshotId"`
	Definition        Definition      `json:"definition"`
	Checkpoint        Checkpoint      `json:"checkpoint"`
	Revision          int64           `json:"revision"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	FinishedAt        *time.Time      `json:"finishedAt"`
	CancelRequestedAt *time.Time      `json:"cancelRequestedAt"`
}

// AttemptRecord retains the complete caller identity before Start and its receipt
// afterwards, including container rounds and retries. Settled means the caller
// confirmed End or established that Start was not accepted; a turn result alone
// does not settle an attempt. History is read through the execution service.
type AttemptRecord struct {
	Sequence  int64         `json:"sequence"`
	Attempt   Attempt       `json:"attempt"`
	Ref       execution.Ref `json:"ref"`
	CreatedAt time.Time     `json:"createdAt"`
	SettledAt *time.Time    `json:"settledAt"`
}

// InterruptCheckpoint settles graph state without constructing a new machine or
// replaying a node. The owner must first confirm every outstanding effect stopped.
func InterruptCheckpoint(checkpoint Checkpoint, cancelled bool) Checkpoint {
	m := &Machine{states: workflowStatesFromJSON(checkpoint.State.Nodes), status: checkpoint.State.Status, failure: checkpoint.State.Error}
	if m.status == "pending" {
		m.status = "running"
	}
	m.Interrupt(cancelled)
	return Checkpoint{State: m.Snapshot()}
}
