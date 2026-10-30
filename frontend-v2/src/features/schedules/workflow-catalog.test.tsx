import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { ContractsClientProvider } from '@/lib/workflows/data/contracts-client-provider'
import { createLocalContractsClient } from '@/lib/workflows/data/contracts-client'
import { LocalWorkflowBackend } from '@/lib/workflows/data/local-workflow-backend'
import { ScheduleForm } from './schedule-form'
import { catalog, targetFromDraft, type Draft, type Catalog } from './schedule-model'

function setup() {
  let stored: string | null = null
  const backend = new LocalWorkflowBackend({ storage: { load: () => stored, save: value => { stored = value } } })
  const client = createLocalContractsClient(backend)
  const onSave = vi.fn<(draft: Draft, data: Catalog) => void>()
  const mount = () => render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><ContractsClientProvider client={client}><ScheduleForm onSave={onSave} onCancel={() => {}} /></ContractsClientProvider></QueryClientProvider>)
  return { backend, client, onSave, mount }
}

function graph(version: number) {
  return JSON.stringify({ nodes: [
    { id: 'start', type: 'workflow', position: { x: 0, y: 0 }, data: { kind: 'start', title: '开始', inputVariables: version === 1 ? [
      { name: 'repository', displayName: '仓库', valueType: 'string', required: true, value: 'default/repo' },
    ] : [
      { name: 'count', displayName: '次数', valueType: 'integer', required: true, value: 3 },
      { name: 'enabled', displayName: '开启', valueType: 'boolean' },
      { name: 'options', displayName: '选项', valueType: 'object' },
    ] } },
    { id: 'end', type: 'workflow', position: { x: 200, y: 0 }, data: { kind: 'output', title: '输出' } },
  ], edges: [{ id: 'edge', source: 'start', target: 'end' }] })
}

it('reads shared publications and the selected version’s Start schema, validating and saving only its inputs', async () => {
  const { backend, mount, onSave } = setup()
  const workflowId = backend.create({ name: '共享工作流', graph: graph(1) }).workflow.id
  backend.publish({ workflowId, version: 'v1' })
  backend.updateDraft({ workflowId, graph: graph(2) })
  backend.publish({ workflowId, version: 'v2' })
  backend.create({ name: '尚未发布的草稿', graph: null })
  mount()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /运行工作流/ }))
  expect(await screen.findByRole('button', { name: /尚未发布的草稿/ })).toBeDisabled()
  await user.type(screen.getByLabelText('任务名称'), '我的计划')
  await user.click(screen.getByRole('button', { name: /共享工作流.*v2/ }))
  expect(await screen.findByLabelText('次数 *')).toHaveValue(3)
  expect(screen.getByLabelText('开启（可选）')).not.toBeChecked()
  await user.selectOptions(screen.getByLabelText('关联项目'), 'website')
  await user.type(screen.getByLabelText('选项（可选）'), 'not JSON')
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(screen.getByText(/请输入有效的选项/)).toBeInTheDocument()
  expect(onSave).not.toHaveBeenCalled()
  await user.selectOptions(screen.getByLabelText('固定执行版本'), 'v1')
  expect(await screen.findByLabelText('仓库 *')).toHaveValue('default/repo')
  expect(screen.queryByLabelText('次数 *')).not.toBeInTheDocument()
  await user.clear(screen.getByLabelText('仓库 *'))
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  expect(screen.getByText('请填写仓库')).toBeInTheDocument()
  await user.type(screen.getByLabelText('仓库 *'), 'my/repo')
  await user.click(screen.getByRole('button', { name: '创建并启用' }))
  const [draft, data] = onSave.mock.calls[0]
  expect(targetFromDraft(draft, data)).toEqual({ kind: 'workflow', workflowId, snapshot: 'v1', inputs: { repository: 'my/repo' } })
  expect(draft.name).toBe('我的计划')
  expect(catalog.workflows).toEqual([])
})

it('shows empty and no-match states without inventing workflows', async () => {
  const { mount } = setup()
  mount()
  await userEvent.click(screen.getByRole('button', { name: /运行工作流/ }))
  expect(await screen.findByText('暂无已发布工作流，请先在工作流页面完成发布。')).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText('选择工作流'), '不存在')
  expect(screen.getByText('没有匹配的工作流')).toBeInTheDocument()
})

it('blocks saving when a listed publication can no longer be read', async () => {
  const { backend, client, mount, onSave } = setup()
  const workflowId = backend.create({ name: '失效工作流', graph: graph(1) }).workflow.id
  backend.publish({ workflowId, version: 'v1' })
  vi.spyOn(client.workflow, 'getVersion').mockRejectedValue(new Error('version unavailable'))
  mount()
  await userEvent.click(screen.getByRole('button', { name: /运行工作流/ }))
  await userEvent.click(await screen.findByRole('button', { name: /失效工作流.*v1/ }))
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('工作流或发布版本已不可用'))
  expect(screen.getByRole('button', { name: '创建并启用' })).toBeDisabled()
  expect(onSave).not.toHaveBeenCalled()
})
