import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { describe, expect, it } from 'vitest'
import { fakeBackend } from '@/test/backend'
import Page from './page'
import { initialGraph } from './draft'

const tenant = '00000000-0000-4000-8000-000000000001'
const id = '00000000-0000-4000-8000-000000000002'
const root = `/api/v1/tenants/${tenant}/workflows`
const workflow = {
  id, tenantId: tenant, name: '整理反馈', description: '提炼关键信息', graph: initialGraph, version: 1,
  createdAt: '2026-10-29T00:00:00Z', updatedAt: '2026-10-29T00:00:00Z', deletedAt: null,
}
const teams = { status: 200, data: { items: [{ id: tenant, name: '演示团队', status: 'active', role: 'owner' }], nextCursor: '' } }
const renderPage = (search = '') => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/workflows', element: <Page /> }], { initialEntries: [`/workflows${search}`] })
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

describe('workflow assets', () => {
  it('creates a draft through the generated API and opens its durable URL', async () => {
    let created = false
    const seen = fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root && c.method === 'post') {
        const body = JSON.parse(c.data)
        expect(body.name).toBe('整理反馈')
        expect(body.graph).toEqual(initialGraph)
        created = true
        return { status: 200, data: { resource: workflow } }
      }
      if (c.url === root) return { status: 200, data: { items: created ? [workflow] : [], nextCursor: '' } }
      if (c.url === root + '/' + id) return { status: 200, data: workflow }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    const router = renderPage()
    await userEvent.click(await screen.findByRole('link', { name: '新建工作流' }))
    await userEvent.type(screen.getByRole('textbox', { name: '名称' }), '整理反馈')
    expect(screen.getByRole('button', { name: '发布版本' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    await screen.findByRole('heading', { name: '整理反馈' })
    expect(router.state.location.search).toContain(id)
    expect(seen).toContain('POST ' + root)
    expect(screen.getByRole('button', { name: '发布版本' })).toBeEnabled()
  })

  it('retains unsaved text on conflicts and blocks navigation until explicitly discarded', async () => {
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [workflow], nextCursor: '' } }
      if (c.url === root + '/' + id && c.method === 'put') return { status: 409, data: { code: 'version_conflict' } }
      if (c.url === root + '/' + id) return { status: 200, data: workflow }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage(`?tenant=${tenant}&workflow=${id}`)
    const input = await screen.findByRole('textbox', { name: '描述' })
    await userEvent.clear(input)
    await userEvent.type(input, '本地未保存内容')
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('草稿已有新版本')
    expect(input).toHaveValue('本地未保存内容')
    await userEvent.click(screen.getByRole('link', { name: '新建工作流' }))
    expect(await screen.findByRole('alertdialog', { name: '未保存的修改' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '继续编辑' }))
    expect(input).toHaveValue('本地未保存内容')
    await userEvent.click(screen.getByRole('link', { name: '新建工作流' }))
    await userEvent.click(screen.getByRole('button', { name: '放弃修改并离开' }))
    expect(await screen.findByRole('heading', { name: '为你的流程起个名字' })).toBeInTheDocument()
  })

  it('preserves an editor buffer when checking the server fails', async () => {
    let failRead = false
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [workflow], nextCursor: '' } }
      if (c.url === root + '/' + id) return failRead ? { status: 500 } : { status: 200, data: workflow }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage(`?workflow=${id}`)
    const input = await screen.findByRole('textbox', { name: '描述' })
    await userEvent.type(input, '仍须保留')
    failRead = true
    await userEvent.click(screen.getByRole('button', { name: '检查最新版本' }))
    await screen.findByRole('alert')
    expect(input).toHaveValue('提炼关键信息仍须保留')
  })

  it('retries a failed list and does not mistake it for an empty list', async () => {
    let fail = true
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return fail ? { status: 500 } : { status: 200, data: { items: [workflow], nextCursor: '' } }
    })
    renderPage()
    await screen.findByRole('alert')
    expect(screen.queryByText('还没有工作流，创建第一个吧。')).not.toBeInTheDocument()
    fail = false
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
    expect(await screen.findByRole('link', { name: /整理反馈/ })).toBeInTheDocument()
  })
})
