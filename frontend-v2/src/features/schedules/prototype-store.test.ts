import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { emptyDraft, taskDraft } from './prototype-model'
import { usePrototype } from './prototype-store'

beforeEach(() => { vi.useFakeTimers(); usePrototype.getState().reset() })
afterEach(() => { usePrototype.getState().reset(); vi.useRealTimers() })
it('runs paused tasks manually without enabling them, keeping historical target snapshots', () => {
  const store = usePrototype.getState()
  store.simulate('report')
  store.simulate('report')
  const running = usePrototype.getState().tasks.find(t => t.id === 'report')!
  expect(running.runs).toHaveLength(2)
  expect(running.runs[0]?.status).toBe('running')
  store.save({ ...taskDraft(running), projectId: 'website', inputs: { audience: '新团队' } }, 'report')
  vi.advanceTimersByTime(1600)
  const completed = usePrototype.getState().tasks.find(t => t.id === 'report')!
  expect(completed.enabled).toBe(false)
  expect(completed.runs[0]).toMatchObject({ status: 'success', projectName: '产品研发', target: { inputs: { audience: '产品与研发团队' } } })
  expect(completed.runs[1]?.status).toBe('failed')
})
it('reset cancels pending results and deletes changes', () => {
  const store = usePrototype.getState()
  store.simulate('morning', true)
  store.save({ ...emptyDraft(), name: '临时任务', prompt: 'test' })
  store.reset()
  vi.advanceTimersByTime(5000)
  expect(usePrototype.getState().tasks).toHaveLength(4)
  expect(usePrototype.getState().tasks[0]?.runs).toHaveLength(1)
  expect(usePrototype.getState().tasks[0]?.runs[0]?.status).toBe('success')
})
