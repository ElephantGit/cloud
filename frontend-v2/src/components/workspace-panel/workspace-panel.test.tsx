import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { fakeBackend } from '@/test/backend'
import { type OpenRequest, WorkspacePanel } from './workspace-panel'

const base = '/api/v2/tenants/t1/chat/conversations/c1/workspace'
const source = { kind: 'chat' as const, tenantId: 't1', conversationId: 'c1' }

const changes = {
  baseCommit: 'a'.repeat(40),
  truncated: false,
  files: [
    { path: 'README.md', status: 'modified', additions: 1, deletions: 0, binary: false },
    { path: 'src/app.go', status: 'added', additions: 2, deletions: 0, binary: false },
  ],
}

const renderPanel = (props: { revision?: string; request?: OpenRequest } = {}) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const ui = (p: typeof props) => (
    <QueryClientProvider client={client}>
      <WorkspacePanel source={source} revision={p.revision ?? '0'} request={p.request} onClose={() => {}} />
    </QueryClientProvider>
  )
  const view = render(ui(props))
  return { rerender: (p: typeof props) => view.rerender(ui(p)) }
}

describe('workspace panel', () => {
  let seen: string[]
  let ready: boolean
  beforeEach(() => {
    ready = true
    seen = fakeBackend((config) => {
      if (!ready) return { status: 404, data: { code: 'workspace_not_ready', params: {}, requestId: 'r' } }
      const path = (config.params as { path?: string } | undefined)?.path ?? ''
      switch (config.url) {
        case `${base}/changes`:
          return { status: 200, data: changes }
        case `${base}/diff`:
          return { status: 200, data: { path, patch: `diff --git a/${path} b/${path}\n--- a/${path}\n+++ b/${path}\n@@ -1 +1,2 @@\n hello\n+agent\n` } }
        case `${base}/entries`:
          return path === ''
            ? { status: 200, data: { path: '', truncated: false, entries: [{ name: 'src', path: 'src', kind: 'directory' }, { name: 'README.md', path: 'README.md', kind: 'file' }] } }
            : { status: 200, data: { path, truncated: false, entries: [{ name: 'app.go', path: 'src/app.go', kind: 'file' }] } }
        case `${base}/file`:
          return { status: 200, data: { path, content: 'package src\n', sizeBytes: 12 } }
      }
    })
  })

  it('expands several diffs at once and opens a file in full', async () => {
    renderPanel()
    const list = await screen.findByLabelText('改动的文件')
    await userEvent.click(within(list).getByText('README.md'))
    expect(await screen.findByRole('region', { name: 'README.md 的改动' })).toHaveTextContent('agent')
    await userEvent.click(within(list).getByText('app.go'))
    expect(await screen.findByRole('region', { name: 'src/app.go 的改动' })).toBeInTheDocument()
    // Opening the second left the first open.
    expect(screen.getByRole('region', { name: 'README.md 的改动' })).toBeInTheDocument()
    await userEvent.click(within(screen.getByRole('region', { name: 'README.md 的改动' }).parentElement!).getByRole('button', { name: '查看文件' }))
    expect(await screen.findByRole('region', { name: 'README.md 的内容' })).toHaveTextContent('package src')
    await userEvent.click(screen.getByRole('button', { name: '查看改动' }))
    expect(await screen.findByRole('region', { name: 'README.md 的改动' })).toBeInTheDocument()
  })

  it('browses the tree and reads a file', async () => {
    renderPanel()
    await userEvent.click(screen.getByRole('button', { name: '文件' }))
    const tree = await screen.findByRole('tree', { name: '工作区文件' })
    await userEvent.click(await within(tree).findByText('src'))
    await userEvent.click(await within(tree).findByText('app.go'))
    expect(await screen.findByRole('region', { name: 'src/app.go 的内容' })).toHaveTextContent('package src')
    expect(seen).toContain(`GET ${base}/file`)
  })

  it('opens the diff an outside request names, resolving an absolute Agent path', async () => {
    const view = renderPanel()
    await screen.findByLabelText('改动的文件')
    view.rerender({ request: { path: '/data/nodes/x/w/r/c-1/src/app.go', nonce: 1 } })
    expect(await screen.findByRole('region', { name: 'src/app.go 的改动' })).toBeInTheDocument()
  })

  it('says the workspace is not ready, then reads it again on a new revision', async () => {
    ready = false
    const view = renderPanel()
    expect(await screen.findByText(/工作区在第一轮开始后才会准备好/)).toBeInTheDocument()
    ready = true
    view.rerender({ revision: '1' })
    expect(await screen.findByLabelText('改动的文件')).toBeInTheDocument()
  })
})
