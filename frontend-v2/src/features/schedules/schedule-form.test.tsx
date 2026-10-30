import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { ScheduleForm } from './schedule-form'

it('validates weekly scheduling and accepts unscoped chat', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<QueryClientProvider client={new QueryClient()}><ScheduleForm onSave={onSave} onCancel={() => {}} data={{ projects: [] }} /></QueryClientProvider>)
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
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ kind: 'chat', projectId: '', rule: expect.objectContaining({ days: [5] }) }), expect.any(Object))
})
