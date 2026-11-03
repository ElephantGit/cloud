import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import { UserContext } from '@/app/auth/user'
import type { CollaborationTarget, User } from '@/api/generated.schemas'
import * as api from './tasks-api/generated'
import type { Task, TaskRun, TaskTimeline } from './tasks-api/generated'
import TaskPage from './task-page'

const targets = vi.hoisted(() => ({ items: [] as CollaborationTarget[] }))
vi.mock('@/api/me/me', () => ({ useGetApiV1MeTenants: () => ({ data: { items: [{ id: 'tenant', role: 'owner' }] }, isLoading: false, isError: false }) }))
vi.mock('@/api/members/members', () => ({ useGetApiV1TenantsTidMembers: () => ({ data: { items: [] }, isLoading: false, isError: false }) }))
vi.mock('@/api/spaces/spaces', () => ({
  useGetApiV1TenantsTidSpaces: () => ({ data: { items: [], nextCursor: '' }, isLoading: false, isError: false }),
  useGetApiV1TenantsTidSpacesSpaceIdPlugins: () => ({ data: { items: [] }, isLoading: false, isError: false }),
}))
vi.mock('@/api/tenants/tenants', () => ({ useGetApiV1TenantsTidCollaborationTargets: () => ({ data: targets }) }))
vi.mock('./api/generated', () => ({ listSquads: vi.fn().mockResolvedValue({ items: [], nextCursor: '' }) }))
vi.mock('@/lib/projects-api/generated', () => ({ getApiV2TenantsTidProjects: vi.fn().mockResolvedValue({ items: [], nextCursor: '' }) }))
// The live thread has its own tests; here a run shows the session its stream would fold.
vi.mock('./run-session', () => ({ useRunSession: () => ({ conversation: null, refused: false }) }))
vi.mock('./tasks-api/generated', () => ({
  getTask: vi.fn(), getTaskTimeline: vi.fn(), listTaskExecutors: vi.fn(), startTaskRun: vi.fn(), stopTaskRun: vi.fn(),
  createTaskComment: vi.fn(), setTaskStatus: vi.fn(), setTaskAssignee: vi.fn(), updateTask: vi.fn(),
}))

const me: User = { id: 'user-me', displayName: '演示用户', status: 'active', version: 1, createdAt: '2026-10-01T00:00:00Z', deletedAt: null }
const agent = { type: 'agent' as const, id: 'agent-1', displayName: 'Claude Code', identifier: 'official/ora-space.claude' }
const task = (overrides: Partial<Task> = {}): Task => ({
  id: 'task-1', tenantId: 'tenant', title: '修复登录页', description: '按钮错位', projectId: 'project-1',
  assignee: agent, status: 'pending', createdBy: 'user-me', createdByName: '演示用户', version: 1,
  createdAt: '2026-10-30T00:00:00Z', updatedAt: '2026-10-30T00:00:00Z', activeRun: null, ...overrides,
})
const run = (overrides: Partial<TaskRun> = {}): TaskRun => ({
  id: 'run-1', taskId: 'task-1', startedBy: 'user-me', startedByName: '演示用户', executorId: 'claude:fixture', executorName: 'Claude Code / Fixture',
  state: 'running', lastResult: '', failureCode: '', endReason: '', createdAt: '2026-10-30T01:00:00Z', endedAt: null, ...overrides,
})
const empty: TaskTimeline = { runs: [], comments: [] }

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><UserContext.Provider value={me}>
    <MemoryRouter initialEntries={['/collaboration/tasks/task-1']}>
      <Routes><Route path="/collaboration/tasks/:taskId" element={<TaskPage />} /></Routes>
    </MemoryRouter>
  </UserContext.Provider></QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  targets.items = [
    { type: 'user', id: 'user-2', displayName: '鲍勃', description: '工作区成员', interactionDescriptor: { mode: 'mention', requiresTask: false, formRef: null } },
    { type: 'agent', id: 'agent-1', displayName: 'Claude Code', description: 'official/ora-space.claude', interactionDescriptor: { mode: 'task', requiresTask: true, formRef: null } },
  ]
  vi.mocked(api.getTask).mockResolvedValue(task())
  vi.mocked(api.getTaskTimeline).mockResolvedValue(empty)
  vi.mocked(api.listTaskExecutors).mockResolvedValue({ items: [
    { id: 'claude:fixture', name: 'Claude Code / Fixture', agentId: 'official/ora-space.claude', agentName: 'Claude Code', modelId: 'messages-fixture', modelName: 'Fixture' },
    { id: 'claude:configured', name: 'Claude Code / Sonnet', agentId: 'official/ora-space.claude', agentName: 'Claude Code', modelId: 'messages-configured', modelName: 'Sonnet' },
  ] })
})

it('starts the Agent with the configured model and shows the run being prepared', async () => {
  // Once started, the page reads the task and its timeline again and finds the run.
  let started = false
  vi.mocked(api.startTaskRun).mockImplementation(async () => { started = true; return run({ executorId: 'claude:configured' }) })
  vi.mocked(api.getTask).mockImplementation(async () => (started
    ? task({ status: 'running', activeRun: { id: 'run-1', state: 'running', startedBy: 'user-me', startedByName: '演示用户', executorName: 'Claude Code / Sonnet' } })
    : task()))
  vi.mocked(api.getTaskTimeline).mockImplementation(async () => (started ? { runs: [run()], comments: [] } : empty))
  renderPage()
  expect(await screen.findByText(/还没有运行/)).toBeInTheDocument()
  // An Agent task enters 进行中 by running, not by hand: its one 开始处理 starts the Agent.
  await waitFor(() => expect(screen.getByLabelText('执行模型')).toHaveValue('claude:configured'))
  expect(screen.getAllByRole('button', { name: '开始处理' })).toHaveLength(1)
  expect(within(screen.getByLabelText('执行')).getByRole('button', { name: '开始处理' })).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '开始处理' }))
  await waitFor(() => expect(api.startTaskRun).toHaveBeenCalledWith('tenant', 'task-1', { executorId: 'claude:configured' }, expect.objectContaining({ headers: expect.anything() })))

  expect(await screen.findByText(/正在准备 Agent/)).toBeInTheDocument()
  expect(await screen.findByText('Agent 正在执行，结束后再推进。')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '停止' })).toBeInTheDocument()
})

it('sends to the Agent by default while it is reachable, and to the team once switched off', async () => {
  vi.mocked(api.getTask).mockResolvedValue(task({ status: 'review', activeRun: { id: 'run-1', state: 'idle', startedBy: 'user-me', startedByName: '演示用户', executorName: 'Claude Code / Fixture' } }))
  vi.mocked(api.getTaskTimeline).mockResolvedValue({ runs: [run({ state: 'idle', lastResult: 'completed' })], comments: [] })
  vi.mocked(api.createTaskComment).mockResolvedValue({ id: 'c1', taskId: 'task-1', authorId: 'user-me', authorName: '演示用户', body: '再改改', mentions: [], runId: 'run-1', createdAt: '2026-10-30T02:00:00Z' })
  renderPage()
  const toAgent = await screen.findByRole('switch', { name: '发给 Agent' })
  await waitFor(() => expect(toAgent).toHaveAttribute('aria-checked', 'true'))
  const input = screen.getByLabelText('评论或给 Agent 的消息')
  await userEvent.type(input, '再改改')
  await userEvent.click(screen.getByRole('button', { name: '发给 Agent' }))
  await waitFor(() => expect(api.createTaskComment).toHaveBeenCalledWith('tenant', 'task-1', {
    body: '再改改', mentions: [], toAgent: true, executorId: 'claude:configured',
  }, expect.anything()))

  await userEvent.click(toAgent)
  expect(toAgent).toHaveAttribute('aria-checked', 'false')
  await userEvent.type(input, '只给团队看')
  await userEvent.click(screen.getByRole('button', { name: '评论' }))
  await waitFor(() => expect(api.createTaskComment).toHaveBeenLastCalledWith('tenant', 'task-1', {
    body: '只给团队看', mentions: [], toAgent: false, executorId: undefined,
  }, expect.anything()))
})

it('sends a comment to the Agent when its own Agent is picked from the @ menu', async () => {
  vi.mocked(api.getTask).mockResolvedValue(task({ status: 'review', activeRun: { id: 'run-1', state: 'idle', startedBy: 'user-me', startedByName: '演示用户', executorName: 'Claude Code / Fixture' } }))
  vi.mocked(api.getTaskTimeline).mockResolvedValue({ runs: [run({ state: 'idle', lastResult: 'completed' })], comments: [] })
  vi.mocked(api.createTaskComment).mockResolvedValue({ id: 'c1', taskId: 'task-1', authorId: 'user-me', authorName: '演示用户', body: '@Claude Code 按钮再大一点', mentions: [], runId: 'run-1', createdAt: '2026-10-30T02:00:00Z' })
  renderPage()
  const input = await screen.findByLabelText('评论或给 Agent 的消息')
  await userEvent.type(input, '@Cla')
  const menu = await screen.findByRole('listbox', { name: '@ 对象' })
  await userEvent.click(within(menu).getByRole('option', { name: /Claude Code/ }))
  await userEvent.type(input, '按钮再大一点')
  await userEvent.click(screen.getByRole('button', { name: '发给 Agent' }))
  await waitFor(() => expect(api.createTaskComment).toHaveBeenCalledWith('tenant', 'task-1', {
    body: '@Claude Code 按钮再大一点', mentions: [{ type: 'agent', id: 'agent-1' }], toAgent: true, executorId: 'claude:configured',
  }, expect.anything()))
})

it('keeps another member to comments while someone else owns the run', async () => {
  vi.mocked(api.getTask).mockResolvedValue(task({ status: 'review', activeRun: { id: 'run-1', state: 'idle', startedBy: 'user-2', startedByName: '鲍勃', executorName: 'Claude Code / Fixture' } }))
  vi.mocked(api.getTaskTimeline).mockResolvedValue({ runs: [run({ state: 'idle', startedBy: 'user-2', startedByName: '鲍勃' })], comments: [] })
  vi.mocked(api.createTaskComment).mockResolvedValue({ id: 'c1', taskId: 'task-1', authorId: 'user-me', authorName: '演示用户', body: '@鲍勃 看起来不错', mentions: [{ type: 'user', id: 'user-2', displayName: '鲍勃' }], runId: '', createdAt: '2026-10-30T02:00:00Z' })
  renderPage()
  expect(await screen.findByRole('switch', { name: '发给 Agent' })).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByText(/当前运行由 鲍勃 发起，只有发起人能追问/)).toBeInTheDocument()
  const input = screen.getByLabelText('评论或给 Agent 的消息')
  await userEvent.type(input, '@')
  const menu = await screen.findByRole('listbox', { name: '@ 对象' })
  expect(within(menu).getByRole('option', { name: /Claude Code/ })).toBeDisabled()
  await userEvent.type(input, '鲍')
  await userEvent.click(within(await screen.findByRole('listbox', { name: '@ 对象' })).getByRole('option', { name: /鲍勃/ }))
  await userEvent.type(input, '看起来不错')
  await userEvent.click(screen.getByRole('button', { name: '评论' }))
  await waitFor(() => expect(api.createTaskComment).toHaveBeenCalledWith('tenant', 'task-1', {
    body: '@鲍勃 看起来不错', mentions: [{ type: 'user', id: 'user-2' }], toAgent: false, executorId: undefined,
  }, expect.anything()))
})

it('says why a refused start failed instead of retrying', async () => {
  vi.mocked(api.startTaskRun).mockRejectedValue({ isAxiosError: true, response: { status: 409, data: { code: 'run_active' } } })
  renderPage()
  await userEvent.click(await screen.findByRole('button', { name: '开始处理' }))
  expect(await screen.findByText('Agent 正在执行，等这一轮结束或先停止。')).toBeInTheDocument()
})
