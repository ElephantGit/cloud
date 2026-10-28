import { describe, expect, it, vi } from 'vitest'
import { resolveCanvasDeletion } from '@/features/workflows/editor/workflow-canvas-deletion'
import type {
  WorkflowCanvasEdge,
  WorkflowCanvasNode,
} from '@/features/workflows/editor/canvas-types'
import type { WorkflowNodeKind } from '@/features/workflows/runtime/types'

/** One card; a member carries both membership keys the way the editor writes them. */
function node(id: string, kind: WorkflowNodeKind, owner?: string): WorkflowCanvasNode {
  return {
    id,
    type: 'workflow',
    position: { x: 0, y: 0 },
    ...(owner === undefined ? {} : { parentId: owner }),
    data: {
      kind,
      title: id,
      description: '',
      ...(owner === undefined ? {} : { containerId: owner }),
    },
  }
}

/** A plain edge named after its two ends. */
function edge(source: string, target: string): WorkflowCanvasEdge {
  return { id: `${source}-${target}`, source, target }
}

/** Start → loop (body: entry → step) → output, and an iteration with one member. */
function graph() {
  const graphNodes = [
    node('start', 'start'),
    node('lp', 'loop'),
    node('entry', 'start', 'lp'),
    node('step', 'aggregator', 'lp'),
    node('it', 'iteration'),
    node('member', 'aggregator', 'it'),
    node('out', 'output'),
  ]
  const graphEdges = [
    edge('start', 'lp'),
    edge('entry', 'step'),
    edge('lp', 'it'),
    edge('it', 'out'),
  ]
  return { graphNodes, graphEdges }
}

/** Editor callbacks with the iteration confirmation answering `confirm`. */
function services(confirm: boolean) {
  return {
    onConfirmIterationDeletion: vi.fn<(memberCount: number) => Promise<boolean>>(() =>
      Promise.resolve(confirm),
    ),
    onHistoryDeleteStart: vi.fn<(nodeIds: string[]) => void>(),
    showIterationHint: vi.fn<() => void>(),
  }
}

/** The deletion React Flow would request for these node ids. */
function requestFor(editor: ReturnType<typeof graph>, ids: string[]) {
  return { nodes: editor.graphNodes.filter((candidate) => ids.includes(candidate.id)), edges: [] }
}

describe('resolveCanvasDeletion', () => {
  it('opens the history transaction for a plain deletion with the deleted node', async () => {
    const editor = graph()
    const calls = services(true)
    const resolved = await resolveCanvasDeletion(requestFor(editor, ['out']), editor, false, calls)

    expect(resolved).toBe(true)
    expect(calls.onHistoryDeleteStart).toHaveBeenCalledExactlyOnceWith(['out'])
  })

  it('opens it with every node a container deletion cascades to', async () => {
    const editor = graph()
    const calls = services(true)
    const resolved = await resolveCanvasDeletion(requestFor(editor, ['lp']), editor, false, calls)

    expect(resolved).not.toBe(true)
    expect(resolved).not.toBe(false)
    const ids = calls.onHistoryDeleteStart.mock.calls[0]?.[0] ?? []
    expect(ids.toSorted()).toEqual(['entry', 'lp', 'step'])
  })

  it('opens nothing when the iteration confirmation is cancelled', async () => {
    const editor = graph()
    const calls = services(false)
    const resolved = await resolveCanvasDeletion(requestFor(editor, ['it']), editor, false, calls)

    expect(calls.onConfirmIterationDeletion).toHaveBeenCalledExactlyOnceWith(1)
    expect(resolved).toBe(false)
    expect(calls.onHistoryDeleteStart).not.toHaveBeenCalled()
  })

  it('opens nothing on a read-only canvas', async () => {
    const editor = graph()
    const calls = services(true)
    const resolved = await resolveCanvasDeletion(requestFor(editor, ['out']), editor, true, calls)

    expect(resolved).toBe(false)
    expect(calls.onHistoryDeleteStart).not.toHaveBeenCalled()
  })
})
