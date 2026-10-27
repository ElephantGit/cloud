import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { WorkflowNodeSession } from '@/api/generated.schemas'
import { RunNodeSession } from '@/features/workflows/run/run-node-session'
import { server } from '@/test/msw-server'
import { renderWithProviders } from '@/test/render'

const SESSION_PATH = '/api/v1/tenants/:tid/workflows/:wfid/runs/:rid/nodes/:nodeId/session'

/** One agent node attempt as the endpoint answers it, with defaults a test can leave alone. */
function session(overrides: {
  records?: WorkflowNodeSession['execution']['records']
  state?: WorkflowNodeSession['execution']['state']
  hasMore?: boolean
  nextBefore?: number | null
  sessionEndReason?: string | null
}): WorkflowNodeSession {
  return {
    execution: {
      attempt: 1,
      executionId: 'exec-1',
      result: null,
      records: overrides.records ?? [],
      state: overrides.state ?? 'available',
      incomplete: false,
      hasMore: overrides.hasMore ?? false,
      nextBefore: overrides.nextBefore ?? null,
    },
    node: {
      status: 'succeeded',
      attempt: 1,
      error: null,
      output: null,
      sessionEndReason: overrides.sessionEndReason ?? null,
    },
  }
}

/** A stored message chunk, as the reader projects it into a row. */
function chunk(sequence: number, text: string) {
  return {
    sequence,
    record: {
      type: 'update',
      update: { sessionUpdate: 'agent_message_chunk', content: [{ type: 'text', text }] },
    },
  }
}

/** Serves the session read from `answer`, which also sees the request's own query. */
function installSession(answer: (url: URL) => Response): void {
  server.use(http.get(SESSION_PATH, ({ request }) => answer(new URL(request.url))))
}

/** Mounts the panel for a fixed agent node. */
function renderPanel() {
  return renderWithProviders(
    <RunNodeSession tenantId="tenant-1" workflowId="workflow-1" runId="run-1" nodeId="agent-1" />,
  )
}

describe('RunNodeSession', () => {
  it('shows the attempt, its records oldest first and the end reason on its own row', async () => {
    installSession(() =>
      HttpResponse.json(
        session({
          records: [chunk(1, '第一条'), chunk(2, '第二条'), chunk(3, '第三条')],
          sessionEndReason: 'end_turn',
        }),
      ),
    )
    renderPanel()

    const region = screen.getByRole('region', { name: '会话' })
    const items = await within(region).findAllByRole('listitem')
    expect(within(region).getByText('第 1 次尝试')).toBeInTheDocument()
    expect(items.map((item) => item.textContent)).toEqual(['第一条', '第二条', '第三条'])
    expect(within(region).getByText('结束原因')).toBeInTheDocument()
    expect(within(region).getByText('end_turn')).toBeInTheDocument()
  })

  it("labels the prompt the Node echoed as the attempt's user input", async () => {
    installSession(() =>
      HttpResponse.json(
        session({
          records: [
            {
              sequence: 1,
              record: {
                type: 'update',
                update: {
                  sessionUpdate: 'user_message_chunk',
                  content: [{ type: 'text', text: '整理发布说明' }],
                },
              },
            },
            chunk(2, '已整理'),
          ],
        }),
      ),
    )
    renderPanel()

    const region = screen.getByRole('region', { name: '会话' })
    const items = await within(region).findAllByRole('listitem')
    expect(items.map((item) => item.textContent)).toEqual(['用户输入整理发布说明', '已整理'])
  })

  it.each([
    ['not_started', '这次尝试还没有派发会话，暂时没有记录。'],
    ['unavailable', '这次尝试已经结束，但没有留下可回放的会话记录。'],
    ['unknown', '无法确认这次尝试是否留下了会话记录。'],
  ] as const)(
    'names the %s state instead of drawing an empty conversation',
    async (state, copy) => {
      installSession(() => HttpResponse.json(session({ state })))
      renderPanel()

      const region = screen.getByRole('region', { name: '会话' })
      expect(await within(region).findByText(copy)).toBeInTheDocument()
      expect(within(region).queryByRole('list')).not.toBeInTheDocument()
    },
  )

  it('loads the earlier page with the attempt and sequence the response pinned', async () => {
    const requested: URL[] = []
    installSession((url) => {
      requested.push(url)
      return HttpResponse.json(
        url.searchParams.get('before') === null
          ? session({ records: [chunk(5, '较新')], hasMore: true, nextBefore: 5 })
          : session({ records: [chunk(2, '更早')] }),
      )
    })
    renderPanel()

    const region = screen.getByRole('region', { name: '会话' })
    await within(region).findByText('较新')
    fireEvent.click(within(region).getByRole('button', { name: '加载更早的记录' }))

    await waitFor(() => expect(within(region).getAllByRole('listitem')).toHaveLength(2))
    expect(
      requested.map((url) => [url.searchParams.get('attempt'), url.searchParams.get('before')]),
    ).toEqual([
      [null, null],
      ['1', '5'],
    ])
    expect(
      within(region)
        .getAllByRole('listitem')
        .map((item) => item.textContent),
    ).toEqual(['更早', '较新'])
  })

  it('tells a failed read apart from a node that recorded nothing', async () => {
    installSession(() =>
      HttpResponse.json({ code: 'internal', params: {}, requestId: 'r' }, { status: 500 }),
    )
    renderPanel()

    const region = screen.getByRole('region', { name: '会话' })
    expect(
      await within(region).findByText('会话记录读取失败，稍后会自动重试。'),
    ).toBeInTheDocument()
  })
})
