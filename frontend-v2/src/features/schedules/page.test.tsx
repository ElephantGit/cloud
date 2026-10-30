import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import Page from './page'
import * as api from './api/generated'

vi.mock('@/api/me/me', () => ({ useGetApiV1MeTenants: () => ({ data: { items: [{ id: 'tenant' }] }, isLoading: false, isError: false }) }))
vi.mock('./api/generated', () => ({ listScheduleConfigs: vi.fn(), createScheduleConfig: vi.fn(), updateScheduleConfig: vi.fn(), setScheduleConfigEnabled: vi.fn() }))
const item = { id: 'task-1', name: '早报', projectId: '', target: { kind: 'chat' as const, prompt: '摘要', agent: 'a', model: 'm' }, rule: { kind: 'daily' as const, time: '09:00', days: [1], hours: 6, timezone: 'Asia/Shanghai' }, enabled: true, version: 1, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', runs: [] }
function renderPage() { const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); return render(<QueryClientProvider client={client}><Page /></QueryClientProvider>) }
beforeEach(() => { HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }; HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }; vi.clearAllMocks(); vi.mocked(api.listScheduleConfigs).mockResolvedValue({ items: [item], nextCursor: '' }) })
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
