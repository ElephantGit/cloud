import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { UserContext } from '@/app/auth/user'
import { demoUser, fakeBackend } from '@/test/backend'
import List from './list'
import Detail from './detail'

const tenant = '00000000-0000-4000-8000-000000000002'
const base = `/api/v2/tenants/${tenant}/dashboard/sessions`
const session = {
  id: 'session-1',
  title: '真实执行会话',
  projectId: 'project-1',
  projectName: '演示项目',
  agent: 'official/ora-space.opencode',
  model: 'fixture',
  status: 'completed',
  source: 'task',
  href: '/collaboration/tasks/task-1',
  createdAt: '2026-11-03T01:00:00Z',
  endedAt: '2026-11-03T01:01:00Z',
}
function show(detail = false) {
  return render(
    <MemoryRouter
      initialEntries={[detail ? '/dashboard/sessions/session-1' : '/dashboard/sessions']}
    >
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <UserContext.Provider value={demoUser}>
          <Routes>
            <Route path='/dashboard/sessions' element={<List />} />
            <Route path='/dashboard/sessions/:id' element={<Detail />} />
          </Routes>
        </UserContext.Provider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}
afterEach(() => vi.unstubAllEnvs())

it.each(['task', 'chat'])('reads real %s session data with demo mock enabled and opens its source link', async (source) => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  const current = { ...session, source, href: source === 'chat' ? '/chat?conversation=chat-1' : session.href }
  const seen = fakeBackend((config) => {
    if (config.url === '/api/v1/me/tenants') {
      return { status: 200, data: { items: [{ id: tenant }] } }
    }
    if (config.url === base) return { status: 200, data: { items: [current], nextCursor: '' } }
    if (config.url === `${base}/session-1`) return { status: 200, data: current }
    if (config.url === `${base}/session-1/history`) {
      return { status: 200, data: { source: 'persisted_snapshot', records: [], incomplete: false } }
    }
    if (config.url?.endsWith('/projects')) {
      return { status: 200, data: { items: [{ id: 'project-1', name: '演示项目' }] } }
    }
  })
  show()
  await userEvent.click(await screen.findByRole('link', { name: session.title }))
  expect(await screen.findByRole('region', { name: '会话概览' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '返回来源' })).toHaveAttribute('href', current.href)
  expect(seen).toContain(`GET ${base}/session-1/history`)
  expect(screen.queryByText('陈默')).not.toBeInTheDocument()
})

it('shows statistics independently of a failed history read and clears old content on revocation', async () => {
  let revoked = false
  fakeBackend((config) => {
    if (config.url === '/api/v1/me/tenants') {
      return { status: 200, data: { items: [{ id: tenant }] } }
    }
    if (config.url === `${base}/session-1`) {
      return { status: revoked ? 404 : 200, data: revoked ? { code: 'not_found' } : session }
    }
    if (config.url?.endsWith('/history')) return { status: 500, data: { code: 'unavailable' } }
    if (config.url?.endsWith('/statistics')) return { status: 200, data: { counts: { total: 0 } } }
    if (config.url?.endsWith('/model-calls')) {
      return { status: 200, data: { items: [], nextCursor: '' } }
    }
  })
  show(true)
  expect(await screen.findByRole('alert')).toHaveTextContent('历史读取失败')
  await userEvent.click(screen.getByRole('button', { name: '模型请求' }))
  expect(await screen.findByText('暂无模型请求观测')).toBeInTheDocument()
  revoked = true
  await userEvent.click(screen.getByRole('button', { name: '刷新' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('会话不可访问')
  expect(screen.queryByText('暂无模型请求观测')).not.toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: session.title })).not.toBeInTheDocument()
})

it('resets pagination when filters change', async () => {
  const requests: Array<Record<string, string>> = []
  fakeBackend((config) => {
    if (config.url === '/api/v1/me/tenants') {
      return { status: 200, data: { items: [{ id: tenant }] } }
    }
    if (config.url === base) {
      requests.push(config.params)
      return {
        status: 200,
        data: { items: [session], nextCursor: config.params?.after ? '' : 'page-two' },
      }
    }
    if (config.url?.endsWith('/projects')) return { status: 200, data: { items: [] } }
  })
  show()
  await userEvent.click(await screen.findByRole('button', { name: '加载更多会话' }))
  await screen.findAllByRole('link', { name: session.title })
  await userEvent.type(screen.getByRole('textbox', { name: 'Agent 标识' }), session.agent)
  await userEvent.selectOptions(screen.getByRole('combobox', { name: '来源' }), 'chat')
  await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
  await vi.waitFor(() => expect(requests.at(-1)).toMatchObject({ agent: session.agent, source: 'chat' }))
  expect(requests.at(-1)?.after).toBeUndefined()
})

it('filters seeded and live sessions using the full Agent ID shown in the hint', async () => {
  const seeded = { ...session, id: 'seeded', title: '预置分析样例（非现场执行）' }
  fakeBackend((config) => {
    if (config.url === '/api/v1/me/tenants') {
      return { status: 200, data: { items: [{ id: tenant }] } }
    }
    if (config.url === base) {
      const agent = config.params?.agent
      return {
        status: 200,
        data: { items: !agent || agent === session.agent ? [session, seeded] : [], nextCursor: '' },
      }
    }
    if (config.url?.endsWith('/projects')) return { status: 200, data: { items: [] } }
  })
  show()
  await screen.findByRole('link', { name: session.title })
  const filter = screen.getByPlaceholderText('例如 official/ora-space.opencode')
  await userEvent.type(filter, 'opencode')
  await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
  expect(await screen.findByText('暂无符合条件的会话。')).toBeInTheDocument()
  await userEvent.clear(screen.getByRole('textbox', { name: 'Agent 标识' }))
  await userEvent.type(screen.getByRole('textbox', { name: 'Agent 标识' }), session.agent)
  await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
  expect(await screen.findByRole('link', { name: session.title })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: seeded.title })).toBeInTheDocument()
})
