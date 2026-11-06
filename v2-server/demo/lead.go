//go:build linux

package demo

import (
	"context"
	"fmt"

	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

// B's accounts (users/lead.json). The names match the dashboard mock in
// frontend-v2/src/lib/demo/dashboard.ts, so the recording does not jump between names.
const (
	leadEmail     = "chen.mo@demo.ora-space.dev"
	suRuiEmail    = "su.rui@demo.ora-space.dev"
	zhouHengEmail = "zhou.heng@demo.ora-space.dev"
)

// leadSquad is the squad 陈默 shows first: three people and two Agents.
const leadSquad = "看板小队"

// leadHistory is the work the squad finished or is doing before the recording starts. Only people
// hold these tasks. seedDashboard separately adds one labelled historical Agent task/run; live
// Agent work starts during recording, and the seed does not write discussion comments.
var leadHistory = []struct {
	title, description, assignee, status string
}{
	{"看板按状态分列", "看板按 待处理 / 进行中 / 待验收 / 已完成 分列，列内保持创建顺序。验收：`node --test` 覆盖空看板和未知状态。", suRuiEmail, store.TaskDone},
	{"状态推进只能按顺序", "advance 只允许 待处理 → 进行中 → 待验收 → 已完成，已完成和已取消的任务不能再推进。验收：非法推进抛错，原任务不被修改。", zhouHengEmail, store.TaskDone},
	{"取消任务时保留原状态记录", "取消任务返回新对象，已完成的任务不能取消。验收：补上已完成、已取消两种边界测试。", zhouHengEmail, store.TaskReview},
	{"看板列显示任务数", "每列标题旁显示该列任务数，取消的任务不计入。验收：README 补充用法示例。", suRuiEmail, store.TaskRunning},
}

// seedLead prepares B: 陈默 with his squad, the 「协作看板服务」 project, the 「需求拆分」 workflow
// and a board that already has some history.
func seedLead(ctx context.Context, s *Seeder) error {
	lead, err := s.Scope(ctx, leadEmail)
	if err != nil {
		return err
	}
	members := map[string]execution.Scope{}
	for _, email := range []string{suRuiEmail, zhouHengEmail} {
		if members[email], err = s.Scope(ctx, email); err != nil {
			return err
		}
	}
	project, err := s.Project(ctx, lead, "协作看板服务", "lead")
	if err != nil {
		return err
	}
	if err := seedLeadSquad(ctx, s, lead, members); err != nil {
		return err
	}
	if _, err := SeedDecomposition(ctx, s.WorkflowSeed(lead, project)); err != nil {
		return err
	}
	if _, err := SeedQualityScan(ctx, s.WorkflowSeed(lead, project)); err != nil {
		return err
	}
	for _, h := range leadHistory {
		task, err := s.Store.CreateTask(ctx, lead.TenantID, lead.UserID, store.TaskInput{
			Title: h.title, Description: h.description, ProjectID: project.ID,
			Assignee: &store.TaskAssigneeRef{Type: store.AssigneeHuman, ID: members[h.assignee].UserID},
		})
		if err != nil {
			return fmt.Errorf("task %s: %w", h.title, err)
		}
		// The assignee moves their own task up to 待验收, as they would on the board; only 陈默, who
		// asked for the work, confirms it.
		for _, status := range []string{store.TaskRunning, store.TaskReview, store.TaskDone} {
			if task.Status == h.status {
				break
			}
			actor := members[h.assignee].UserID
			if status == store.TaskDone {
				actor = lead.UserID
			}
			if task, err = s.Store.SetTaskStatus(ctx, lead.TenantID, actor, task.ID, status, task.Version); err != nil {
				return fmt.Errorf("task %s → %s: %w", h.title, status, err)
			}
		}
	}
	return seedDashboard(ctx, s, lead, project)
}

func seedLeadSquad(ctx context.Context, s *Seeder, lead execution.Scope, members map[string]execution.Scope) error {
	squad, err := s.Store.CreateSquad(ctx, lead.TenantID, lead.UserID, store.SquadInput{
		Name:        leadSquad,
		Description: "负责协作看板服务的迭代：人拆需求、验收，Agent 写代码、跑测试。",
		Charter:     "大需求先用「需求拆分」拆成子任务，再分给人或 Agent；Agent 交付的结果由人验收。",
	})
	if err != nil {
		return fmt.Errorf("squad: %w", err)
	}
	agents, err := spaceAgents(ctx, s, lead)
	if err != nil {
		return err
	}
	for _, m := range []store.SquadMemberInput{
		{Type: store.MemberHuman, UserID: members[suRuiEmail].UserID, Role: "前端"},
		{Type: store.MemberHuman, UserID: members[zhouHengEmail].UserID, Role: "后端"},
		{Type: store.MemberAgent, SpaceAgentID: agents["ora-space.opencode"], Role: "编码与测试"},
		{Type: store.MemberAgent, SpaceAgentID: agents["ora-space.claude"], Role: "代码评审"},
	} {
		m.Version = squad.Version
		if squad, err = s.Store.AddSquadMember(ctx, lead.TenantID, lead.UserID, squad.ID, m); err != nil {
			return fmt.Errorf("squad member %s: %w", m.Role, err)
		}
	}
	return nil
}

// spaceAgents maps the demo space's preinstalled Agents by identifier to their space Agent ids.
func spaceAgents(ctx context.Context, s *Seeder, scope execution.Scope) (map[string]string, error) {
	spaces, err := s.Store.TenantSpaces(ctx, scope.TenantID, scope.UserID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, space := range spaces {
		if space.Slug != store.DemoSpaceSlug {
			continue
		}
		plugins, err := s.Store.SpaceAgentPlugins(ctx, scope.TenantID, space.ID, scope.UserID)
		if err != nil {
			return nil, err
		}
		for _, p := range plugins {
			out[p.Identifier] = p.ID
		}
	}
	for _, a := range store.DemoAgents {
		if out[a.Identifier] == "" {
			return nil, fmt.Errorf("demo space has no %s Agent", a.Name)
		}
	}
	return out, nil
}
