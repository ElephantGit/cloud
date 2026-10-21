import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw-server'
import type { ScheduledTask } from './api'
import { ScheduledTasksPage } from './scheduled-tasks-page'

const TID = 'tid1'
const LIST = '/api/v1/tenants/tid1/scheduled-tasks'
const ISSUES = '/api/v1/tenants/tid1/issues'
const TARGETS = '/api/v1/tenants/tid1/collaboration/targets'

function task(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return {
    id: 'st1',
    issueId: 'i1',
    name: '每日同步',
    prompt: '同步仓库',
    agentId: 'a1',
    schedule: { kind: 'daily', hour: 9, minute: 0 },
    timezone: 'Asia/Shanghai',
    revision: 1,
    version: 1,
    enabled: false,
    createdAt: '2026-10-01T00:00:00Z',
    updatedAt: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

function serve(tasks: ScheduledTask[] = []) {
  server.use(
    http.get(LIST, () => HttpResponse.json({ tasks })),
    http.get(ISSUES, () =>
      HttpResponse.json({ items: [{ id: 'i1', number: 1, title: '问题一' }], nextCursor: '' }),
    ),
    http.get(TARGETS, () =>
      HttpResponse.json({
        items: [{ type: 'agent', id: 'a1', displayName: 'Codex', description: '' }],
        nextCursor: '',
      }),
    ),
  )
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ScheduledTasksPage slug={TID} />
    </QueryClientProvider>,
  )
}

describe('ScheduledTasksPage', () => {
  it('renders the task list with its schedule summary and enabled state', async () => {
    serve([
      task(),
      task({
        id: 'st2',
        name: '每周报告',
        schedule: { kind: 'weekly', hour: 18, minute: 30, weekdays: ['monday', 'friday'] },
        enabled: true,
      }),
    ])
    renderPage()
    const first = await screen.findByRole('article', { name: '每日同步' })
    expect(first).toHaveTextContent('每天 09:00')
    expect(first).toHaveTextContent('已停用')
    const second = await screen.findByRole('article', { name: '每周报告' })
    expect(second).toHaveTextContent('每周 周一、周五 18:30')
    expect(second).toHaveTextContent('已启用')
  })

  it('creates a task by posting the draft with the consent policy', async () => {
    serve()
    let body: unknown
    server.use(
      http.post(LIST, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ resource: task() })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: '新建定时任务' }))
    await user.type(screen.getByLabelText('名称'), '每日同步')
    await user.type(screen.getByLabelText('提示词'), '同步仓库')
    await user.selectOptions(screen.getByLabelText('绑定 issue'), 'i1')
    await user.selectOptions(screen.getByLabelText('执行智能体'), 'a1')
    await user.click(screen.getByRole('button', { name: '创建' }))
    await waitFor(() =>
      expect(body).toMatchObject({
        name: '每日同步',
        prompt: '同步仓库',
        issueId: 'i1',
        agentId: 'a1',
        schedule: { kind: 'daily', hour: 9, minute: 0 },
        timezone: 'Asia/Shanghai',
        approval: { policy: 'full_auto_in_cloud_workspace' },
        enabled: false,
      }),
    )
  })

  it('deletes a task only after confirmation', async () => {
    serve([task()])
    let deleted = null
    server.use(
      http.delete(`${LIST}/st1`, () => {
        deleted = 'st1'
        return HttpResponse.json({ taskId: 'st1' })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(deleted).toBeNull()
    await user.click(screen.getByRole('button', { name: '确认删除' }))
    expect(deleted).toBe('st1')
  })

  it('disables an enabled task through the switch', async () => {
    serve([task({ enabled: true })])
    let disabled = false
    server.use(
      http.post(`${LIST}/st1/disable`, () => {
        disabled = true
        return HttpResponse.json({ resource: task({ enabled: false }) })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    expect(await screen.findByRole('article', { name: '每日同步' })).toHaveTextContent('已启用')
    await user.click(screen.getByRole('switch'))
    expect(disabled).toBe(true)
  })
})
