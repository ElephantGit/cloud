import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkflowEditorState } from '@/features/workflows/editor/use-workflow-editor-state'
import type {
  WorkflowCanvasEdge,
  WorkflowCanvasNode,
} from '@/features/workflows/editor/canvas-types'
import { TEST_TENANT_ID, workflowFixture } from '@/test/cloud-handlers'
import { server } from '@/test/msw-server'

/** One plain card of the given kind at x along the top row. */
function node(id: string, kind: 'start' | 'aggregator' | 'output', x: number): WorkflowCanvasNode {
  return {
    id,
    type: 'workflow',
    position: { x, y: 0 },
    data: { kind, title: id, description: '' },
  }
}

/** Start → aggregator → output, the smallest graph whose middle node carries two edges. */
function graphFixture(): { nodes: WorkflowCanvasNode[]; edges: WorkflowCanvasEdge[] } {
  return {
    nodes: [node('start', 'start', 0), node('pick', 'aggregator', 300), node('out', 'output', 600)],
    edges: [
      { id: 'e-start-pick', source: 'start', target: 'pick' },
      { id: 'e-pick-out', source: 'pick', target: 'out' },
    ],
  }
}

/** Mounts the editor state over the fixture graph with a query client of its own. */
function renderEditorState() {
  const { nodes, edges } = graphFixture()
  const workflow = workflowFixture({ graph: { nodes, edges, viewport: { x: 0, y: 0, zoom: 1 } } })
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return renderHook(() => useWorkflowEditorState(TEST_TENANT_ID, workflow), { wrapper })
}

/** Ids on the canvas right now, sorted so assertions read the same in any order. */
function idsOf(items: readonly { id: string }[]): string[] {
  return items.map((item) => item.id).toSorted()
}

/** Deletes `pick` the way React Flow does: the canvas opens the transaction, edges go, then the node. */
function deletePickInFlowOrder(result: { current: ReturnType<typeof useWorkflowEditorState> }) {
  act(() => result.current.onHistoryDeleteStart(['pick']))
  act(() =>
    result.current.onEdgesChange([
      { type: 'remove', id: 'e-start-pick' },
      { type: 'remove', id: 'e-pick-out' },
    ]),
  )
  act(() => result.current.onNodesChange([{ type: 'remove', id: 'pick' }]))
}

describe('useWorkflowEditorState deletion history', () => {
  beforeEach(() => {
    // Autosave may fire while a test runs; answer it like the server would.
    server.use(
      http.put(`/api/v1/tenants/${TEST_TENANT_ID}/workflows/:wfid`, () =>
        HttpResponse.json({ resource: workflowFixture({ version: 2 }) }),
      ),
    )
  })

  it('records a node deletion and the edges it takes along as one undo step', () => {
    const { result } = renderEditorState()
    deletePickInFlowOrder(result)
    act(() => result.current.onHistoryDeleteEnd())

    expect(idsOf(result.current.nodes)).toEqual(['out', 'start'])
    expect(result.current.edges).toEqual([])
    expect(result.current.history.past).toHaveLength(1)
    expect(result.current.history.currentEvent).toBe('node.delete')

    act(() => result.current.history.undo())
    expect(idsOf(result.current.nodes)).toEqual(['out', 'pick', 'start'])
    expect(idsOf(result.current.edges)).toEqual(['e-pick-out', 'e-start-pick'])
    expect(result.current.history.canUndo).toBe(false)
  })

  it('keeps an edge-only deletion as its own edge step', () => {
    const { result } = renderEditorState()
    act(() => result.current.onEdgesChange([{ type: 'remove', id: 'e-pick-out' }]))
    act(() => result.current.onHistoryDeleteEnd())

    expect(result.current.history.past).toHaveLength(1)
    expect(result.current.history.currentEvent).toBe('edge.delete')
    act(() => result.current.history.undo())
    expect(idsOf(result.current.edges)).toEqual(['e-pick-out', 'e-start-pick'])
  })

  it('records later deletions normally once a deletion transaction has closed', () => {
    const { result } = renderEditorState()
    act(() => result.current.onHistoryDeleteStart(['out']))
    act(() => result.current.onEdgesChange([{ type: 'remove', id: 'e-pick-out' }]))
    act(() => result.current.onNodesChange([{ type: 'remove', id: 'out' }]))
    act(() => result.current.onHistoryDeleteEnd())
    act(() => result.current.onEdgesChange([{ type: 'remove', id: 'e-start-pick' }]))

    expect(result.current.history.past).toHaveLength(2)
    expect(result.current.history.currentEvent).toBe('edge.delete')
  })

  it('closes the deletion with the node removals even without onDelete', () => {
    const { result } = renderEditorState()
    deletePickInFlowOrder(result)

    expect(result.current.history.past).toHaveLength(1)
    // The edit after it is its own step, not swallowed by a transaction left open.
    act(() => result.current.onNodesChange([{ type: 'remove', id: 'out' }]))
    expect(result.current.history.past).toHaveLength(2)
  })

  it('commits a deletion a failed stream left open before the next one begins', () => {
    const { result } = renderEditorState()
    act(() => result.current.onHistoryDeleteStart(['out']))
    // The edges went, but the node stream never arrived.
    act(() => result.current.onEdgesChange([{ type: 'remove', id: 'e-pick-out' }]))
    act(() => result.current.onHistoryDeleteStart(['pick']))
    act(() => result.current.onEdgesChange([{ type: 'remove', id: 'e-start-pick' }]))
    act(() => result.current.onNodesChange([{ type: 'remove', id: 'pick' }]))

    expect(result.current.history.past).toHaveLength(2)
    act(() => result.current.history.undo())
    expect(idsOf(result.current.nodes)).toEqual(['out', 'pick', 'start'])
    expect(idsOf(result.current.edges)).toEqual(['e-start-pick'])
  })
})
