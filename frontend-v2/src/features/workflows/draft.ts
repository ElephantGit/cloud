import { createStore } from 'zustand/vanilla'
import type { Workflow } from '@/api/generated.schemas'

export const initialGraph = {
  nodes: [{ id: 'start', type: 'workflow', deletable: false, position: { x: 80, y: 80 },
    data: { kind: 'start', title: '开始', description: '', inputVariables: [] } }],
  edges: [],
  viewport: { x: 0, y: 0, zoom: 1 },
}

export const savedFields = (workflow?: Workflow) => ({
  name: workflow?.name ?? '',
  description: workflow?.description ?? '',
  graph: JSON.stringify(workflow?.graph ?? initialGraph, null, 2),
  version: workflow?.version ?? 0,
})

/** Each editor owns its unsaved buffer; background refetches never replace it. */
export function createDraftStore(workflow?: Workflow) {
  return createStore(() => ({
    ...savedFields(workflow),
    dirty: false,
    error: null as unknown,
    message: '',
    archiveConfirm: false,
  }))
}

export function parseGraph(text: string): Record<string, unknown> {
  let value: unknown
  try { value = JSON.parse(text) } catch { throw new Error('流程文档不是有效的 JSON，请检查格式。') }
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('流程文档必须是 JSON 对象。')
  return value as Record<string, unknown>
}
