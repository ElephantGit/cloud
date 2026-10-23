import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '@/test/msw-server'
import { ScheduledRunArtifacts } from './scheduled-run-artifacts'

const LIST = '/api/v1/tenants/tid1/issues/i1/runs/r1/artifacts'
const ARTIFACT = {
  index: 3,
  name: 'reports/result.html',
  kind: 'html',
  sizeBytes: 42,
  sha256: 'a'.repeat(64),
}
const URL = 'https://objects.example.test/scheduled-runs/tid1/r1/3?signature=first'
const GRANT = { ...ARTIFACT, url: URL, expiresAt: '2099-10-24T01:00:00Z' }

function renderArtifacts() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={client}>
      <ScheduledRunArtifacts tid="tid1" issueId="i1" runId="r1" />
    </QueryClientProvider>,
  )
  return { ...view, client, user: userEvent.setup() }
}

async function openArtifacts() {
  server.use(http.get(LIST, () => HttpResponse.json({ artifacts: [ARTIFACT] })))
  const context = renderArtifacts()
  await context.user.click(screen.getByRole('button', { name: '查看产物' }))
  await screen.findByText(/reports\/result.html/)
  return context
}

describe('ScheduledRunArtifacts', () => {
  it('loads metadata on expansion and requests fresh grants only after explicit clicks', async () => {
    let lists = 0
    let grants = 0
    server.use(
      http.get(LIST, () => {
        lists++
        return HttpResponse.json({ artifacts: [ARTIFACT] })
      }),
      http.get(LIST + '/3', () => {
        grants++
        return HttpResponse.json({ ...GRANT, url: URL + grants })
      }),
    )
    const { user, client } = renderArtifacts()
    expect(lists).toBe(0)
    await user.click(screen.getByRole('button', { name: '查看产物' }))
    await screen.findByText(/reports\/result.html/)
    expect(lists).toBe(1)
    expect(grants).toBe(0)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '获取下载链接' }))
    const link = await screen.findByRole('link', { name: '下载 reports/result.html' })
    expect(link).toHaveAttribute('href', URL + '1')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(link).toHaveAttribute('referrerpolicy', 'no-referrer')
    expect(screen.getByText(/失效后请重新获取/)).toBeInTheDocument()
    expect(JSON.stringify(client.getQueryData([LIST]))).not.toContain('signature')
    await user.click(screen.getByRole('button', { name: '获取下载链接' }))
    expect(await screen.findByRole('link')).toHaveAttribute('href', URL + '2')
    expect(grants).toBe(2)
    await user.click(screen.getByRole('button', { name: '收起产物' }))
    await user.click(screen.getByRole('button', { name: '查看产物' }))
    await screen.findByText(/reports\/result.html/)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(grants).toBe(2)
  })

  it('shows an empty current index even when history previously counted files', async () => {
    server.use(http.get(LIST, () => HttpResponse.json({ artifacts: [] })))
    const { user } = renderArtifacts()
    await user.click(screen.getByRole('button', { name: '查看产物' }))
    expect(await screen.findByText(/暂无可下载产物/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '获取下载链接' })).not.toBeInTheDocument()
  })

  it('allows retrying a failed metadata read without minting a download grant', async () => {
    let attempts = 0
    server.use(
      http.get(LIST, () => {
        attempts++
        return attempts === 1
          ? HttpResponse.json({ code: 'unavailable' }, { status: 503 })
          : HttpResponse.json({ artifacts: [ARTIFACT] })
      }),
    )
    const { user } = renderArtifacts()
    await user.click(screen.getByRole('button', { name: '查看产物' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('产物列表加载失败')
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText(/reports\/result.html/)).toBeInTheDocument()
    expect(attempts).toBe(2)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it.each([
    [404, 'artifact_unavailable', '产物已不可用'],
    [503, 'object_store_unavailable', '产物存储暂不可用'],
    [403, 'forbidden', '操作失败'],
  ])(
    'shows a safe failure for download status %s and clears a previous link',
    async (status, code, message) => {
      let attempts = 0
      server.use(
        http.get(LIST + '/3', () => {
          attempts++
          return attempts === 1
            ? HttpResponse.json(GRANT)
            : HttpResponse.json({ code, detail: 'private-server-detail' }, { status })
        }),
      )
      const { user } = await openArtifacts()
      await user.click(screen.getByRole('button', { name: '获取下载链接' }))
      await screen.findByRole('link')
      await user.click(screen.getByRole('button', { name: '获取下载链接' }))
      expect(await screen.findByRole('alert')).toHaveTextContent(message)
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      expect(screen.queryByText(/private-server-detail/)).not.toBeInTheDocument()
      expect(attempts).toBe(2)
    },
  )

  it.each(['javascript:alert(1)', 'https://user:password@example.test/file', 'not-a-url'])(
    'does not render an unsafe download URL %s',
    async (url) => {
      server.use(http.get(LIST + '/3', () => HttpResponse.json({ ...GRANT, url })))
      const { user } = await openArtifacts()
      await user.click(screen.getByRole('button', { name: '获取下载链接' }))
      await screen.findByRole('alert')
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    },
  )
})
