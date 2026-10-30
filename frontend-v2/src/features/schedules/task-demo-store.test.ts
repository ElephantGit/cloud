import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { demoSources, useTaskScheduleDemo } from './task-demo-store'
import { emptyDraft, taskDraft } from './schedule-model'
import { useChatStore } from '../chat/store'

const scope = 'member:tenant'
beforeEach(() => {
  vi.useFakeTimers()
  useTaskScheduleDemo.getState().reset()
})
afterEach(() => {
  useTaskScheduleDemo.getState().reset()
  vi.useRealTimers()
})
function createTask() {
  return useTaskScheduleDemo
    .getState()
    .save(scope, { ...emptyDraft(), kind: 'task', name: '周期检查', sourceTaskId: 'CLOUD-128', supplementalInstruction: '检查链接' })!
}

it('uses latest task content without touching chat and preserves snapshots and pending runs through edits', () => {
  const chat = useChatStore.getState()
  const id = createTask()
  const store = useTaskScheduleDemo.getState()
  store.run(scope, id)
  store.run(scope, id)
  expect(useTaskScheduleDemo.getState().schedules[scope][0].runs).toHaveLength(1)
  const pending = useTaskScheduleDemo.getState().schedules[scope][0]
  store.save(scope, { ...taskDraft(pending), supplementalInstruction: '新要求' }, id)
  vi.runAllTimers()
  useTaskScheduleDemo.setState({
    sources: demoSources.map((s, i) =>
      i ? s : { ...s, description: '最新内容', projectId: 'new', projectName: '新项目', status: '已完成' },
    ),
  })
  store.run(scope, id, true)
  vi.runAllTimers()
  const runs = useTaskScheduleDemo.getState().schedules[scope][0].runs
  expect(runs[0]).toMatchObject({
    status: 'failed',
    source: { description: '最新内容', projectName: '新项目', status: '已完成' },
    target: { supplementalInstruction: '新要求' },
  })
  expect(runs[1]).toMatchObject({ status: 'success', source: demoSources[0], target: { supplementalInstruction: '检查链接' } })
  expect(useChatStore.getState()).toBe(chat)
})

it('isolates user/tenant state, supports manual runs while paused and blocks missing sources', () => {
  const id = createTask()
  const store = useTaskScheduleDemo.getState()
  store.run('another:tenant', id)
  store.toggle('another:tenant', id)
  expect(useTaskScheduleDemo.getState().schedules[scope][0]).toMatchObject({ enabled: true, runs: [] })
  store.toggle(scope, id)
  store.run(scope, id)
  vi.runAllTimers()
  expect(useTaskScheduleDemo.getState().schedules[scope][0]).toMatchObject({ enabled: false, runs: [{ status: 'success' }] })
  useTaskScheduleDemo.setState({ sources: [] })
  store.run(scope, id)
  expect(store.save(scope, taskDraft(useTaskScheduleDemo.getState().schedules[scope][0]), id)).toBeUndefined()
  expect(useTaskScheduleDemo.getState().schedules[scope][0].runs).toHaveLength(1)
})

it('rejects unavailable projects and reset cancels outstanding demo results', () => {
  const store = useTaskScheduleDemo.getState()
  expect(store.save(scope, { ...emptyDraft(), kind: 'task', name: '无效', sourceTaskId: 'CLOUD-155' })).toBeUndefined()
  const id = createTask()
  store.run(scope, id)
  store.reset()
  vi.runAllTimers()
  expect(useTaskScheduleDemo.getState().schedules).toEqual({})
})
