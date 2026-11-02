import { describe, expect, it } from 'vitest'
import type { Task } from './tasks-api/generated'
import { agentReach, filterTasks, viewOf } from './task-views'

const task = (overrides: Partial<Task>): Task => ({
  id: 'task', tenantId: 'tenant', title: '任务', description: '', projectId: 'project', assignee: null, status: 'pending',
  createdBy: 'someone', createdByName: '某人', version: 1, createdAt: '2026-11-01T00:00:00Z', updatedAt: '2026-11-01T00:00:00Z',
  activeRun: null, ...overrides,
})
const agent = { type: 'agent' as const, id: 'agent-1', displayName: 'OpenCode', identifier: 'official/ora-space.opencode' }
const running = { id: 'run-1', state: 'running' as const, startedBy: 'me', startedByName: '我', executorName: 'OpenCode / Fixture' }

describe('task views', () => {
  const mine = task({ id: 'mine', assignee: { type: 'human', id: 'me', displayName: '我' } })
  const mineDone = task({ id: 'mine-done', status: 'done', assignee: { type: 'human', id: 'me', displayName: '我' } })
  const myReview = task({ id: 'my-review', status: 'review', createdBy: 'me', assignee: agent })
  const busy = task({ id: 'busy', status: 'running', assignee: agent, activeRun: running })
  const squad = task({ id: 'squad', assignee: { type: 'squad', id: 'squad-1', displayName: '产品小队' } })
  const all = [mine, mineDone, myReview, busy, squad]
  const ids = (list: Task[]) => list.map(t => t.id)

  it('scopes one team-wide list', () => {
    expect(ids(filterTasks(all, 'mine', 'me'))).toEqual(['mine', 'mine-done'])
    expect(ids(filterTasks(all, 'todo', 'me'))).toEqual(['mine', 'my-review'])
    expect(ids(filterTasks(all, 'todo', 'me', null, new Set(['squad'])))).toEqual(['mine', 'my-review', 'squad'])
    expect(ids(filterTasks(all, 'all', 'me'))).toEqual(ids(all))
    expect(ids(filterTasks(all, 'all', 'me', 'squad-1'))).toEqual(['squad'])
  })

  it('reads an unknown view as everything', () => {
    expect(viewOf('todo')).toBe('todo')
    expect(viewOf('running')).toBe('all')
    expect(viewOf('nope')).toBe('all')
    expect(viewOf(null)).toBe('all')
  })
})

describe('who may send the Agent a turn', () => {
  it('lets anyone start, and only the starter follow up', () => {
    expect(agentReach(task({ assignee: agent }), 'me').ok).toBe(true)
    expect(agentReach(task({ assignee: agent, activeRun: { ...running, state: 'idle' } }), 'me')).toMatchObject({ ok: true, reason: '作为同一次运行的下一轮发给 Agent。' })
    const other = agentReach(task({ assignee: agent, activeRun: { ...running, state: 'idle', startedBy: 'bob', startedByName: '鲍勃' } }), 'me')
    expect(other.ok).toBe(false)
    expect(other.reason).toMatch(/当前运行由 鲍勃 发起/)
  })

  it('waits for a running turn and refuses closed or unassigned tasks', () => {
    expect(agentReach(task({ assignee: agent, activeRun: running }), 'me').ok).toBe(false)
    expect(agentReach(task({ assignee: agent, status: 'done' }), 'me').ok).toBe(false)
    expect(agentReach(task({}), 'me').ok).toBe(false)
  })
})
