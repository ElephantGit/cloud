import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import TaskPreview from './task-preview'
import { useTaskPreview } from './task-preview-model'
import { useChatStore } from '../chat/store'

beforeEach(() => {
  useTaskPreview.getState().reset()
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute('open')
  }
})
afterEach(() => {
  useTaskPreview.getState().reset()
  vi.useRealTimers()
})

it('uses latest source content, freezes history, allows completed sources and never touches chat', () => {
  vi.useFakeTimers()
  const chat = useChatStore.getState()
  const original = structuredClone(useTaskPreview.getState().sources)
  useTaskPreview.getState().run('weekly-check')
  useTaskPreview.getState().run('weekly-check')
  expect(useTaskPreview.getState().schedules[0].runs).toHaveLength(1)
  vi.runAllTimers()
  useTaskPreview.setState({
    sources: original.map((s, i) => (i ? s : { ...s, description: '最新内容', project: '新项目', status: '已完成' })),
  })
  useTaskPreview.getState().run('weekly-check', true)
  vi.runAllTimers()
  const runs = useTaskPreview.getState().schedules[0].runs
  expect(runs[0]).toMatchObject({ status: 'failed', source: { description: '最新内容', project: '新项目', status: '已完成' } })
  expect(runs[1].source).toEqual(original[0])
  expect(useChatStore.getState()).toBe(chat)
})

it('blocks unavailable sources without deleting their history', () => {
  vi.useFakeTimers()
  useTaskPreview.getState().run('weekly-check')
  vi.runAllTimers()
  useTaskPreview.setState({ sources: [] })
  useTaskPreview.getState().run('weekly-check')
  expect(useTaskPreview.getState().schedules[0].runs).toHaveLength(1)
})

it('selects completed tasks, preserves edited fields across replacement, and saves in memory', async () => {
  render(<TaskPreview />)
  await userEvent.click(screen.getByRole('button', { name: '新建定时任务' }))
  expect(screen.getByRole('button', { name: '保存演示配置' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: /官网上线检查 CLOUD-128/ }))
  expect(screen.getByLabelText('任务名称')).toHaveValue('官网上线检查 · 定时执行')
  fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '自定义定时名称' } })
  fireEvent.change(screen.getByLabelText('补充说明（可选）'), { target: { value: '保留这段说明' } })
  await userEvent.click(screen.getByRole('button', { name: /项目依赖检查 CLOUD-142/ }))
  expect(screen.getByLabelText('任务名称')).toHaveValue('自定义定时名称')
  expect(screen.getByLabelText('补充说明（可选）')).toHaveValue('保留这段说明')
  await userEvent.click(screen.getByRole('button', { name: '保存演示配置' }))
  expect(useTaskPreview.getState().schedules[0]).toMatchObject({
    name: '自定义定时名称',
    target: { sourceTaskId: 'CLOUD-142', supplementalInstruction: '保留这段说明' },
  })
})

it('confirms closing source-only edits and handles empty results', async () => {
  render(<TaskPreview />)
  await userEvent.click(screen.getByRole('button', { name: '新建定时任务' }))
  await userEvent.click(screen.getByRole('button', { name: /官网上线检查 CLOUD-128/ }))
  await userEvent.click(screen.getByRole('button', { name: '关闭面板' }))
  expect(screen.getByText('放弃未保存的修改？')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  fireEvent.change(screen.getByLabelText('选择任务'), { target: { value: 'no-match' } })
  expect(screen.getByText('没有匹配的任务')).toBeInTheDocument()
  act(() => useTaskPreview.setState({ sources: [] }))
  expect(screen.getByText('暂无可用任务')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '保存演示配置' })).toBeDisabled()
})
