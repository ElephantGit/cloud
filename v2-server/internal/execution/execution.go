// Package execution defines the in-process Agent execution boundary shared by v2 pages.
// It contains no Node driver, database implementation, or HTTP routes.
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Stable errors let consumers distinguish admission failures from an admitted turn's result.
var (
	ErrUnavailable     = errors.New("execution unavailable")
	ErrNotFound        = errors.New("execution not found")
	ErrInvalidRequest  = errors.New("invalid execution request")
	ErrWorkspaceBusy   = errors.New("workspace busy")
	ErrConflict        = errors.New("execution identity conflict")
	ErrUnsupported     = errors.New("execution capability unsupported")
	ErrStopUnconfirmed = errors.New("execution stop not confirmed")
)

// Scope comes from the authenticated page service, never directly from a request body.
// Implementations must check membership and resource ownership on every operation.
// Hidden resources return ErrNotFound, including lookups through caller identity.
type Scope struct {
	TenantID string
	UserID   string
}

// CallerKind identifies the page service that owns the business operation.
type CallerKind string

const (
	CallerChat     CallerKind = "chat"
	CallerWorkflow CallerKind = "workflow"
	CallerSchedule CallerKind = "schedule"
)

// Round identifies one enclosing workflow container iteration, from outermost to innermost.
// Index is zero-based. ContainerID prevents two sibling containers from aliasing.
type Round struct {
	ContainerID string
	Index       int
}

// Caller is an immutable attempt identity within Scope. ID is the chat conversation ID,
// workflow run ID, or schedule occurrence ID, respectively. AttemptID identifies a single
// prompt admission, including a chat turn. A retry after a terminal turn uses a NEW AttemptID.
// NodeID and Rounds apply only to workflow callers; all fields participate in identity.
type Caller struct {
	Kind      CallerKind
	ID        string
	NodeID    string
	Rounds    []Round
	AttemptID string
}

// Target selects a prepared project and workspace. ProjectID is required in the first runtime.
// An empty WorkspaceID selects that project's main workspace; a nonempty ID must belong to it.
// Workspace provisioning and project lists belong to the shared project service, not each page.
type Target struct {
	ProjectID   string
	WorkspaceID string
}

// StartRequest starts a session with its first turn. ExecutorID references a server-owned
// Agent-plugin/model binding; it never contains provider credentials or executable paths.
type StartRequest struct {
	Caller     Caller
	Target     Target
	ExecutorID string
	Prompt     string
}

// AppendRequest adds a turn to a live session, retaining its workspace and executor binding.
// A new caller attempt is required. Dead sessions cannot be silently recreated or replayed.
type AppendRequest struct {
	SessionID string
	Caller    Caller
	Prompt    string
}

// Ref identifies an admitted turn. SessionID is stable across Append calls; TurnID is not.
type Ref struct {
	SessionID string
	TurnID    string
}

// Status describes a turn, independently of the Agent's protocol stop reason.
type Status string

const (
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusCancelled   Status = "cancelled"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
)

// Result is written once at termination. Answer is the assembled visible assistant text,
// excluding thought/tool output; it can be empty or partial on failure/cancellation.
// StopReason retains the protocol reason separately. FailureCode is a stable, sanitized code,
// never an infrastructure error string. Consumers decide how these map to their domain status.
type Result struct {
	Status      Status
	Answer      string
	StopReason  string
	FailureCode string
}

// Turn is the readable record of one admitted prompt. Result is nil only while running.
type Turn struct {
	Ref       Ref
	Caller    Caller
	Prompt    string
	StartedAt time.Time
	EndedAt   *time.Time
	Result    *Result
}

// Session is shared execution metadata; page titles, ordering and deletion remain page-owned.
// EndedAt means the execution session can no longer accept Append, not merely that a turn ended.
type Session struct {
	ID         string
	Target     Target
	ExecutorID string
	CreatedAt  time.Time
	EndedAt    *time.Time
}

// TurnPage lists admitted turns in stable admission order. NextCursor is empty at the current
// end; cursors are opaque and scoped to the session. Events are read separately via Subscribe.
type TurnPage struct {
	Items      []Turn
	NextCursor string
}

// EventKind distinguishes protocol updates from the authoritative turn result.
type EventKind string

const (
	EventACPUpdate EventKind = "acp_update"
	EventTurnEnded EventKind = "turn_ended"
)

// Event is immutable after publication. Sequence starts at 1 and increases without gaps within
// Ref. ACPUpdate holds an unchanged ACP SessionUpdate, preserving its sessionUpdate discriminator
// and extension metadata (text, thought, tool, plan, usage, etc.). Result is set only on TurnEnded.
// Readers must not concatenate thought or tool text into the final answer.
type Event struct {
	Ref        Ref
	Sequence   uint64
	OccurredAt time.Time
	Kind       EventKind
	ACPUpdate  json.RawMessage
	Result     *Result
}

// Stream replays retained events then follows live ones with no handoff gap. Next returns io.EOF
// only after the terminal event is delivered, or immediately if the supplied cursor already
// acknowledges it. A transport/storage failure is a non-EOF error.
// One goroutine calls Next; Close may run concurrently and must unblock it. Context cancellation
// or Close releases the subscription only: stopping observation must never cancel execution.
type Stream interface {
	Next(context.Context) (Event, error)
	Close() error
}

// Service is the shared execution port. It is a proposed contract, not an available runtime.
// Start/Append return after admission; their contexts control admission, not the admitted lifetime.
// Within a scope, repeating an identical caller attempt and request returns the original Ref;
// changing its target, executor, session or prompt returns ErrConflict without dispatching again.
// A workspace admits at most one running turn; contention returns ErrWorkspaceBusy (no hidden queue).
type Service interface {
	Start(context.Context, Scope, StartRequest) (Ref, error)
	Append(context.Context, Scope, AppendRequest) (Ref, error)

	// Subscribe returns events strictly after afterSequence, replaying from the start at zero.
	// An ahead-of-history cursor is invalid, rather than silently discarding a terminal event.
	Subscribe(ctx context.Context, scope Scope, ref Ref, afterSequence uint64) (Stream, error)

	// Cancel waits for execution-side confirmation and durable terminal publication. It is
	// idempotent for terminal turns and returns their actual result, including a racing success.
	// A deadline or ErrStopUnconfirmed means stopping is NOT confirmed: consumers must not
	// publish cancellation or start a replacement attempt on that basis. The stop request may
	// still complete later; callers reconcile via ReadAttempt or Subscribe.
	Cancel(context.Context, Scope, Ref) (Result, error)

	// End closes a session and confirms its execution-side cleanup, cancelling any active turn.
	// Successful End is idempotent. Failure must not be presented as confirmed cleanup.
	End(context.Context, Scope, string) error
	ReadSession(context.Context, Scope, string) (Session, error)
	// ReadTurns accepts a limit from 1 to 100; an empty cursor starts at the first turn.
	ReadTurns(ctx context.Context, scope Scope, sessionID, cursor string, limit int) (TurnPage, error)
	ReadAttempt(context.Context, Scope, Caller) (Turn, error)
}
