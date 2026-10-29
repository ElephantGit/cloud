import type { CanvasNode, Graph, Edge } from './workflow-canvas'

export function parseCanvasGraph(text: string): { graph: Graph | null; problem: string } {
  try {
    const g: unknown = JSON.parse(text)
    if (!g || typeof g !== 'object' || Array.isArray(g)) return { graph: null, problem: '文档必须是对象' }
    const value = g as Record<string, unknown>
    const finite = (number: unknown) => typeof number === 'number' && Number.isFinite(number)
    const position = (item: unknown) => !!item && typeof item === 'object' && !Array.isArray(item) && finite((item as Record<string, unknown>).x) && finite((item as Record<string, unknown>).y)
    const nodes = Array.isArray(value.nodes) && value.nodes.every((node: unknown) => {
      if (!node || typeof node !== 'object' || Array.isArray(node)) return false
      const item = node as Record<string, unknown>
      const size = (key: string) => item[key] === undefined || (finite(item[key]) && (item[key] as number) > 0)
      return typeof item.id === 'string' && item.id.length > 0 && position(item.position) && !!item.data && typeof item.data === 'object' && !Array.isArray(item.data) && typeof (item.data as Record<string, unknown>).kind === 'string' && (item.parentId === undefined || typeof item.parentId === 'string') && size('width') && size('height') && size('initialWidth') && size('initialHeight')
    })
    const edges = Array.isArray(value.edges) && value.edges.every((edge: unknown) => {
      if (!edge || typeof edge !== 'object' || Array.isArray(edge)) return false
      const item = edge as Record<string, unknown>
      return typeof item.id === 'string' && typeof item.source === 'string' && typeof item.target === 'string' && (item.sourceHandle == null || typeof item.sourceHandle === 'string') && (item.targetHandle == null || typeof item.targetHandle === 'string')
    })
    const view = value.viewport
    const viewport = view === undefined || (!!view && typeof view === 'object' && !Array.isArray(view) && position(view) && finite((view as Record<string, unknown>).zoom) && ((view as Record<string, unknown>).zoom as number) > 0)
    if (!nodes) return { graph: null, problem: '节点格式不受支持' }
    const nodeIds = (value.nodes as CanvasNode[]).map((node) => node.id)
    if (new Set(nodeIds).size !== nodeIds.length) return { graph: null, problem: '节点 ID 重复' }
    if (!edges) return { graph: null, problem: '连线格式不受支持' }
    const graphEdges = value.edges as Edge[]
    if (new Set(graphEdges.map((edge) => edge.id)).size !== graphEdges.length || graphEdges.some((edge) => !nodeIds.includes(edge.source) || !nodeIds.includes(edge.target))) return { graph: null, problem: '连线 ID 重复或引用了不存在的节点' }
    if (!viewport) return { graph: null, problem: '视图格式不受支持' }
    return { graph: value as Graph, problem: '' }
  } catch { return { graph: null, problem: '文档不是有效 JSON' } }
}
/** Returns a user-facing reason when a graph cannot be rendered without changing it. */
export function validateCanvasGraph(text: string): string | null { return parseCanvasGraph(text).problem || null }
