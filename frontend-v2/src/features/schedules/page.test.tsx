import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import Page from './page'
import { ScheduleForm } from './schedule-form'
import { useMockSchedules } from './mock-store'

beforeEach(() => { useMockSchedules.getState().reset() })
it('filters the list while retaining paused tasks', async () => {
  const user = userEvent.setup()
  render(<MemoryRouter><Page /></MemoryRouter>)
  await user.click(screen.getByRole('switch', { name: '每日 AI 简报启用状态' }))
  await user.selectOptions(screen.getByLabelText('状态筛选'), 'paused')
  expect(screen.getAllByRole('switch')).toHaveLength(2)
  expect(screen.getByRole('table')).toBeInTheDocument()
})
it('keeps both execution drafts, validates project and workflow inputs, and submits only on valid input', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<ScheduleForm onSave={onSave} onCancel={() => {}} />)
  fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '发布检查任务' } })
  fireEvent.change(screen.getByLabelText('任务指令'), { target: { value: '保留这段指令' } })
  await user.click(screen.getByRole('button', { name: /运行工作流/ }))
  await user.click(screen.getByRole('button', { name: /发布检查/ }))
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(onSave).not.toHaveBeenCalled()
  expect(screen.getByText('运行工作流需要项目，请选择运行环境')).toBeInTheDocument()
  expect(screen.getByText('请填写检查分支')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /直接对话/ }))
  expect(screen.getByLabelText('任务指令')).toHaveValue('保留这段指令')
  await user.click(screen.getByRole('button', { name: /运行工作流/ }))
  await user.selectOptions(screen.getByRole('combobox', { name: '关联项目' }), 'website')
  fireEvent.change(screen.getByLabelText('检查分支 *'), { target: { value: 'main' } })
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ kind: 'workflow', projectId: 'website', workflowId: 'release', snapshot: 'v3', inputs: { branch: 'main' } }))
})
it('validates weekly scheduling and accepts unscoped chat', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<ScheduleForm onSave={onSave} onCancel={() => {}} />)
  fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '每日简报' } })
  fireEvent.change(screen.getByLabelText('任务指令'), { target: { value: '总结资讯' } })
  expect(screen.getByRole('combobox', { name: '关联项目' })).toHaveValue('')
  await user.selectOptions(screen.getByLabelText('重复频率'), 'weekly')
  await user.click(within(screen.getByRole('group', { name: '执行星期' })).getByRole('button', { name: '星期一' }))
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(onSave).not.toHaveBeenCalled()
  expect(screen.getByText('请至少选择一天')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '星期五' }))
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ kind: 'chat', projectId: '', rule: expect.objectContaining({ days: [5] }) }))
})
