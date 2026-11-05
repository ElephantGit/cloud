import { expect, it } from 'vitest'
import type { Task, TaskRun } from '@/lib/tasks-api/generated'
import { overviewModel, type OverviewSnapshot } from './overview-model'

const now = new Date(2026, 10, 5, 12)
const task = (id: string, status: Task['status'], kind: 'human' | 'squad' = 'human'): Task => ({ id, status, title: id, description: '', tenantId: 'tenant', projectId: 'project', assignee: { type: kind, id: 'owner', displayName: '负责人' }, createdBy: 'user', createdByName: '成员', version: 1, createdAt: now.toISOString(), updatedAt: now.toISOString(), activeRun: null })
const run = (id: string, result: TaskRun['lastResult'], createdAt = now.toISOString()): TaskRun => ({ demoSeed: false, id, taskId: 'a', startedBy: 'user', startedByName: '成员', executorId: 'agent', executorName: '真实 Agent', state: 'ended', lastResult: result, failureCode: '', endReason: 'closed', createdAt, endedAt: now.toISOString() })
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

it('excludes nonexecuting seeds but retains actual fixture-backed runs', () => {
  const data = snapshot([task('a', 'review')])
  const seeded = { ...run('seed', 'completed'), demoSeed: true, executorId: 'opencode:fixture' }
  data.timelines.a = { runs: [seeded], comments: [] }
  const onlySeed = overviewModel(data, now)
  expect(onlySeed.weekRuns).toHaveLength(0)
  expect(onlySeed.outputs).toHaveLength(0)
  expect(onlySeed.counts.review).toBe(1)
  data.timelines.a.runs.push({ ...run('actual-fixture', 'completed'), executorId: 'opencode:fixture' })
  const withActual = overviewModel(data, now)
  expect(withActual.weekRuns.map(run => run.id)).toEqual(['actual-fixture'])
  expect(withActual.outputs.map(item => item.run.id)).toEqual(['actual-fixture'])
})

it('does not count a prior successful turn as success while the next turn runs', () => {
  const data = snapshot([task('a', 'review')])
  data.timelines.a = { runs: [{ ...run('second-turn', 'completed'), state: 'running' }], comments: [] }
  const model = overviewModel(data, now)
  expect(model.weekRuns).toHaveLength(1)
  expect(model.successfulWeekRuns).toHaveLength(0)
  expect(model.outputs).toHaveLength(0)
})

it.each([
  [new Date(2026, 10, 2, 12), new Date(2026, 10, 2)],
  [new Date(2026, 10, 1, 12), new Date(2026, 9, 26)],
])('uses the local Monday boundary across month changes (%s)', (clock, monday) => {
  const data = snapshot([task('a', 'review')])
  data.timelines.a = { runs: [
    run('before', 'completed', new Date(monday.getTime() - 1).toISOString()),
    run('at', 'completed', monday.toISOString()),
    run('now', 'completed', clock.toISOString()),
    run('future', 'completed', new Date(clock.getTime() + 1).toISOString()),
    { ...run('failed', 'failed', clock.toISOString()) },
  ], comments: [] }
  const model = overviewModel(data, clock)
  expect(model.weekRuns.map(run => run.id).sort()).toEqual(['at', 'failed', 'now'])
  expect(model.successfulWeekRuns.map(run => run.id).sort()).toEqual(['at', 'now'])
})
