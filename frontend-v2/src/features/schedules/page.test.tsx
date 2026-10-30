import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import Page from './page'
import * as api from './api/generated'
import { useTaskScheduleDemo } from './task-demo-store'

vi.mock('@/app/auth/user', () => ({ useUser: () => ({ id: 'user' }) }))

vi.mock('@/api/me/me', () => ({ useGetApiV1MeTenants: () => ({ data: { items: [{ id: 'tenant' }] }, isLoading: false, isError: false }) }))
vi.mock('./api/generated', () => ({ listScheduleConfigs: vi.fn(), createScheduleConfig: vi.fn(), updateScheduleConfig: vi.fn(), setScheduleConfigEnabled: vi.fn() }))
const item = { id: 'task-1', name: '早报', projectId: '', target: { kind: 'chat' as const, prompt: '摘要', agent: 'a', model: 'm' }, rule: { kind: 'daily' as const, time: '09:00', days: [1], hours: 6, timezone: 'Asia/Shanghai' }, enabled: true, version: 1, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', runs: [] }
function renderPage() { const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); return render(<QueryClientProvider client={client}><Page /></QueryClientProvider>) }
beforeEach(() => { useTaskScheduleDemo.getState().reset(); HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }; HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }; vi.clearAllMocks(); vi.mocked(api.listScheduleConfigs).mockResolvedValue({ items: [item], nextCursor: '' }) })
it('loads persistent configurations and disables unimplemented execution', async () => { renderPage(); expect(await screen.findByText('早报')).toBeInTheDocument(); expect(api.listScheduleConfigs).toHaveBeenCalledWith('tenant'); await userEvent.click(screen.getByRole('button', { name: /查看早报/ })); expect(screen.getByText(/立即运行和运行历史尚未接入/)).toBeInTheDocument(); expect(screen.queryByRole('button', { name: /模拟运行/ })).not.toBeInTheDocument() })
it('creates a text configuration through the generated client', async () => { vi.mocked(api.createScheduleConfig).mockResolvedValue({ ...item, name: '新任务' }); renderPage(); await screen.findByText('早报'); await userEvent.click(screen.getByRole('button', { name: '新建任务' })); fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '新任务' } }); fireEvent.change(screen.getByLabelText('任务指令'), { target: { value: '摘要' } }); await userEvent.click(screen.getByRole('button', { name: '创建并启用' })); await waitFor(() => expect(api.createScheduleConfig).toHaveBeenCalledWith('tenant', expect.objectContaining({ name: '新任务', projectId: '', target: expect.objectContaining({ kind: 'chat', prompt: '摘要' }) }))) })
it('shows a load failure instead of sample tasks', async () => { vi.mocked(api.listScheduleConfigs).mockRejectedValue(new Error('offline')); renderPage(); expect(await screen.findByRole('alert')).toHaveTextContent('加载失败'); expect(screen.queryByText('早报')).not.toBeInTheDocument() })
it('preserves edit input and blocks resubmission after version conflict', async () => {
  vi.mocked(api.updateScheduleConfig).mockRejectedValue({ isAxiosError: true, response: { status: 409 } })
  renderPage(); await screen.findByText('早报'); await userEvent.click(screen.getByRole('button', { name: /查看早报/ })); await userEvent.click(screen.getByRole('button', { name: '编辑' }));
  fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '我的修改' } }); await userEvent.click(screen.getByRole('button', { name: '保存修改' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('已被其他操作修改'); expect(screen.getByLabelText('任务名称')).toHaveValue('我的修改'); expect(screen.getByRole('button', { name: '保存修改' })).toBeDisabled(); expect(api.updateScheduleConfig).toHaveBeenCalledWith('tenant', 'task-1', expect.objectContaining({ version: 1 }))
})

it('clears a previous edit conflict when opening a new task', async () => {
  vi.mocked(api.updateScheduleConfig).mockRejectedValue({ isAxiosError: true, response: { status: 409 } })
  renderPage(); await screen.findByText('早报'); await userEvent.click(screen.getByRole('button', { name: /查看早报/ })); await userEvent.click(screen.getByRole('button', { name: '编辑' }));
  await userEvent.click(screen.getByRole('button', { name: '保存修改' })); await screen.findByRole('alert'); await userEvent.click(screen.getByRole('button', { name: '关闭面板' }));
  await userEvent.click(screen.getByRole('button', { name: '新建任务' })); expect(screen.getByRole('button', { name: '创建并启用' })).toBeEnabled(); expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it('adds existing tasks through the original drawer and table without changing persistent records', async () => {
  renderPage(); await screen.findByText('早报')
  expect(screen.queryByText('体验新功能')).not.toBeInTheDocument()
  for (const name of ['任务名称', '执行方式', '关联项目', '时间安排', '最近运行', '启用']) expect(screen.getByRole('columnheader', { name })).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '新建任务' }))
  fireEvent.change(screen.getByLabelText('任务指令'), { target: { value: '保留原对话草稿' } })
  await userEvent.click(screen.getByRole('button', { name: /已有任务/ }))
  await userEvent.click(screen.getByRole('button', { name: /官网上线检查 CLOUD-128/ }))
  expect(screen.getByLabelText('任务名称')).toHaveValue('官网上线检查 · 定时执行')
  expect(screen.getByLabelText('关联项目')).toHaveValue('Ora 官网')
  fireEvent.change(screen.getByLabelText('任务名称'), { target: { value: '我的计划' } })
  fireEvent.change(screen.getByLabelText('补充说明（可选）'), { target: { value: '我的补充说明' } })
  await userEvent.click(screen.getByRole('button', { name: '更换任务' }))
  await userEvent.click(screen.getByRole('button', { name: /项目依赖检查 CLOUD-142/ }))
  expect(screen.getByLabelText('任务名称')).toHaveValue('我的计划')
  expect(screen.getByLabelText('补充说明（可选）')).toHaveValue('我的补充说明')
  await userEvent.click(screen.getByRole('button', { name: /直接对话/ }))
  expect(screen.getByLabelText('任务指令')).toHaveValue('保留原对话草稿')
  await userEvent.click(screen.getByRole('button', { name: /已有任务/ }))
  await userEvent.click(screen.getByRole('button', { name: '创建并启用' }))
  await userEvent.click(screen.getByRole('button', { name: '关闭面板' }))
  expect(screen.getByRole('button', { name: '查看我的计划' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '查看早报' })).toBeInTheDocument()
  await userEvent.click(screen.getByRole('switch', { name: '我的计划启用状态' }))
  expect(screen.getByRole('switch', { name: '我的计划启用状态' })).not.toBeChecked()
  await userEvent.selectOptions(screen.getByLabelText('执行方式筛选'), 'task')
  expect(screen.queryByRole('button', { name: '查看早报' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '查看我的计划' })).toBeInTheDocument()
  expect(api.createScheduleConfig).not.toHaveBeenCalled()
  expect(api.updateScheduleConfig).not.toHaveBeenCalled()
  expect(api.setScheduleConfigEnabled).not.toHaveBeenCalled()
})

it('keeps selection edits on close and displays missing sources and empty searches', async () => {
  renderPage(); await screen.findByText('早报')
  await userEvent.click(screen.getByRole('button', { name: '新建任务' }))
  await userEvent.click(screen.getByRole('button', { name: /已有任务/ }))
  fireEvent.change(screen.getByLabelText('选择任务'), { target: { value: 'no-match' } })
  expect(screen.getByText(/没有匹配的任务/)).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('选择任务'), { target: { value: '142' } })
  await userEvent.click(screen.getByRole('button', { name: /CLOUD-142/ }))
  await userEvent.click(screen.getByRole('button', { name: '关闭面板' }))
  expect(screen.getByText('放弃未保存的修改？')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  expect(screen.getByLabelText('任务名称')).toHaveValue('项目依赖检查 · 定时执行')
  await userEvent.click(screen.getByRole('button', { name: '清除选择' }))
  expect(screen.getByRole('button', { name: '创建并启用' })).toBeDisabled()
})
