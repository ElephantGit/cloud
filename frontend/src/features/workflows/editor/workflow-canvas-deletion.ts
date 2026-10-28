import {
  repairIterationGraphAfterNodeDeletion,
  resolveIterationDeletionCascade,
} from '@/features/workflows/runtime/iteration-graph'
import type {
  WorkflowCanvasEdge,
  WorkflowCanvasNode,
} from '@/features/workflows/editor/canvas-types'

/** Human-readable notices the canvas raises while authoring iteration regions. */
export type WorkflowIterationHintKey =
  'workflows.iteration.useInternalAdd' | 'workflows.iteration.collectTargetDeleted'

/** The graph the author asked to delete, shaped for React Flow's before-delete hook. */
export type WorkflowDeletionRequest = {
  nodes: WorkflowCanvasNode[]
  edges: WorkflowCanvasEdge[]
}

/** What a deletion needs from the editor: the confirmation, the history and the hint. */
export interface WorkflowCanvasDeletionServices {
  onConfirmIterationDeletion: ((memberCount: number) => Promise<boolean>) | undefined
  onHistoryDeleteStart: ((nodeIds: string[]) => void) | undefined
  showIterationHint: (
    key: WorkflowIterationHintKey,
    params?: Record<string, string | number>,
  ) => void
}

/**
 * React Flow's `onBeforeDelete` for the editable canvas: confirms and widens the request
 * like {@link confirmAndResolveDeletion}, then opens the history transaction that folds the
 * edge removals and node removals React Flow is about to stream into one undo step.
 *
 * @param request - What React Flow is about to delete.
 * @param editor - The graph the request is resolved against.
 * @param readOnly - A preview canvas refuses every deletion.
 * @param services - Confirmation, history and hint callbacks from the editor.
 * @returns `false` to keep everything, `true` to delete as asked, or the widened request.
 */
export async function resolveCanvasDeletion(
  request: WorkflowDeletionRequest,
  editor: { graphNodes: WorkflowCanvasNode[]; graphEdges: WorkflowCanvasEdge[] },
  readOnly: boolean,
  services: WorkflowCanvasDeletionServices,
): Promise<boolean | WorkflowDeletionRequest> {
  const resolved = await confirmAndResolveDeletion(request, editor, readOnly, services)
  const removedNodes = nodesLeaving(request, resolved)
  if (removedNodes.length > 0) {
    services.onHistoryDeleteStart?.(removedNodes.map((node) => node.id))
  }
  return resolved
}

/** The nodes a resolved deletion will actually remove: none when refused. */
function nodesLeaving(
  request: WorkflowDeletionRequest,
  resolved: boolean | WorkflowDeletionRequest,
): WorkflowDeletionRequest['nodes'] {
  if (resolved === false) {
    return []
  }
  return resolved === true ? request.nodes : resolved.nodes
}

/** Resolves a deletion request (keyboard delete or a card's trash button):
 * iteration containers confirm with their member count, then drag their whole
 * region out; plain deletions hint when they orphan a region's collect target.
 */
async function confirmAndResolveDeletion(
  request: WorkflowDeletionRequest,
  editor: { graphNodes: WorkflowCanvasNode[]; graphEdges: WorkflowCanvasEdge[] },
  readOnly: boolean,
  services: {
    onConfirmIterationDeletion: ((memberCount: number) => Promise<boolean>) | undefined
    showIterationHint: (
      key: WorkflowIterationHintKey,
      params?: Record<string, string | number>,
    ) => void
  },
): Promise<boolean | WorkflowDeletionRequest> {
  if (readOnly) {
    return false
  }
  const graph = { nodes: editor.graphNodes, edges: editor.graphEdges }
  const requestedNodeIds = new Set(request.nodes.map((node) => node.id))
  const cascade = resolveIterationDeletionCascade(graph, requestedNodeIds)
  const repair = repairIterationGraphAfterNodeDeletion(graph, cascade.nodeIds)
  const deletedIteration = request.nodes.find((node) => node.data.kind === 'iteration')
  const deletedContainer =
    deletedIteration ?? request.nodes.find((node) => node.data.kind === 'loop')
  if (deletedIteration !== undefined && cascade.memberCount > 0) {
    if (services.onConfirmIterationDeletion === undefined) {
      return false
    }
    const confirmed = await services.onConfirmIterationDeletion(cascade.memberCount)
    if (!confirmed) {
      return false
    }
  }
  if (repair.clearedCollectSelectorIterationIds.length > 0) {
    const iterationId = repair.clearedCollectSelectorIterationIds[0]
    if (iterationId !== undefined) {
      const title = editor.graphNodes.find((node) => node.id === iterationId)?.data.title
      if (title !== undefined) {
        services.showIterationHint('workflows.iteration.collectTargetDeleted', { name: title })
      }
    }
  }
  if (deletedContainer === undefined) {
    // An ordinary node delete: React Flow removes it and its edges, and the
    // draft's change pipeline runs the collect-selector repair it needs.
    return true
  }
  // Deleting a container takes its members and their edges along in one step.
  return {
    nodes: [...cascade.nodeIds].flatMap((id) => {
      const node = editor.graphNodes.find((candidate) => candidate.id === id)
      return node === undefined ? [] : [node]
    }),
    edges: [...cascade.edgeIds].flatMap((id) => {
      const edge = editor.graphEdges.find((candidate) => candidate.id === id)
      return edge === undefined ? [] : [edge]
    }),
  }
}
