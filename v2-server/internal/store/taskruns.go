package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ScheduledTaskStartPrefix cannot occur in a client Idempotency-Key HTTP header.
const ScheduledTaskStartPrefix = "\nschedule:"

// Task run end reasons; the empty reason is a live run.
const (
	TaskRunStopped     = "stopped"
	TaskRunReplaced    = "replaced"
	TaskRunClosed      = "closed"
	TaskRunInterrupted = "interrupted"
)

// Task run states as pages read them: a turn is running, the session waits for the next turn, or
// the session can take no more turns.
const (
	TaskRunRunning = "running"
	TaskRunIdle    = "idle"
	TaskRunEnded   = "ended"
)

var (
	// ErrRunActive reports a change that a running Agent turn forbids.
	ErrRunActive = errors.New("task run turn active")
	// ErrInvalidMention reports an @ target that is not a live member or installed Agent.
	ErrInvalidMention = errors.New("invalid mention")
)

// TaskRun is one execution session of a task's Agent. The whole workspace reads it; the session
// belongs to StartedBy, the only person who can send it another turn.
type TaskRun struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"taskId"`
	TenantID      string     `json:"-"`
	StartedBy     string     `json:"startedBy"`
	StartedByName string     `json:"startedByName"`
	Scheduled     bool       `json:"scheduled,omitempty"`
	StartKey      string     `json:"-"`
	ExecutorID    string     `json:"executorId"`
	ExecutorName  string     `json:"executorName"`
	SessionID     string     `json:"-"`
	LastTurnID    string     `json:"-"`
	State         string     `json:"state"`
	LastResult    string     `json:"lastResult"`
	FailureCode   string     `json:"failureCode"`
	EndReason     string     `json:"endReason"`
	CreatedAt     time.Time  `json:"createdAt"`
	EndedAt       *time.Time `json:"endedAt"`
	turnActive    bool
}

// Live reports whether the run's session can still take turns.
func (r TaskRun) Live() bool { return r.EndedAt == nil }

// TurnActive reports whether one of the run's turns is still running.
func (r TaskRun) TurnActive() bool { return r.EndedAt == nil && r.turnActive }

// TaskMention is one typed @ target of a comment. The name is resolved when the comment is written.
type TaskMention struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// TaskComment is a person's entry in a task's timeline. RunID is set when it was sent to the Agent.
type TaskComment struct {
	ID         string        `json:"id"`
	TaskID     string        `json:"taskId"`
	AuthorID   string        `json:"authorId"`
	AuthorName string        `json:"authorName"`
	Body       string        `json:"body"`
	Mentions   []TaskMention `json:"mentions"`
	RunID      string        `json:"runId"`
	CreatedAt  time.Time     `json:"createdAt"`
}

const taskRunColumns = `r.id, r.task_id, r.tenant_id::text, r.started_by::text, u.display_name, r.start_key,
	r.executor_id, r.executor_name, COALESCE(r.session_id, ''), r.last_turn_id, r.turn_active,
	r.last_result, r.failure_code, r.end_reason, r.created_at, r.ended_at`

const taskRunJoins = `FROM v2_task_runs r JOIN users u ON u.id = r.started_by`

func scanTaskRun(row pgx.Row) (TaskRun, error) {
	var r TaskRun
	e := row.Scan(&r.ID, &r.TaskID, &r.TenantID, &r.StartedBy, &r.StartedByName, &r.StartKey,
		&r.ExecutorID, &r.ExecutorName, &r.SessionID, &r.LastTurnID, &r.turnActive,
		&r.LastResult, &r.FailureCode, &r.EndReason, &r.CreatedAt, &r.EndedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if e != nil {
		return r, e
	}
	r.Scheduled = strings.HasPrefix(r.StartKey, ScheduledTaskStartPrefix)
	switch {
	case r.EndedAt != nil:
		r.State = TaskRunEnded
	case r.turnActive:
		r.State = TaskRunRunning
	default:
		r.State = TaskRunIdle
	}
	return r, nil
}

func taskRuns(ctx context.Context, q dbTX, where string, args ...any) ([]TaskRun, error) {
	rows, e := q.Query(ctx, `SELECT `+taskRunColumns+` `+taskRunJoins+` WHERE `+where+` ORDER BY r.created_at, r.id`, args...)
	if e != nil {
		return nil, fmt.Errorf("list task runs: %w", e)
	}
	defer rows.Close()
	out := []TaskRun{}
	for rows.Next() {
		r, e := scanTaskRun(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TaskTimeline returns a task's runs and comments, oldest first, to a current workspace member.
func (s *Store) TaskTimeline(ctx context.Context, tenant, caller, taskID string) ([]TaskRun, []TaskComment, error) {
	if _, e := s.Task(ctx, tenant, caller, taskID); e != nil {
		return nil, nil, e
	}
	runs, e := taskRuns(ctx, s.pool, `r.tenant_id = $1 AND r.task_id = $2`, tenant, taskID)
	if e != nil {
		return nil, nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT c.id, c.task_id, c.author_id::text, u.display_name, c.body, c.mentions,
			COALESCE(c.run_id::text, ''), c.created_at
		FROM v2_task_comments c JOIN users u ON u.id = c.author_id
		WHERE c.tenant_id = $1 AND c.task_id = $2 ORDER BY c.created_at, c.id`, tenant, taskID)
	if e != nil {
		return nil, nil, fmt.Errorf("list task comments: %w", e)
	}
	defer rows.Close()
	comments := []TaskComment{}
	for rows.Next() {
		c, e := scanTaskComment(rows)
		if e != nil {
			return nil, nil, e
		}
		comments = append(comments, c)
	}
	return runs, comments, rows.Err()
}

func scanTaskComment(row pgx.Row) (TaskComment, error) {
	var c TaskComment
	var mentions []byte
	if e := row.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.AuthorName, &c.Body, &mentions, &c.RunID, &c.CreatedAt); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, e
	}
	c.Mentions = []TaskMention{}
	if e := json.Unmarshal(mentions, &c.Mentions); e != nil {
		return c, fmt.Errorf("decode comment mentions: %w", e)
	}
	return c, nil
}

// TaskRun returns one run of the tenant's task without a membership check; callers check first.
func (s *Store) TaskRun(ctx context.Context, tenant, taskID, runID string) (TaskRun, error) {
	return scanTaskRun(s.pool.QueryRow(ctx, `SELECT `+taskRunColumns+` `+taskRunJoins+`
		WHERE r.tenant_id = $1 AND r.task_id = $2 AND r.id = $3`, tenant, taskID, runID))
}

// LiveTaskRun returns the task's live run, if any.
func (s *Store) LiveTaskRun(ctx context.Context, tenant, taskID string) (TaskRun, bool, error) {
	r, e := scanTaskRun(s.pool.QueryRow(ctx, `SELECT `+taskRunColumns+` `+taskRunJoins+`
		WHERE r.tenant_id = $1 AND r.task_id = $2 AND r.ended_at IS NULL`, tenant, taskID))
	if errors.Is(e, ErrNotFound) {
		return r, false, nil
	}
	return r, e == nil, e
}

// TaskRunByKey finds the run a person's start request with this key created.
func (s *Store) TaskRunByKey(ctx context.Context, tenant, taskID, user, key string) (TaskRun, error) {
	return scanTaskRun(s.pool.QueryRow(ctx, `SELECT `+taskRunColumns+` `+taskRunJoins+`
		WHERE r.tenant_id = $1 AND r.task_id = $2 AND r.started_by = $3 AND r.start_key = $4`, tenant, taskID, user, key))
}

// NewTaskRun is a run about to ask the execution service for its first turn.
type NewTaskRun struct {
	ExpectedVersion                                                 int64
	TenantID, TaskID, StartedBy, StartKey, ExecutorID, ExecutorName string
}

// CreateTaskRun records a run before its first turn starts. Another live run of the task returns
// ErrRunActive: the caller ends it first.
func (s *Store) CreateTaskRun(ctx context.Context, in NewTaskRun) (TaskRun, error) {
	var id string
	query := `INSERT INTO v2_task_runs (task_id,tenant_id,started_by,start_key,executor_id,executor_name) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
	args := []any{in.TaskID, in.TenantID, in.StartedBy, in.StartKey, in.ExecutorID, in.ExecutorName}
	if in.ExpectedVersion != 0 {
		// Reserve against the version whose assignment/status was checked. FOR UPDATE
		// serializes this admission with reassignment and manual state transitions.
		query = `INSERT INTO v2_task_runs (task_id,tenant_id,started_by,start_key,executor_id,executor_name)
 SELECT $1::uuid,$2::uuid,$3::uuid,$4,$5,$6 FROM
 (SELECT id FROM v2_tasks WHERE id=$1 AND tenant_id=$2 AND version=$7 FOR UPDATE) eligible RETURNING id`
		args = append(args, in.ExpectedVersion)
	}
	e := s.pool.QueryRow(ctx, query, args...).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return TaskRun{}, ErrConflict
	}
	var pg *pgconn.PgError
	if errors.As(e, &pg) && pg.Code == "23505" {
		return TaskRun{}, ErrRunActive
	}
	if e != nil {
		return TaskRun{}, fmt.Errorf("insert task run: %w", e)
	}
	return s.TaskRun(ctx, in.TenantID, in.TaskID, id)
}

// DeleteTaskRun removes a run whose first turn the execution service refused.
func (s *Store) DeleteTaskRun(ctx context.Context, id string) error {
	_, e := s.pool.Exec(ctx, `DELETE FROM v2_task_runs WHERE id = $1 AND session_id IS NULL`, id)
	return e
}

// RecordTaskTurn notes an accepted turn: the run's session and its now running last turn.
func (s *Store) RecordTaskTurn(ctx context.Context, runID, sessionID, turnID string) error {
	tag, e := s.pool.Exec(ctx, `
		UPDATE v2_task_runs SET session_id = $2, last_turn_id = $3, turn_active = (ended_at IS NULL),
            last_result = CASE WHEN ended_at IS NULL THEN '' ELSE last_result END,
            failure_code = CASE WHEN ended_at IS NULL THEN '' ELSE failure_code END
		WHERE id = $1 AND (session_id IS NULL OR session_id = $2)`, runID, sessionID, turnID)
	if e != nil {
		return fmt.Errorf("record task turn: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SettleTaskTurn records a turn's result once; it reports false when the turn was already settled
// or is no longer the run's latest.
func (s *Store) SettleTaskTurn(ctx context.Context, runID, turnID, result, failureCode string) (bool, error) {
	tag, e := s.pool.Exec(ctx, `
		UPDATE v2_task_runs SET turn_active = false, last_result = $3, failure_code = $4
		WHERE id = $1 AND last_turn_id = $2 AND turn_active`, runID, turnID, result, failureCode)
	if e != nil {
		return false, fmt.Errorf("settle task turn: %w", e)
	}
	return tag.RowsAffected() == 1, nil
}

// EndTaskRun records that the run's session can take no more turns. A reason already written wins,
// so a stop is not later reported as an interruption.
func (s *Store) EndTaskRun(ctx context.Context, runID, reason string) error {
	_, e := s.pool.Exec(ctx, `
		UPDATE v2_task_runs SET ended_at = now(), turn_active = false,
			end_reason = CASE WHEN end_reason = '' THEN $2 ELSE end_reason END
		WHERE id = $1 AND ended_at IS NULL`, runID, reason)
	if e != nil {
		return fmt.Errorf("end task run: %w", e)
	}
	return nil
}

// MarkTaskRunEnding writes the reason before the session is ended, so a page reading the run on
// session_ended already sees why. A failed end withdraws it with ClearTaskRunEnding.
func (s *Store) MarkTaskRunEnding(ctx context.Context, runID, reason string) error {
	_, e := s.pool.Exec(ctx, `UPDATE v2_task_runs SET end_reason = $2 WHERE id = $1 AND ended_at IS NULL`, runID, reason)
	return e
}

// ClearTaskRunEnding withdraws a reason whose end did not happen.
func (s *Store) ClearTaskRunEnding(ctx context.Context, runID string) error {
	_, e := s.pool.Exec(ctx, `UPDATE v2_task_runs SET end_reason = '' WHERE id = $1 AND ended_at IS NULL`, runID)
	return e
}

// InterruptTaskRuns ends every run a previous process left live and returns those whose turn was
// still running, so their tasks can leave 进行中.
func (s *Store) InterruptTaskRuns(ctx context.Context) ([]TaskRun, error) {
	var out []TaskRun
	e := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		live, e := taskRuns(ctx, tx, `r.ended_at IS NULL`)
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `
			UPDATE v2_task_runs SET ended_at = now(), end_reason = 'interrupted',
				last_result = CASE WHEN turn_active THEN 'interrupted' ELSE last_result END, turn_active = false
			WHERE ended_at IS NULL`); e != nil {
			return fmt.Errorf("interrupt task runs: %w", e)
		}
		for _, r := range live {
			if r.turnActive {
				out = append(out, r)
			}
		}
		return nil
	})
	return out, e
}

// NewTaskComment is a comment to record; Mentions carry typed ids only.
type NewTaskComment struct {
	TenantID, TaskID, AuthorID, CreateKey, Body string
	Mentions                                    []TaskMention
	RunID, TurnID                               string
}

// TaskCommentByKey finds the comment a person's request with this key created.
func (s *Store) TaskCommentByKey(ctx context.Context, tenant, taskID, author, key string) (TaskComment, error) {
	return scanTaskComment(s.pool.QueryRow(ctx, `
		SELECT c.id, c.task_id, c.author_id::text, u.display_name, c.body, c.mentions,
			COALESCE(c.run_id::text, ''), c.created_at
		FROM v2_task_comments c JOIN users u ON u.id = c.author_id
		WHERE c.tenant_id = $1 AND c.task_id = $2 AND c.author_id = $3 AND c.create_key = $4`, tenant, taskID, author, key))
}

// ResolveTaskMentions checks each @ target against the workspace and fills in its current name.
func (s *Store) ResolveTaskMentions(ctx context.Context, tenant string, in []TaskMention) ([]TaskMention, error) {
	out := make([]TaskMention, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		key := m.Type + "/" + m.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		var name string
		var e error
		switch m.Type {
		case "user":
			if e = requireActiveUser(ctx, s.pool, tenant, m.ID); e == nil {
				e = s.pool.QueryRow(ctx, `SELECT display_name FROM users WHERE id = $1`, m.ID).Scan(&name)
			}
		case "agent":
			if e = requireActiveAgent(ctx, s.pool, tenant, m.ID); e == nil {
				e = s.pool.QueryRow(ctx, `SELECT name FROM v2_space_agents WHERE id = $1`, m.ID).Scan(&name)
			}
		default:
			return nil, ErrInvalidMention
		}
		if errors.Is(e, ErrUserUnavailable) || errors.Is(e, ErrAgentUnavailable) {
			return nil, ErrInvalidMention
		}
		if e != nil {
			return nil, fmt.Errorf("resolve mention: %w", e)
		}
		out = append(out, TaskMention{Type: m.Type, ID: m.ID, DisplayName: name})
	}
	return out, nil
}

// CreateTaskComment records a comment once per key; a retry returns the comment it made.
func (s *Store) CreateTaskComment(ctx context.Context, in NewTaskComment) (TaskComment, error) {
	mentions, e := json.Marshal(in.Mentions)
	if e != nil {
		return TaskComment{}, e
	}
	if in.Mentions == nil {
		mentions = []byte("[]")
	}
	if _, e := s.pool.Exec(ctx, `
		INSERT INTO v2_task_comments (task_id, tenant_id, author_id, create_key, body, mentions, run_id, turn_id)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid, $8)
		ON CONFLICT (task_id, author_id, create_key) DO NOTHING`,
		in.TaskID, in.TenantID, in.AuthorID, in.CreateKey, in.Body, mentions, in.RunID, in.TurnID); e != nil {
		return TaskComment{}, fmt.Errorf("insert task comment: %w", e)
	}
	return s.TaskCommentByKey(ctx, in.TenantID, in.TaskID, in.AuthorID, in.CreateKey)
}

// TaskAgentIdentifier returns the plugin identity (namespace/identifier) of a Space Agent.
func (s *Store) TaskAgentIdentifier(ctx context.Context, tenant, agentID string) (string, error) {
	var id string
	e := s.pool.QueryRow(ctx, `SELECT source_namespace || '/' || identifier FROM v2_space_agents
		WHERE id = $1 AND tenant_id = $2`, agentID, tenant).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, e
}

// CollaborationAgents lists the workspace's installed, system-provided Space Agents.
func (s *Store) CollaborationAgents(ctx context.Context, tenant, caller string) ([]TaskAssignee, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT id::text, name, source_namespace || '/' || identifier FROM v2_space_agents
		WHERE tenant_id = $1 AND kind = 'agent' AND status = 'active' AND observed_state = 'installed'
			AND requested_by_user_id IS NULL
		ORDER BY name, id`, tenant)
	if e != nil {
		return nil, fmt.Errorf("list collaboration agents: %w", e)
	}
	defer rows.Close()
	out := []TaskAssignee{}
	for rows.Next() {
		a := TaskAssignee{Type: AssigneeAgent}
		if e := rows.Scan(&a.ID, &a.DisplayName, &a.Identifier); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PendingMention is the latest comment that @-mentioned someone on an open task they have not
// commented on since. It is how a mention reaches a person: the task waits on them until they reply.
type PendingMention struct {
	TaskID     string    `json:"taskId"`
	CommentID  string    `json:"commentId"`
	AuthorID   string    `json:"authorId"`
	AuthorName string    `json:"authorName"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PendingMentions lists, one per task, the mentions of the caller still waiting on their reply.
// A mention of oneself, and mentions on done or cancelled tasks, wait on nobody.
func (s *Store) PendingMentions(ctx context.Context, tenant, caller string) ([]PendingMention, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT DISTINCT ON (c.task_id) c.task_id, c.id, c.author_id::text, u.display_name, c.created_at
		FROM v2_task_comments c
		JOIN v2_tasks t ON t.id = c.task_id
		JOIN users u ON u.id = c.author_id
		WHERE c.tenant_id = $1 AND t.status NOT IN ('done', 'cancelled') AND c.author_id <> $2::uuid
			AND c.mentions @> jsonb_build_array(jsonb_build_object('type', 'user', 'id', $2::text))
			AND NOT EXISTS (SELECT 1 FROM v2_task_comments r
				WHERE r.task_id = c.task_id AND r.author_id = $2::uuid AND r.created_at > c.created_at)
		ORDER BY c.task_id, c.created_at DESC, c.id DESC`, tenant, caller)
	if e != nil {
		return nil, fmt.Errorf("list pending mentions: %w", e)
	}
	defer rows.Close()
	out := []PendingMention{}
	for rows.Next() {
		var m PendingMention
		if e := rows.Scan(&m.TaskID, &m.CommentID, &m.AuthorID, &m.AuthorName, &m.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
