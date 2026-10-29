import type { AxiosAdapter } from 'axios'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { describe, expect, it } from 'vitest'
import { http } from '@/lib/api-client'
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
  it('serializes saves and retains edits made while the first request is in flight', async () => {
    let release: (() => void) | undefined
    let server = { ...workflow }
    const writes: { description: string; version: number }[] = []
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [server], nextCursor: '' } }
      if (c.url === root + '/' + id) return { status: 200, data: server }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    const fallback = http.defaults.adapter!
    http.defaults.adapter = async (c) => {
      if (c.url === root + '/' + id && c.method === 'put') {
        const body = JSON.parse(c.data)
        writes.push(body)
        if (writes.length === 1) await new Promise<void>((resolve) => { release = resolve })
        server = { ...server, description: body.description, graph: body.graph, version: server.version + 1 }
        return { status: 200, statusText: '200', data: server, headers: {}, config: c }
      }
      return (fallback as AxiosAdapter)(c)
    }
    renderPage('?workflow=' + id)
    const input = await screen.findByRole('textbox', { name: '描述' })
    fireEvent.change(input, { target: { value: '第一次' } })
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    await waitFor(() => expect(writes).toHaveLength(1))
    fireEvent.change(input, { target: { value: '第二次' } })
    release!()
    await waitFor(() => expect(writes).toHaveLength(2))
    expect(writes.map((w) => [w.description, w.version])).toEqual([['第一次', 1], ['第二次', 2]])
    await waitFor(() => expect(screen.getByText('草稿已保存。')).toBeInTheDocument())
    expect(input).toHaveValue('第二次')
  })

  it('imports the desktop document without losing container or unknown fields', async () => {
    const document = { id: 'desktop-id', name: '桌面示例', custom: { retained: true }, nodes: [{ id: 'start', type: 'workflow', position: { x: 0, y: 0 }, data: { kind: 'start' } }, { id: 'loop', type: 'workflow', position: { x: 100, y: 100 }, data: { kind: 'loop' } }, { id: 'inner', type: 'workflow', parentId: 'loop', position: { x: 5, y: 5 }, data: { kind: 'start', containerId: 'loop', extension: 42 } }], edges: [], annotations: [], globalVariables: [] }
    let stored: unknown
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root && c.method === 'post') { stored = JSON.parse(c.data).graph; return { status: 200, data: { resource: { ...workflow, graph: stored } } } }
      if (c.url === root) return { status: 200, data: { items: [], nextCursor: '' } }
      if (c.url === root + '/' + id) return { status: 200, data: { ...workflow, graph: stored } }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage('?workflow=new')
    await userEvent.type(await screen.findByRole('textbox', { name: '名称' }), '导入示例')
    await userEvent.click(screen.getByText('流程 JSON 和导入'))
    await userEvent.upload(screen.getByLabelText('导入 JSON 文件'), new File([JSON.stringify(document)], 'desktop.reactflow.json', { type: 'application/json' }))
    expect(await screen.findByRole('dialog', { name: '导入预览' })).toHaveTextContent('3 个节点 · 0 条连线')
    await userEvent.click(screen.getByRole('button', { name: '确认导入' }))
    await waitFor(() => expect(screen.getByRole('textbox', { name: '流程 JSON' })).toHaveValue(JSON.stringify(document, null, 2)))
    await userEvent.click(screen.getByRole('button', { name: '保存草稿' }))
    await waitFor(() => expect(stored).toEqual(document))
  })

  it('saves the current buffer before leaving a dirty draft', async () => {
    let persisted = ''
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [workflow], nextCursor: '' } }
      if (c.url === root + '/' + id && c.method === 'put') { persisted = JSON.parse(c.data).description; return { status: 200, data: { ...workflow, description: persisted, version: 2 } } }
      if (c.url === root + '/' + id) return { status: 200, data: workflow }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage('?workflow=' + id)
    const description = await screen.findByRole('textbox', { name: '描述', hidden: true })
    fireEvent.change(description, { target: { value: '离开前保存' } })
    await userEvent.click(screen.getByRole('link', { name: '新建工作流' }))
    await userEvent.click(await screen.findByRole('button', { name: '保存并离开' }))
    await screen.findByRole('heading', { name: '为你的流程起个名字' })
    expect(persisted).toBe('离开前保存')
  })

  it('searches through every cursor page', async () => {
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return c.params?.after ? { status: 200, data: { items: [{ ...workflow, id: 'next', name: '第二页目标' }], nextCursor: '' } } : { status: 200, data: { items: [workflow], nextCursor: 'next-page' } }
    })
    renderPage()
    await userEvent.type(await screen.findByRole('textbox', { name: '搜索工作流' }), '第二页')
    expect(await screen.findByRole('link', { name: '第二页目标' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /整理反馈/ })).not.toBeInTheDocument()
  })

  it('flushes a dirty draft before publishing', async () => {
    let server = { ...workflow }
    let publishedDescription = ''
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [server], nextCursor: '' } }
      if (c.url === root + '/' + id && c.method === 'put') { server = { ...server, description: JSON.parse(c.data).description, version: server.version + 1 }; return { status: 200, data: server } }
      if (c.url === root + '/' + id + '/publish') { publishedDescription = server.description; return { status: 200, data: { resource: { id: 'snapshot', version: 1 } } } }
      if (c.url === root + '/' + id) return { status: 200, data: server }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage('?workflow=' + id)
    const description = await screen.findByRole('textbox', { name: '描述', hidden: true })
    fireEvent.change(description, { target: { value: '发布前的新内容' } })
    await userEvent.click(screen.getByRole('button', { name: '发布版本' }))
    expect(await screen.findByText('版本 v1 已发布，内容已冻结。')).toBeInTheDocument()
    expect(publishedDescription).toBe('发布前的新内容')
  })

  it('does not overwrite an unsupported global variables shape', async () => {
    const odd = { ...workflow, graph: { ...initialGraph, globalVariables: { legacy: true } } }
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [odd], nextCursor: '' } }
      if (c.url === root + '/' + id) return { status: 200, data: odd }
      if (c.url?.endsWith('/snapshots')) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage('?workflow=' + id)
    await userEvent.click(await screen.findByRole('button', { name: '全局变量' }))
    expect(screen.getByRole('alertdialog', { name: '全局变量格式不支持' })).toHaveTextContent('原始数据已保留')
    expect(screen.queryByRole('button', { name: /^保存$/ })).not.toBeInTheDocument()
  })

  it('previews, cancels, confirms and undoes a desktop import without losing the draft', async () => {
    fakeBackend((c) => {
      if (c.url === '/api/v1/me/tenants') return teams
      if (c.url === root) return { status: 200, data: { items: [], nextCursor: '' } }
    })
    renderPage('?workflow=new')
    await userEvent.click(await screen.findByText('流程 JSON 和导入'))
    const editor = screen.getByRole('textbox', { name: '流程 JSON' })
    const original = (editor as HTMLTextAreaElement).value
    const doc = { ...initialGraph, name: '导入名称', nodes: [...initialGraph.nodes, { id: 'agent-1', type: 'workflow', position: { x: 200, y: 100 }, data: { kind: 'agent' } }] }
    const file = new File([JSON.stringify(doc)], 'test.reactflow.json', { type: 'application/json' })
    await userEvent.upload(screen.getByLabelText('导入 JSON 文件'), new File(['{'], 'bad.json', { type: 'application/json' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('不是有效的 JSON')
    expect(editor).toHaveValue(original)
    await userEvent.upload(screen.getByLabelText('导入 JSON 文件'), file)
    expect(await screen.findByRole('dialog', { name: '导入预览' })).toHaveTextContent('导入名称')
    await userEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(editor).toHaveValue(original)
    await userEvent.upload(screen.getByLabelText('导入 JSON 文件'), file)
    await userEvent.click(await screen.findByRole('button', { name: '确认导入' }))
    expect(screen.getByRole('textbox', { name: '名称' })).toHaveValue('导入名称')
    expect(editor).toHaveValue(JSON.stringify(doc, null, 2))
    await userEvent.click(screen.getByRole('button', { name: '撤销' }))
    expect(editor).toHaveValue(original)
    expect(screen.getByRole('textbox', { name: '名称' })).toHaveValue('')
    await userEvent.click(screen.getByRole('button', { name: '重做' }))
    expect(screen.getByRole('textbox', { name: '名称' })).toHaveValue('导入名称')
  })

})
