import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import type { WorkflowForm } from '@/lib/workflows/api/generated'
import { RunForm } from './run-form'

const api = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn(), submit: vi.fn(), retry: vi.fn() }))
vi.mock('@/lib/workflows/api/generated', () => ({ getWorkflowForm: api.read, saveWorkflowFormDraft: api.save, submitWorkflowForm: api.submit, retryWorkflowFormTasks: api.retry }))
vi.mock('@/components/task-assignee-picker', () => ({ TaskAssigneePicker: () => <span>负责人选择</span> }))
vi.mock('@/api/members/members', () => ({ useGetApiV1TenantsTidMembers: () => ({ data: { items: [{ userId: 'admin', displayName: '管理员' }] } }) }))
vi.mock('@/lib/squads-api/generated', () => ({ listSquads: async () => ({ items: [] }) }))
vi.mock('./dts-receipt', () => ({ DtsReceipt: () => null }))

let form: WorkflowForm
beforeEach(() => {
  vi.clearAllMocks()
  form = {
    id: 'form', runId: 'run', nodeId: 'review', reviewerId: 'reviewer', version: 1, status: 'open', canEdit: true,
    report: '报告正文', submittedBy: '', results: [], answers: [],
    config: { title: '确认问题', submitLabel: '提交', reportSelector: ['report', 'output'], itemsSelector: ['report', 'findings'], fields: [{ name: 'notes', label: '备注', type: 'textarea', required: false }] },
    findings: [{ key: 'disk', title: '磁盘异常', description: '检查占用', evidence: '86%', severity: '中', module: '存储' }],
  }
  api.read.mockImplementation(async () => structuredClone(form))
})

function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  render(<QueryClientProvider client={client}><MemoryRouter><RunForm tenantId="tenant" runId="run" runStatus="awaiting_input" /></MemoryRouter></QueryClientProvider>)
  return client
}

it('renders authoritative submitted answers without losing an unsaved local draft', async () => {
  const client = mount()
  fireEvent.change(await screen.findByLabelText('是否为问题'), { target: { value: 'confirmed' } })
  form = { ...form, version: 2, status: 'submitted', canEdit: false, submittedBy: 'admin', answers: [{ key: 'disk', decision: 'rejected', values: { notes: '管理员判定无需处理' } }] }
  await act(async () => { await client.invalidateQueries({ queryKey: ['workflow-form', 'tenant', 'run'] }) })
  await waitFor(()=>expect(screen.getByLabelText('是否为问题')).toHaveValue('rejected'))
  expect(screen.getByLabelText('备注')).toHaveValue('管理员判定无需处理')
  expect(screen.getByText('查看未保存的本地修改')).toBeInTheDocument()
  expect(screen.getByText(/磁盘异常：是问题/)).toBeInTheDocument()
  expect(api.submit).not.toHaveBeenCalled()
})

it('refreshes a clean draft when another reviewer saves a newer version', async () => {
  const client = mount()
  await screen.findByLabelText('备注')
  form = { ...form, version: 2, answers: [{ key: 'disk', decision: 'rejected', values: { notes: '最新草稿' } }] }
  await act(async () => { await client.invalidateQueries({ queryKey: ['workflow-form', 'tenant', 'run'] }) })
  await waitFor(()=>expect(screen.getByLabelText('备注')).toHaveValue('最新草稿'))
  expect(screen.queryByText('查看未保存的本地修改')).not.toBeInTheDocument()
})
