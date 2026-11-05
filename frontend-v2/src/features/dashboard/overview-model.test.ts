import { expect, it } from 'vitest'
import type { Task, TaskRun } from '@/lib/tasks-api/generated'
import { overviewModel, type OverviewSnapshot } from './overview-model'

const now = new Date(2026, 10, 5, 12)
const task = (id: string, status: Task['status'], kind: 'human' | 'squad' = 'human'): Task => ({ id, status, title: id, description: '', tenantId: 'tenant', projectId: 'project', assignee: { type: kind, id: 'owner', displayName: '负责人' }, createdBy: 'user', createdByName: '成员', version: 1, createdAt: now.toISOString(), updatedAt: now.toISOString(), activeRun: null })
const run = (id: string, result: TaskRun['lastResult'], createdAt = now.toISOString()): TaskRun => ({ id, taskId: 'a', startedBy: 'user', startedByName: '成员', executorId: 'agent', executorName: '真实 Agent', state: 'ended', lastResult: result, failureCode: '', endReason: 'closed', createdAt, endedAt: now.toISOString() })
const snapshot = (tasks: Task[]): OverviewSnapshot => ({ tasks, timelines: {}, fetchedAt: now.toISOString() })

it('counts pending and groups squad work once without counting cancelled work', () => {
  const model = overviewModel(snapshot([task('a', 'pending'), task('b', 'done'), task('c', 'cancelled'), task('d', 'running', 'squad')]), now)
  expect(model.counts).toEqual({ pending: 1, running: 1, review: 0, done: 1, cancelled: 1 })
  expect(model.owners).toHaveLength(2)
  expect(model.owners[0]).toMatchObject({ pending: 1, done: 1 })
  expect(model.owners[1]).toMatchObject({ kind: 'squad', running: 1 })
})

it('only presents current deliverables whose latest run succeeded', () => {
  const data = snapshot([task('a', 'review'), task('b', 'done')])
  data.timelines.a = { runs: [run('old', 'completed', new Date(2026, 10, 4).toISOString()), run('latest', 'failed')], comments: [] }
  data.timelines.b = { runs: [run('ok', 'completed')], comments: [] }
  expect(overviewModel(data, now).outputs.map(item => item.task.id)).toEqual(['b'])
})

it('does not turn last update time into a completion event or invent empty data', () => {
  expect(overviewModel(snapshot([]), now).weekRuns).toEqual([])
  const model = overviewModel(snapshot([task('pending', 'pending')]), now)
  expect(model.days.at(-1)?.count).toBe(1)
  expect(model.counts.done).toBe(0)
  expect(model.outputs).toEqual([])
})
