import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { UserContext } from '@/app/auth/user'
import { demoUser, fakeBackend } from '@/test/backend'
import RealOverview from './real-overview'

const base = '/api/v2/tenants/team/tasks'
const task = { id: 'task-1', title: '真实任务交付', description: '实际任务描述', tenantId: 'team', projectId: 'project', assignee: { type: 'agent', id: 'agent', displayName: '执行 Agent' }, status: 'review', createdBy: 'user', createdByName: '成员', version: 1, createdAt: '2026-11-05T00:00:00Z', updatedAt: '2026-11-05T01:00:00Z', activeRun: null }
const timeline = { runs: [{ id: 'run', taskId: task.id, startedBy: 'user', startedByName: '成员', executorId: 'executor', executorName: '真实执行器', state: 'ended', lastResult: 'completed', failureCode: '', endReason: 'closed', createdAt: '2026-11-05T00:00:00Z', endedAt: '2026-11-05T01:00:00Z' }], comments: [] }
const tenants = { status: 200, data: { items: [{ id: 'team' }] } }
function show() {
  return render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><UserContext.Provider value={demoUser}><RealOverview /></UserContext.Provider></QueryClientProvider></MemoryRouter>)
}
afterEach(() => vi.unstubAllEnvs())

it.each(['', '1'])('reads real task ownership and output with demo flag %s', async flag => {
  vi.stubEnv('VITE_DEMO_MOCK', flag)
  const seen = fakeBackend(config => {
    if (config.url === '/api/v1/me/tenants') return tenants
    if (config.url === base) return { status: 200, data: { items: [task], nextCursor: '' } }
    if (config.url === `${base}/${task.id}/timeline`) return { status: 200, data: timeline }
  })
  show()
  const link = await screen.findByRole('link', { name: task.title })
  expect(link).toHaveAttribute('href', '/collaboration/tasks/task-1')
  expect(screen.getByText('真实执行器', { exact: false })).toBeInTheDocument()
  expect(screen.getByRole('progressbar', { name: '执行 Agent 完成率' })).toHaveAttribute('aria-valuenow', '0')
  expect(seen).toContain(`GET ${base}/${task.id}/timeline`)
})

it('hides old totals and outputs on revoked refresh, then recovers on retry', async () => {
  let denied = false
  fakeBackend(config => {
    if (config.url === '/api/v1/me/tenants') return tenants
    if (config.url === base) return denied ? { status: 403 } : { status: 200, data: { items: [task], nextCursor: '' } }
    if (config.url === `${base}/${task.id}/timeline`) return { status: 200, data: timeline }
  })
  show()
  await screen.findByRole('link', { name: task.title })
  denied = true
  await userEvent.click(screen.getByRole('button', { name: '刷新概览' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('团队数据不可访问')
  expect(screen.queryByRole('region', { name: '任务概览' })).not.toBeInTheDocument()
  expect(screen.queryByRole('link', { name: task.title })).not.toBeInTheDocument()
  denied = false
  await userEvent.click(screen.getByRole('button', { name: '刷新概览' }))
  expect(await screen.findByRole('link', { name: task.title })).toBeInTheDocument()
})

it.each(['incomplete', 'denied timeline'])('does not publish partial totals for %s data', async mode => {
  fakeBackend(config => {
    if (config.url === '/api/v1/me/tenants') return tenants
    if (config.url === base) return { status: 200, data: { items: [task], nextCursor: mode === 'incomplete' ? 'more' : '' } }
    if (config.url === `${base}/${task.id}/timeline`) return { status: 403 }
  })
  show()
  await screen.findByRole('alert')
  expect(screen.queryByRole('region', { name: '任务概览' })).not.toBeInTheDocument()
})

it('shows zero totals and explicit empty states without demo members', async () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  fakeBackend(config => {
    if (config.url === '/api/v1/me/tenants') return tenants
    if (config.url === base) return { status: 200, data: { items: [], nextCursor: '' } }
  })
  show()
  expect(await screen.findByText('还没有团队任务。')).toBeInTheDocument()
  const stats = screen.getByRole('region', { name: '任务概览' })
  expect(within(stats).getAllByText('0', { exact: true })).toHaveLength(4)
  expect(screen.queryByText('陈默')).not.toBeInTheDocument()
})

it('does not remain loading when no workspace is available', async () => {
  fakeBackend(config => config.url === '/api/v1/me/tenants' ? { status: 200, data: { items: [] } } : undefined)
  show()
  expect(await screen.findByText('暂无可访问的工作区。')).toBeInTheDocument()
})
