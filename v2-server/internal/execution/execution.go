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
	ErrUnavailable      = errors.New("execution unavailable")
	ErrNotFound         = errors.New("execution not found")
	ErrInvalidRequest   = errors.New("invalid execution request")
	ErrWorkspaceBusy    = errors.New("workspace busy")
	ErrConflict         = errors.New("execution identity conflict")
	ErrUnsupported      = errors.New("execution capability unsupported")
	ErrStopUnconfirmed  = errors.New("execution stop not confirmed")
	ErrSessionEnded     = errors.New("execution session ended")
	ErrAdmissionUnknown = errors.New("execution admission outcome unknown")
	ErrAdmissionClosed  = errors.New("execution admission closed")
)

// Scope comes from the authenticated page service or persisted schedule owner, never directly
// from a request body. Background callers do not need an online browser or stored cookie.
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
	Prompt     Prompt
}

// AppendRequest adds a turn to a live session, retaining its workspace and executor binding.
// A new caller attempt is required. Dead sessions cannot be silently recreated or replayed.
type AppendRequest struct {
	SessionID string
	Caller    Caller
	Prompt    Prompt
}

// Ref identifies an admitted turn. SessionID is stable across Append calls; TurnID is not.
type Ref struct {
	SessionID string
	TurnID    string
}

// Status describes a turn, independently of the Agent's protocol stop reason.
type Status string

const (
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
	Status Status
	Answer string
	// AnswerComplete is true only after authoritative full-result retrieval. Completed
	// requires true; missing, damaged or over-limit history produces failed + a reason.
	AnswerComplete bool
	StopReason     string
	FailureCode    string
	// Usage is absent for the fixed Node route, which does not relay PromptResponse.usage.
	// Model-only providers may normalize reported counters to ACP Usage. Never invent zeroes.
	Usage json.RawMessage
}

// Turn is the readable record of one admitted prompt. Result is nil only while running.
type Turn struct {
	Ref       Ref
	Caller    Caller
	Prompt    Prompt
	StartedAt time.Time
	EndedAt   *time.Time
	Result    *Result
}

// Session is shared execution metadata; page titles, ordering and deletion remain page-owned.
// EndedAt means the execution session can no longer accept Append, not merely that a turn ended.
type Session struct {
	ID string
	// Target/Executor are nil only for model-only sessions. Selection snapshots never
	// change when the configured catalog changes; no credentials are retained here.
	Target    *Target
	Executor  *Executor
	Model     Model
	CreatedAt time.Time
	EndedAt   *time.Time
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
	EventHistorySnapshot EventKind = "history_snapshot"
	EventTurnEnded       EventKind = "turn_ended"
	EventTextDelta       EventKind = "text_delta"
	EventSessionEnded    EventKind = "session_ended"
)

// Event is immutable after publication. Sequence starts at 1 and increases without gaps within
// SessionID. Ref.TurnID is empty outside a turn. History replaces the entire displayed Node
// projection; it is never appended as ACP chunks. Result is set only on TurnEnded, which does
// not prove session shutdown or Workspace release. Thought/tool text is not part of Answer.
type Event struct {
	Ref        Ref
	Sequence   uint64
	OccurredAt time.Time
	Kind       EventKind
	History    *HistorySnapshot
	// TextDelta carries model-only text without inventing ACP tools or Agent events.
	TextDelta string
	Result    *Result
}

// HistorySnapshot is a full replacement of the available settled/pending Node history.
// Missing ACP chrome/usage is unavailable, not zero. Incomplete concerns process display;
// Result.AnswerComplete independently describes the authoritative final answer.
type HistorySnapshot struct {
	Records    []HistoryRecord `json:"records"`
	Incomplete bool            `json:"incomplete"`
}

// HistoryRecord identifies an upsert position within a Node history, not the session's Event
// cursor. Data is the fixed Node's HistoryLine/preview record, including available _meta and
// tool details, not a raw ACP delta. Preview is provisional and must never form a final result.
type HistoryRecord struct {
	Sequence uint32          `json:"sequence"`
	Data     json.RawMessage `json:"data"`
	Preview  bool            `json:"preview,omitempty"`
}

// Stream replays retained events then follows live ones with no handoff gap. Next returns io.EOF
// only after session_ended is delivered, or immediately if the cursor already acknowledges it.
// turn_ended does not close the stream: later snapshots and subsequent turns remain observable.
// A transport/storage failure is a non-EOF error.
// One goroutine calls Next; Close may run concurrently and must unblock it. Context cancellation
// or Close releases the subscription only: stopping observation must never cancel execution.
type Stream interface {
	Next(context.Context) (Event, error)
	Close() error
}

// Service is the shared execution port. It is a proposed contract, not an available runtime.
// Start/StartModel/Append return after admission; contexts control admission, not execution.
// Within a scope, repeating an identical caller attempt and request returns the original Ref;
// changing its target, executor, session or prompt returns ErrConflict without dispatching again.
// An identity closed by SealAttempt instead returns ErrAdmissionClosed on every admission path.
// This lookup precedes Busy checks. Admission is atomic across callers for a Workspace, not a
// project. ErrWorkspaceBusy means not admitted and will never start later; there is no queue.
// A confirmed rejection cannot admit later. Context errors are conclusive only when admission
// stopped before commit; admission winning the race returns Ref. An ambiguous commit returns
// ErrAdmissionUnknown. Retrying the SAME request may admit it if the previous call did not;
// callers that must not start work use SealAttempt instead. ReadAttempt's ErrNotFound alone
// cannot settle an overlapping or unknown admission. Never retry uncertainty with a new attempt.
type Service interface {
	Start(context.Context, Scope, StartRequest) (Ref, error)
	StartModel(context.Context, Scope, ModelStartRequest) (Ref, error)
	// Append keeps the execution mode/binding; ended sessions return ErrSessionEnded.
	Append(context.Context, Scope, AppendRequest) (Ref, error)

	// Subscribe returns events strictly after afterSequence, replaying from the start at zero.
	// Sequences are session-wide. Workflow/schedule consumers filter TurnID and stop observing
	// on their turn's terminal event, not EOF. An ahead-of-history cursor is invalid.
	Subscribe(ctx context.Context, scope Scope, sessionID string, afterSequence uint64) (Stream, error)

	// Cancel waits for execution-side confirmation and durable terminal publication. It is
	// idempotent for terminal turns and returns their actual result, including a racing success.
	// For an active Node turn it ends the entire Agent session (no turn-only Node cancel),
	// confirms cleanup, then releases the Workspace. A previously terminal turn is a no-op;
	// its still-live session continues to hold the Workspace until End. Model-only cancellation
	// stops the provider request without ending the conversation and holds no Workspace.
	// A deadline or ErrStopUnconfirmed means stopping is NOT confirmed: consumers must not
	// publish cancellation or start a replacement attempt on that basis. The stop request may
	// still complete later; callers reconcile via ReadAttempt or Subscribe.
	Cancel(context.Context, Scope, Ref) (Result, error)

	// End closes only this session, cancelling its active turn and confirming cleanup. It
	// cannot stop a later session sharing the Workspace. Successful End is idempotent and
	// keeps history/replay. Cleanup failure cannot rewrite a persisted Result.
	// A naturally completed turn may publish its Result while its session remains live and
	// holds the Workspace between turns. Only that owner can Append; another Start stays Busy.
	// Execution-side quiescence and cleanup precede Workspace release; a stop timeout never
	// releases it. On restart old live sessions get EndedAt/session_ended and
	// running turns get interrupted/turn_ended before reads open. Clean residue before admission.
	End(context.Context, Scope, string) error
	ReadSession(context.Context, Scope, string) (Session, error)
	// ReadTurns accepts a limit from 1 to 100; an empty cursor starts at the first turn.
	ReadTurns(ctx context.Context, scope Scope, sessionID, cursor string, limit int) (TurnPage, error)
	// ReadAttempt is a pure lookup of admitted turns. ErrNotFound is not proof that an
	// overlapping/unknown request was never admitted, even after the caller restarts.
	ReadAttempt(context.Context, Scope, Caller) (Turn, error)

	// SealAttempt resolves admission without submitting a prompt or dispatching work. It
	// serializes with Start, StartModel and Append using Scope and the complete Caller identity.
	// If admission already committed, it returns AdmissionAccepted and the original Ref,
	// without cancelling or changing that execution; ReadAttempt reads its latest result.
	// Otherwise it durably closes admission for this identity before returning
	// AdmissionNotAdmitted. Every overlapping or later admission of that identity must then
	// return ErrAdmissionClosed. Closing an absent identity creates no Session, Turn or
	// Workspace reservation. The closure survives restart and cannot expire into acceptance.
	//
	// This is a storage mutation, not a read-only query. Only a confirmed transaction outcome
	// permits a successful resolution. Unknown commits, unfinished lock waits/cancellation and
	// unavailable recovery barriers return errors (including ErrAdmissionUnknown/context errors); callers
	// retain their unknown occupancy and may retry SealAttempt with the same identity. They
	// must never interpret an error or zero-value resolution as proof of non-admission.
	// Scope authorization and startup recovery gates are the same as for other operations.
	SealAttempt(context.Context, Scope, Caller) (AdmissionResolution, error)
}
