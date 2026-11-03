package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
)

// DashboardSession exposes safe frozen metadata without provider configuration or host paths.
type DashboardSession struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	ProjectID   string     `json:"projectId"`
	ProjectName string     `json:"projectName"`
	Agent       string     `json:"agent"`
	Model       string     `json:"model"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Href        string     `json:"href"`
	CreatedAt   time.Time  `json:"createdAt"`
	EndedAt     *time.Time `json:"endedAt"`
}

// DashboardFilter binds pagination to the complete selection and authenticated workspace.
type DashboardFilter struct{ Project, Agent, Status, From, To, After string }

// Only a current tenant member can read an Agent session bound to its original task run.
const dashboardRows = ` FROM v2_execution_sessions s
 JOIN v2_task_runs tr ON tr.session_id=s.id AND tr.tenant_id=s.tenant_id AND tr.started_by=s.user_id
 JOIN v2_tasks task ON task.id=tr.task_id AND task.tenant_id=s.tenant_id
 JOIN v2_projects p ON p.id=task.project_id AND p.tenant_id=s.tenant_id
 JOIN memberships m ON m.tenant_id=s.tenant_id AND m.user_id=$2::uuid
 JOIN LATERAL(SELECT caller FROM v2_execution_turns WHERE session_id=s.id ORDER BY position LIMIT 1) f
   ON f.caller->>'Kind'='task' AND f.caller->>'ID'=tr.id::text
 JOIN LATERAL(SELECT data FROM v2_execution_turns WHERE session_id=s.id ORDER BY position DESC LIMIT 1) l ON true
 WHERE s.tenant_id=$1::uuid AND s.data->'Executor'<>'null'::jsonb
 AND NOT starts_with(tr.start_key, E'\\nschedule:')`

const dashboardColumns = `SELECT s.id,task.title,p.id,p.data->>'Name',
 coalesce(s.data->'Executor'->>'AgentID',''),coalesce(s.data->'Model'->>'ID',''),
 CASE WHEN l.data->'Result' IS NULL OR l.data->'Result'='null'::jsonb THEN 'running' ELSE l.data->'Result'->>'Status' END,
 'task','/collaboration/tasks/'||task.id::text,(s.data->>'CreatedAt')::timestamptz,s.ended_at`

type dashboardCursor struct {
	Tenant, User, Selection, ID string
	At                          time.Time
}

func dashboardSelection(f DashboardFilter) string {
	return fmt.Sprintf("%q", []string{f.Project, f.Agent, f.Status, f.From, f.To})
}

// DashboardSessions lists newest-first admitted Agent sessions without triggering execution.
func (s *Store) DashboardSessions(ctx context.Context, scope execution.Scope, f DashboardFilter) ([]DashboardSession, string, error) {
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return nil, "", err
	}
	var anchor dashboardCursor
	if f.After != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.After)
		if err != nil || json.Unmarshal(raw, &anchor) != nil || anchor.Tenant != scope.TenantID || anchor.User != scope.UserID || anchor.Selection != dashboardSelection(f) || anchor.ID == "" {
			return nil, "", execution.ErrInvalidRequest
		}
	}
	rows, err := s.pool.Query(ctx, dashboardColumns+dashboardRows+`
 AND ($3='' OR p.id=$3) AND ($4='' OR s.data->'Executor'->>'AgentID'=$4)
 AND ($5='' OR CASE WHEN l.data->'Result' IS NULL OR l.data->'Result'='null'::jsonb THEN 'running' ELSE l.data->'Result'->>'Status' END=$5)
 AND ($6='' OR (s.data->>'CreatedAt')::timestamptz>=nullif($6,'')::timestamptz)
 AND ($7='' OR (s.data->>'CreatedAt')::timestamptz<nullif($7,'')::timestamptz)
 AND ($8='' OR ((s.data->>'CreatedAt')::timestamptz,s.id)<($9::timestamptz,$8))
 ORDER BY (s.data->>'CreatedAt')::timestamptz DESC,s.id DESC LIMIT 51`, scope.TenantID, scope.UserID, f.Project, f.Agent, f.Status, f.From, f.To, anchor.ID, anchor.At)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []DashboardSession{}
	for rows.Next() {
		var item DashboardSession
		if err := scanDashboard(rows, &item); err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > 50 {
		items = items[:50]
		last := items[49]
		raw, _ := json.Marshal(dashboardCursor{scope.TenantID, scope.UserID, dashboardSelection(f), last.ID, last.CreatedAt})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return nil, "", err
	}
	return items, next, nil
}

type dashboardScanner interface{ Scan(...any) error }

func scanDashboard(row dashboardScanner, item *DashboardSession) error {
	return row.Scan(&item.ID, &item.Title, &item.ProjectID, &item.ProjectName, &item.Agent, &item.Model, &item.Status, &item.Source, &item.Href, &item.CreatedAt, &item.EndedAt)
}
func (s *Store) dashboardSession(ctx context.Context, scope execution.Scope, id string) (DashboardSession, error) {
	var item DashboardSession
	if err := s.CheckExecutionScope(ctx, scope); err != nil {
		return item, err
	}
	err := scanDashboard(s.pool.QueryRow(ctx, dashboardColumns+dashboardRows+` AND s.id=$3`, scope.TenantID, scope.UserID, id), &item)
	return item, executionError(err)
}

// DashboardMetadata allows statistics to remain available even when history is damaged.
func (s *Store) DashboardMetadata(ctx context.Context, scope execution.Scope, id string) (DashboardSession, error) {
	return s.dashboardSession(ctx, scope, id)
}

// DashboardHistory reads a bounded persisted settled snapshot. It is never a Revision archive.
func (s *Store) DashboardHistory(ctx context.Context, scope execution.Scope, id string) ([]json.RawMessage, bool, error) {
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, false, err
	}
	var raw []byte
	var size int
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN octet_length(data::text)<=33554432 THEN data ELSE NULL END,octet_length(data::text) FROM v2_execution_events WHERE session_id=$1 AND data->>'Kind'='history_snapshot' ORDER BY sequence DESC LIMIT 1`, id).Scan(&raw, &size)
	if err != nil {
		if executionError(err) == execution.ErrNotFound {
			_, readErr := s.dashboardSession(ctx, scope, id)
			return []json.RawMessage{}, true, readErr
		}
		return nil, false, err
	}
	if size > 32<<20 {
		return nil, false, execution.ErrUnsupported
	}
	var event execution.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, false, err
	}
	if event.History == nil || len(event.History.Records) > 100000 {
		return nil, false, execution.ErrUnsupported
	}
	records := []json.RawMessage{}
	for _, record := range event.History.Records {
		if record.Preview {
			event.History.Incomplete = true
		} else {
			if !json.Valid(record.Data) {
				return nil, false, execution.ErrUnavailable
			}
			records = append(records, record.Data)
		}
	}
	if _, err := s.dashboardSession(ctx, scope, id); err != nil {
		return nil, false, err
	}
	return records, event.History.Incomplete, nil
}
