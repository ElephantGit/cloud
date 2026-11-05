import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { Node } from '@xyflow/react'
import type { WorkflowNodeData } from '@/lib/workflows/mock'
import { WorkflowFormFields } from './workflow-form-fields'

vi.mock('@/lib/workflows/data/contracts-client', () => ({ useContractsClient: () => ({ tenantId: 'workflow-tenant' }) }))
const roster = vi.hoisted(() => vi.fn(() => ({ data: { items: [{ userId: 'reviewer', displayName: '确认成员', status: 'active' }] } })))
vi.mock('@/api/members/members', () => ({ useGetApiV1TenantsTidMembers: roster }))

const review: Node<WorkflowNodeData, 'workflow'> = { id: 'review', type: 'workflow', position: { x: 0, y: 0 }, data: { kind: 'form', title: '问题确认', description: '' } }

it('binds typed upstream variables and members from the workflow tenant', () => {
  const update = vi.fn()
  render(<WorkflowFormFields node={review} onUpdate={update} onClose={vi.fn()} variableCatalog={[
    { selector: ['inspect', 'structured_output', 'findings'], sourceNodeId: 'inspect', sourceNodeTitle: '检查结果', variableName: 'findings', valueType: 'array[object]' },
    { selector: ['report', 'structured_output', 'reportMarkdown'], sourceNodeId: 'report', sourceNodeTitle: '报告', variableName: 'reportMarkdown', valueType: 'string' },
  ]} />)
  expect(roster).toHaveBeenCalledWith('workflow-tenant', undefined, { query: { enabled: true } })
  fireEvent.change(screen.getByLabelText('问题列表来源变量'), { target: { value: JSON.stringify(['inspect', 'structured_output', 'findings']) } })
  expect(update).toHaveBeenLastCalledWith(expect.objectContaining({ data: expect.objectContaining({ formConfig: expect.objectContaining({ itemsSelector: ['inspect', 'structured_output', 'findings'] }) }) }))
  fireEvent.change(screen.getByLabelText('确认人'), { target: { value: 'reviewer' } })
  expect(update).toHaveBeenLastCalledWith(expect.objectContaining({ data: expect.objectContaining({ formConfig: expect.objectContaining({ reviewerId: 'reviewer' }) }) }))
})

it('selects the task source by its visible form name', () => {
  const update = vi.fn()
  render(<WorkflowFormFields node={{ ...review, id: 'tasks', data: { kind: 'createTasks', title: '提单', description: '' } }} onUpdate={update} onClose={vi.fn()} variableCatalog={[]} graphNodes={[review]} />)
  expect(screen.getByRole('option', { name: '问题确认' })).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('上游表单'), { target: { value: 'review' } })
  expect(update).toHaveBeenLastCalledWith(expect.objectContaining({ data: expect.objectContaining({ createTasksConfig: { formNodeId: 'review' } }) }))
})
