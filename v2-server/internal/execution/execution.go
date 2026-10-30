// Package execution defines the in-process Agent execution boundary shared by v2 pages.
// It contains no Node driver, database implementation, or HTTP routes.
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Stable errors distinguish a rejected call from an accepted turn's result.
var (
	ErrUnavailable     = errors.New("execution unavailable")
	ErrNotFound        = errors.New("execution not found")
	ErrInvalidRequest  = errors.New("invalid execution request")
	ErrWorkspaceBusy   = errors.New("workspace busy")
	ErrConflict        = errors.New("execution identity conflict")
	ErrUnsupported     = errors.New("execution capability unsupported")
	ErrStopUnconfirmed = errors.New("execution stop not confirmed")
	ErrSessionEnded    = errors.New("execution session ended")
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
// turn, including a chat turn. An explicit rerun after termination uses a NEW AttemptID.
// NodeID and Rounds apply only to workflow callers; all fields participate in identity.
type Caller struct {
	Kind      CallerKind
	ID        string
	NodeID    string
	Rounds    []Round
	AttemptID string
}

// Target selects a project. Start creates a conversation directory from its stored repository;
// workflow callers share one directory within the same run. Callers never select a directory.
type Target struct {
	ProjectID string
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

// Ref identifies an accepted turn. SessionID is stable across Append calls; TurnID is not.
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

// Turn is the readable record of one accepted prompt. Result is nil only while running.
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
	Target   *Target
	Executor *Executor
	Model    Model
	// WorkspaceID is a server-generated directory identity, empty for model-only sessions.
	// Sessions belonging to the same workflow run share this identity.
	WorkspaceID string
	CreatedAt   time.Time
	EndedAt     *time.Time
}

// TurnPage lists accepted turns in stable creation order. NextCursor is empty at the current
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
// not prove session shutdown. Thought/tool text is not part of Answer.
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

// Service is the shared execution port. This package defines the contract, not its runtime.
// Start/StartModel/Append record a turn under a process-local mutex and a database write before
// returning its Ref. Their contexts control this call, not the subsequent execution lifetime.
// Within a scope, repeating an identical caller attempt and request returns the original Ref;
// changing its target, executor, session or prompt returns ErrConflict without dispatching again.
// ErrWorkspaceBusy means the previous turn in this session or workflow run is still active.
// Different conversations and workflow runs do not compete for a directory, even in one project.
// Pages own their queues; this service does not enqueue a rejected request.
type Service interface {
	// Start clones a directory for this conversation or reuses this workflow run's directory.
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
	// It stops only the identified turn, leaving the conversation available for Append. A
	// terminal turn is a no-op and cannot stop a later turn. A deadline or ErrStopUnconfirmed
	// does not confirm cancellation; callers observe the actual result through reads/events.
	Cancel(context.Context, Scope, Ref) (Result, error)

	// End closes only this session, stopping its active turn and waiting for confirmation.
	// Successful End is idempotent, keeps history and leaves the directory on disk. It must
	// not stop other sessions on the same project's Node or rewrite a persisted Result.
	// Restart ends old live sessions and marks running turns interrupted before serving requests;
	// it does not replay prompts. Append to any ended session returns ErrSessionEnded.
	End(context.Context, Scope, string) error
	ReadSession(context.Context, Scope, string) (Session, error)
	// ReadTurns accepts a limit from 1 to 100; an empty cursor starts at the first turn.
	ReadTurns(ctx context.Context, scope Scope, sessionID, cursor string, limit int) (TurnPage, error)
	// ReadAttempt looks up a recorded turn by its complete caller identity without dispatching.
	ReadAttempt(context.Context, Scope, Caller) (Turn, error)
}
