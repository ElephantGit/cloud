import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { emptyDraft, taskDraft } from './schedule-model'
import { useMockSchedules } from './mock-store'

beforeEach(() => { vi.useFakeTimers(); useMockSchedules.getState().reset() })
afterEach(() => { useMockSchedules.getState().reset(); vi.useRealTimers() })
it('runs paused tasks manually without enabling them, keeping historical target snapshots', () => {
  const store = useMockSchedules.getState()
  store.toggle('summary')
  store.simulate('summary')
  store.simulate('summary')
  const running = useMockSchedules.getState().tasks.find(t => t.id === 'summary')!
  expect(running.runs).toHaveLength(2)
  expect(running.runs[0]?.status).toBe('running')
  store.save({ ...taskDraft(running), projectId: 'website', prompt: '新的指令' }, 'summary')
  vi.advanceTimersByTime(1600)
  const completed = useMockSchedules.getState().tasks.find(t => t.id === 'summary')!
  expect(completed.enabled).toBe(false)
  expect(completed.runs[0]).toMatchObject({ status: 'success', projectName: '产品研发', target: { prompt: '总结今天的研发进展、未解决的问题和下一步计划。请分点列出。' } })
  expect(completed.runs[1]?.status).toBe('success')
})
it('reset cancels pending results and deletes changes', () => {
  const store = useMockSchedules.getState()
  store.simulate('morning', true)
  store.save({ ...emptyDraft(), name: '临时任务', prompt: 'test' })
  store.reset()
  vi.advanceTimersByTime(5000)
  expect(useMockSchedules.getState().tasks).toHaveLength(2)
  expect(useMockSchedules.getState().tasks[0]?.runs).toHaveLength(1)
  expect(useMockSchedules.getState().tasks[0]?.runs[0]?.status).toBe('success')
})
