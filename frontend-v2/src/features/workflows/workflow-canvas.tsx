import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from 'react'
import { ArrowDownLeft, ArrowUpRight, Hand, Minus, Plus, Redo2, Undo2, X } from 'lucide-react'
import './workflow-canvas.css'
import { parseCanvasGraph } from './workflow-graph-validation'

export type CanvasNode = { id: string; type?: string; position: { x: number; y: number }; data: Record<string, unknown>; parentId?: string; [key: string]: unknown }
export type Edge = { id: string; source: string; target: string; sourceHandle?: string; targetHandle?: string; [key: string]: unknown }
type Annotation = { id: string; type: string; position: { x: number; y: number }; data: { text: string; theme: string; [key: string]: unknown }; [key: string]: unknown }
export type Graph = { nodes: CanvasNode[]; edges: Edge[]; annotations?: unknown; viewport?: { x: number; y: number; zoom: number }; [key: string]: unknown }
export type InspectorProps = { node: CanvasNode; nodes: CanvasNode[]; edges: Edge[]; onChange: (node: CanvasNode) => void; onClose: () => void }
const kinds = [ ['start', '开始'], ['agent', 'Agent'], ['condition', '条件分支'], ['aggregator', '变量聚合'], ['loop', '循环'], ['loopExit', '退出循环'], ['iteration', '迭代'], ['output', '输出'] ] as const
const label = (kind: unknown) => kinds.find(([key]) => key === kind)?.[1] ?? String(kind ?? '节点')
const isContainer = (node: CanvasNode) => node.data.kind === 'loop' || node.data.kind === 'iteration'
type Scope = 'workflow' | 'loop' | 'iteration'
function allowsScope(kind: unknown, scope: Scope): boolean {
  if (kind === 'agent' || kind === 'condition') return true
  if (kind === 'aggregator') return scope !== 'loop'
  if (kind === 'output') return scope !== 'iteration'
  if (kind === 'loopExit') return scope === 'loop'
  return scope === 'workflow'
}
function nodeScope(node: CanvasNode, graph: Graph, handle?: string): Scope {
  if (node.data.kind === 'iteration' && handle === 'iteration-entry') return 'iteration'
  const parent = graph.nodes.find((item) => item.id === node.parentId)
  return parent?.data.kind === 'loop' ? 'loop' : parent?.data.kind === 'iteration' ? 'iteration' : 'workflow'
}
const width = (node: CanvasNode) => isContainer(node) ? Number(node.width ?? node.initialWidth ?? 530) : node.data.kind === 'condition' ? 320 : 230
const height = (node: CanvasNode) => node.data.kind === 'iteration' && node.data.collapsed === true ? 90 : isContainer(node) ? Number(node.height ?? node.initialHeight ?? 260) : node.data.kind === 'condition' ? Math.max(120, 80 + ports(node).length * 25) : 110
const nextId = (prefix: string, ids: string[]) => { let n = 1; while (ids.includes(`${prefix}-${n}`)) n++; return `${prefix}-${n}` }
const unusedId = (preferred: string, ids: string[]) => ids.includes(preferred) ? nextId(preferred, ids) : preferred
function validAnnotations(value: unknown): value is Annotation[] {
  return Array.isArray(value) && value.every((item) => item && typeof item === 'object' && typeof item.id === 'string' && item.position && typeof item.position.x === 'number' && typeof item.position.y === 'number' && item.data && typeof item.data.text === 'string' && typeof item.data.theme === 'string')
}
function location(node: CanvasNode, graph: Graph): { x: number; y: number } {
  let x = node.position.x, y = node.position.y, parentId = node.parentId
  const visited = new Set([node.id])
  while (parentId && !visited.has(parentId)) {
    visited.add(parentId)
    const parent = graph.nodes.find((item) => item.id === parentId)
    if (!parent) break
    x += parent.position.x; y += parent.position.y; parentId = parent.parentId
  }
  return { x, y }
}
function hiddenByCollapsedParent(node: CanvasNode, nodes: CanvasNode[]): boolean {
  let parentId = node.parentId
  const visited = new Set([node.id])
  while (parentId && !visited.has(parentId)) {
    visited.add(parentId)
    const parent = nodes.find((item) => item.id === parentId)
    if (!parent) return false
    if (parent.data.kind === 'iteration' && parent.data.collapsed === true) return true
    parentId = parent.parentId
  }
  return false
}
function hasSelectedAncestor(node: CanvasNode, nodes: CanvasNode[], selected: string[]): boolean {
  let parentId = node.parentId
  const visited = new Set([node.id])
  while (parentId && !visited.has(parentId)) {
    if (selected.includes(parentId)) return true
    visited.add(parentId); parentId = nodes.find((item) => item.id === parentId)?.parentId
  }
  return false
}
function caseIds(node: CanvasNode): string[] {
  return Array.isArray(node.data.cases) ? node.data.cases.flatMap((item) => typeof item === 'object' && item && 'id' in item && typeof item.id === 'string' ? [item.id] : []) : []
}
function defaults(kind: string): Record<string, unknown> {
  const common = { kind, title: label(kind), description: '' }
  if (kind === 'start') return { ...common, input: '', inputVariables: [] }
  if (kind === 'agent') return { ...common, agentConfig: { schemaVersion: 3, executor: { agentCli: '', modelId: '' }, roleId: '', skills: [], mcps: [], prompt: '' } }
  if (kind === 'condition') return { ...common, cases: [{ id: 'case-1', logic: 'and', conditions: [] }] }
  if (kind === 'aggregator') return { ...common, aggregatorConfig: { variables: [] } }
  if (kind === 'loop') return { ...common, loopConfig: { maxIterations: 10, maxIterationsBehavior: 'fail', variables: [], until: { logic: 'and', conditions: [] }, outputs: [] } }
  if (kind === 'iteration') return { ...common, iterationConfig: { iteratorSelector: [], collectSelector: [], errorStrategy: 'fail', maxIterations: 100 } }
  if (kind === 'output') return { ...common, outputs: [] }
  return common
}
function ports(node: CanvasNode): string[] {
  if (node.data.kind === 'loopExit' || node.data.kind === 'output') return []
  if (node.data.kind === 'condition') {
    const cases = Array.isArray(node.data.cases) ? node.data.cases : []
    return [...cases.flatMap((item) => typeof item === 'object' && item && 'id' in item && typeof item.id === 'string' ? [item.id] : []), 'else']
  }
  if (node.data.kind === 'iteration') return node.data.collapsed === true ? [''] : ['iteration-entry', '']
  return ['']
}

/** Desktop-compatible graph canvas. Unknown authored fields stay intact during edits. */
export function WorkflowCanvas({ graph, onChange, readOnly = false, renderInspector, externalEdit }: { graph: string; onChange: (graph: string) => void; readOnly?: boolean; renderInspector?: (props: InspectorProps) => ReactNode; externalEdit?: { before: string; id: number } }) {
  const parsed = parseCanvasGraph(graph)
  const [selected, setSelected] = useState<string | null>(null)
  const [selection, setSelection] = useState<string[]>([])
  const [selectionBox, setSelectionBox] = useState<{ x: number; y: number; width: number; height: number } | null>(null)
  const [wirePreview, setWirePreview] = useState<{ x1: number; y1: number; x2: number; y2: number } | null>(null)
  const [connection, setConnection] = useState<{ source: string; handle: string } | null>(null)
  const [linkError, setLinkError] = useState('')
  const [containerError, setContainerError] = useState('')
  const [pendingDelete, setPendingDelete] = useState<string[] | null>(null)
  const [past, setPast] = useState<string[]>([])
  const externalEditId = useRef<number | undefined>(undefined)
  const [future, setFuture] = useState<string[]>([])
  const [panMode, setPanMode] = useState(false)
  const [snapToGrid, setSnapToGrid] = useState(false)
  const [readOnlyViewport, setReadOnlyViewport] = useState<Graph['viewport']>()
  const [preview, setPreview] = useState<Graph | null>(null)
  const previewRef = useRef<Graph | null>(null)
  const portDrag = useRef<{ source: string; handle: string; x: number; y: number } | null>(null)
  const suppressPortClick = useRef(false)
  const shell = useRef<HTMLDivElement>(null)
  const stage = useRef<HTMLDivElement>(null)
  const drag = useRef<{ annotation?: string; node?: string; nodes?: string[]; marquee?: boolean; startX: number; startY: number; x: number; y: number; original: Graph } | null>(null)
  const current = preview ?? parsed.graph
  const viewport = (readOnly ? preview?.viewport ?? readOnlyViewport : undefined) ?? current?.viewport ?? { x: 0, y: 0, zoom: 1 }
  const commit = (next: Graph) => {
    if (readOnly) return
    const text = JSON.stringify(next, null, 2)
    if (text === graph) return
    setPast((history) => [...history.slice(-99), graph]); setFuture([]); onChange(text)
  }
  const undo = () => { if (readOnly || !past.length) return; const previous = past[past.length - 1]; setPast(past.slice(0, -1)); setFuture([graph, ...future]); onChange(previous) }
  const redo = () => { if (readOnly || !future.length) return; setPast([...past, graph]); onChange(future[0]); setFuture(future.slice(1)) }
  useEffect(() => {
    if (!externalEdit || externalEdit.id === externalEditId.current) return
    externalEditId.current = externalEdit.id
    setPast((history) => [...history.slice(-99), externalEdit.before]); setFuture([])
  }, [externalEdit])
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (readOnly || pendingDelete || !shell.current || shell.current.closest('[inert]') || !shell.current.contains(target)) return
      if (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') { event.preventDefault(); if (event.shiftKey) redo(); else undo() }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'y') { event.preventDefault(); redo() }
    }
    window.addEventListener('keydown', key); return () => window.removeEventListener('keydown', key)
  })
  if (!current) return <p role="alert">{parsed.problem}，原始数据已保留。</p>
  const annotationsReadable = current.annotations === undefined || validAnnotations(current.annotations)
  const notes = validAnnotations(current.annotations) ? current.annotations : []
  const selectedNode = current.nodes.find((node) => node.id === selected)
  const connectionSource = connection ? current.nodes.find((node) => node.id === connection.source) : undefined
  const visibleNodes = current.nodes.filter((node) => !hiddenByCollapsedParent(node, current.nodes))
  const bounds = visibleNodes.map((node) => ({ ...location(node, current), width: width(node), height: height(node) }))
  const minX = Math.min(0, ...bounds.map((box) => box.x)), minY = Math.min(0, ...bounds.map((box) => box.y))
  const maxX = Math.max(1, ...bounds.map((box) => box.x + box.width)), maxY = Math.max(1, ...bounds.map((box) => box.y + box.height))
  function add(kind: string) {
    if (!current || readOnly) return
    const source = connection ? current.nodes.find((item) => item.id === connection.source) : undefined
    const scope = source ? nodeScope(source, current, connection?.handle) : 'workflow'
    if (!allowsScope(kind, scope) || (source && kind === 'start')) return
    if (kind === 'start' && current.nodes.some((node) => node.data.kind === 'start')) return
    const id = nextId(kind, current.nodes.map((node) => node.id))
    const node: CanvasNode = { id, type: 'workflow', position: { x: Math.round((100 - viewport.x) / viewport.zoom + current.nodes.length * 35), y: Math.round((100 - viewport.y) / viewport.zoom + current.nodes.length * 35) }, data: defaults(kind), ...(kind === 'start' ? { deletable: false } : {}), ...(kind === 'loop' || kind === 'iteration' ? { initialWidth: 530, initialHeight: 260 } : {}) }
    if (source && connection && !isContainer(node)) {
      const parent = scope === 'workflow' ? undefined : scope === 'iteration' && source.data.kind === 'iteration' && connection.handle === 'iteration-entry' ? source : current.nodes.find((item) => item.id === source.parentId)
      node.position = parent ? source.id === parent.id ? { x: 40, y: 90 } : { x: source.position.x + width(source) + 100, y: source.position.y } : { x: source.position.x + width(source) + 100, y: source.position.y }
      if (parent) { node.parentId = parent.id; if (scope === 'loop') { node.extent = 'parent'; node.data = { ...node.data, containerId: parent.id } } }
      const nodes = current.nodes.map((item) => {
        if (item.id !== parent?.id) return item
        const nextWidth = Math.max(width(item), node.position.x + width(node) + 40), nextHeight = Math.max(height(item), node.position.y + height(node) + 40)
        return { ...item, initialWidth: nextWidth, initialHeight: nextHeight, ...(item.width === undefined ? {} : { width: nextWidth }), ...(item.height === undefined ? {} : { height: nextHeight }) }
      })
      commit({ ...current, nodes: [...nodes, node], edges: [...current.edges, { id: nextId('edge', current.edges.map((edge) => edge.id)), type: 'workflow', source: source.id, target: node.id, ...(connection.handle ? { sourceHandle: connection.handle } : {}) }] })
      setConnection(null)
    } else if (kind === 'loop') {
      if (source) node.position = { x: source.position.x + width(source) + 100, y: source.position.y }
      const startId = unusedId(id + '-start', current.nodes.map((item) => item.id))
      const agentId = unusedId(id + '-agent', [...current.nodes.map((item) => item.id), startId])
      const agentData = defaults('agent')
      const start: CanvasNode = { id: startId, type: 'workflow', position: { x: 40, y: 145 }, parentId: id, deletable: false, data: { ...defaults('start'), title: '轮次开始', containerId: id } }
      const agent: CanvasNode = { id: agentId, type: 'workflow', position: { x: 350, y: 145 }, parentId: id, data: { ...agentData, title: '循环 Agent', containerId: id } }
      node.initialWidth = 650; node.initialHeight = 300
      node.data.loopConfig = { maxIterations: 3, maxIterationsBehavior: 'fail', variables: [{ name: 'value', valueType: 'string', initial: { kind: 'constant', value: '' }, feedback: [agentId, 'output'] }], until: { logic: 'and', conditions: [{ variableSelector: [agentId, 'output'], operator: 'not_empty' }] }, outputs: [{ name: 'result', variableSelector: [agentId, 'output'] }] }
      commit({ ...current, schemaVersion: 2, nodes: [...current.nodes, node, start, agent], edges: [...current.edges, { id: nextId('edge', current.edges.map((edge) => edge.id)), source: startId, target: agentId, type: 'workflow' }, ...(source && connection ? [{ id: nextId('edge', [...current.edges.map((edge) => edge.id), nextId('edge', current.edges.map((edge) => edge.id))]), source: source.id, target: node.id, type: 'workflow', ...(connection.handle ? { sourceHandle: connection.handle } : {}) }] : [])] })
      if (source) setConnection(null)
    } else {
      const edges = source && connection ? [...current.edges, { id: nextId('edge', current.edges.map((edge) => edge.id)), type: 'workflow', source: source.id, target: node.id, ...(connection.handle ? { sourceHandle: connection.handle } : {}) }] : current.edges
      if (source) node.position = { x: source.position.x + width(source) + 100, y: source.position.y }
      commit({ ...current, nodes: [...current.nodes, node], edges }); if (source) setConnection(null)
    }
    setSelected(id); setSelection([id])
  }
  function copyNode(node: CanvasNode) {
    if (!current || readOnly || node.parentId || isContainer(node) || node.data.kind === 'start' || node.data.kind === 'loopExit') return
    const id = nextId(String(node.data.kind || 'node'), current.nodes.map((item) => item.id))
    const copy = { ...node, id, position: { x: node.position.x + 30, y: node.position.y + 30 } }
    commit({ ...current, nodes: [...current.nodes, copy] }); setSelected(id); setSelection([id])
  }
  function removeNodes(selectedIds: string[], confirmed = false) {
    if (!current || readOnly) return
    const ids = new Set(current.nodes.filter((node) => selectedIds.includes(node.id) && node.deletable !== false).map((node) => node.id))
    const containsNonemptyContainer = current.nodes.some((node) => ids.has(node.id) && isContainer(node) && current.nodes.some((child) => child.parentId === node.id))
    if (containsNonemptyContainer && !confirmed) { setPendingDelete([...ids]); return }
    for (let changed = true; changed;) {
      changed = false
      for (const child of current.nodes) if (child.parentId && ids.has(child.parentId) && !ids.has(child.id)) { ids.add(child.id); changed = true }
    }
    if (!ids.size) return
    commit({ ...current, nodes: current.nodes.filter((node) => !ids.has(node.id)), edges: current.edges.filter((edge) => !ids.has(edge.source) && !ids.has(edge.target)) })
    setPendingDelete(null); setSelection([]); setSelected(null)
  }
  function attach(node: CanvasNode, parentId: string) {
    if (!current) return
    const absolute = location(node, current); const parent = current.nodes.find((item) => item.id === parentId)
    const scope: Scope = parent?.data.kind === 'loop' ? 'loop' : parent?.data.kind === 'iteration' ? 'iteration' : 'workflow'
    if (!allowsScope(node.data.kind, scope)) { setContainerError('此节点不支持目标容器。'); return }
    const parentPosition = parent ? location(parent, current) : { x: 0, y: 0 }
    const { containerId: _container, ...data } = node.data
    const { parentId: _parent, extent: _extent, ...rest } = node
    const updated: CanvasNode = parent ? { ...rest, parentId, ...(parent.data.kind === 'loop' ? { extent: 'parent' } : {}), position: { x: Math.max(20, absolute.x - parentPosition.x), y: Math.max(55, absolute.y - parentPosition.y) }, data: parent.data.kind === 'loop' ? { ...data, containerId: parentId } : data } : { ...rest, position: absolute, data }
    const nextNodes = current.nodes.map((item) => item.id === node.id ? updated : item)
    const invalid = current.edges.some((edge) => {
      if (edge.source !== node.id && edge.target !== node.id) return false
      const source = nextNodes.find((item) => item.id === edge.source), target = nextNodes.find((item) => item.id === edge.target)
      if (!source || !target) return false
      const iterationOwner = (item: CanvasNode) => nextNodes.find((candidate) => candidate.id === item.parentId && candidate.data.kind === 'iteration')?.id
      const sourceOwner = iterationOwner(source), targetOwner = iterationOwner(target)
      const entry = edge.sourceHandle === 'iteration-entry'
      return source.data.containerId !== target.data.containerId || !!(sourceOwner && sourceOwner !== targetOwner) || !!(targetOwner && sourceOwner !== targetOwner && !(source.id === targetOwner && entry)) || !!(entry && targetOwner !== source.id)
    })
    if (invalid) { setContainerError('请先移除跨越容器边界的连线，再更改容器。'); return }
    setContainerError('')
    commit({ ...current, nodes: nextNodes })
  }
  function pointerDown(event: PointerEvent, node?: CanvasNode) {
    if (!current || (node && (event.target as HTMLElement).closest('button'))) return
    let nodes: string[] | undefined
    if (node) {
      nodes = selection.includes(node.id) ? selection : [node.id]
      setSelection(nodes); setSelected(nodes.length === 1 ? node.id : null)
    }
    if (readOnly && node) return
    event.stopPropagation(); event.currentTarget.setPointerCapture?.(event.pointerId)
    const marquee = !node && !panMode && event.button !== 1
    if (marquee) { setSelection([]); setSelected(null) }
    drag.current = { node: node?.id, nodes, marquee, startX: event.clientX, startY: event.clientY, x: node?.position.x ?? viewport.x, y: node?.position.y ?? viewport.y, original: current }
  }
  function annotationDown(event: PointerEvent, note: Annotation) {
    if (readOnly || !current || (event.target as HTMLElement).closest('textarea, button, select')) return
    event.stopPropagation(); event.currentTarget.setPointerCapture?.(event.pointerId)
    drag.current = { annotation: note.id, startX: event.clientX, startY: event.clientY, x: note.position.x, y: note.position.y, original: current }
  }
  function addAnnotation() {
    if (!current || !annotationsReadable) return
    const id = nextId('annotation', [...current.nodes.map((node) => node.id), ...notes.map((note) => note.id)])
    commit({ ...current, annotations: [...notes, { id, type: 'annotation', position: { x: Math.round((120 - viewport.x) / viewport.zoom), y: Math.round((120 - viewport.y) / viewport.zoom) }, width: 240, height: 140, data: { text: '', theme: 'yellow' } }] })
  }
  function fit() {
    if (!current || !stage.current || !current.nodes.length) return
    const boxes = visibleNodes.map((node) => ({ ...location(node, current), width: width(node), height: height(node) }))
    const minX = Math.min(...boxes.map((box) => box.x)), minY = Math.min(...boxes.map((box) => box.y))
    const maxX = Math.max(...boxes.map((box) => box.x + box.width)), maxY = Math.max(...boxes.map((box) => box.y + box.height))
    const zoom = Math.max(.25, Math.min(1.5, (stage.current.clientWidth - 80) / (maxX - minX), (stage.current.clientHeight - 80) / (maxY - minY)))
    const nextViewport = { x: 40 - minX * zoom, y: 40 - minY * zoom, zoom }
    if (readOnly) setReadOnlyViewport(nextViewport); else commit({ ...current, viewport: nextViewport })
  }
  function arrange() {
    if (!current || readOnly) return
    const roots = current.nodes.filter((node) => !node.parentId)
    const columns = new Map(roots.map((node) => [node.id, 0]))
    for (let pass = 0; pass < roots.length; pass++) {
      let changed = false
      for (const edge of current.edges) {
        if (!columns.has(edge.source) || !columns.has(edge.target)) continue
        const next = Math.min(roots.length - 1, (columns.get(edge.source) ?? 0) + 1)
        if ((columns.get(edge.target) ?? 0) < next) { columns.set(edge.target, next); changed = true }
      }
      if (!changed) break
    }
    const layers = new Map<number, CanvasNode[]>()
    for (const node of roots) { const layer = columns.get(node.id) ?? 0; layers.set(layer, [...(layers.get(layer) ?? []), node]) }
    const positions = new Map<string, { x: number; y: number }>()
    let left = 80
    for (const [, items] of [...layers].sort(([a], [b]) => a - b)) {
      const columnWidth = Math.max(...items.map(width)) + 90
      let top = 80
      for (let i = 0; i < items.length; i += 2) {
        const row = items.slice(i, i + 2)
        row.forEach((node, column) => positions.set(node.id, { x: left + column * columnWidth, y: top }))
        top += Math.max(...row.map(height)) + 70
      }
      left += columnWidth * Math.min(items.length, 2) + 100
    }
    const nodes = current.nodes.map((node) => positions.has(node.id) ? { ...node, position: positions.get(node.id)! } : node)
    commit({ ...current, nodes })
  }
  function pointerMove(event: PointerEvent) {
    const d = drag.current; if (!d) return
    const dx = event.clientX - d.startX, dy = event.clientY - d.startY
    if (!dx && !dy) return
    if (d.marquee) {
      const rect = stage.current?.getBoundingClientRect(); if (!rect) return
      setSelectionBox({ x: Math.min(d.startX, event.clientX) - rect.left, y: Math.min(d.startY, event.clientY) - rect.top, width: Math.abs(dx), height: Math.abs(dy) })
      return
    }
    const next = d.node ? { ...d.original, nodes: d.original.nodes.map((node) => {
      if (!d.nodes?.includes(node.id) || hasSelectedAncestor(node, d.original.nodes, d.nodes)) return node
      const parent = d.original.nodes.find((candidate) => candidate.id === node.parentId)
      const snap = (value: number) => snapToGrid ? Math.round(value / 20) * 20 : Math.round(value)
      const x = snap(node.position.x + dx / viewport.zoom), y = snap(node.position.y + dy / viewport.zoom)
      return { ...node, position: parent ? { x: Math.max(0, Math.min(width(parent) - width(node), x)), y: Math.max(50, Math.min(height(parent) - height(node), y)) } : { x, y } }
    }) } : d.annotation ? { ...d.original, annotations: (validAnnotations(d.original.annotations) ? d.original.annotations : []).map((note) => note.id === d.annotation ? { ...note, position: { x: snapToGrid ? Math.round((d.x + dx / viewport.zoom) / 20) * 20 : Math.round(d.x + dx / viewport.zoom), y: snapToGrid ? Math.round((d.y + dy / viewport.zoom) / 20) * 20 : Math.round(d.y + dy / viewport.zoom) } } : note) } : { ...d.original, viewport: { ...viewport, x: d.x + dx, y: d.y + dy } }
    previewRef.current = next; setPreview(next)
  }
  function pointerUp(event: PointerEvent) {
    const d = drag.current; if (!d) return
    drag.current = null
    if (d.marquee && stage.current) {
      const rect = stage.current.getBoundingClientRect()
      const x1 = (Math.min(d.startX, event.clientX) - rect.left - viewport.x) / viewport.zoom, x2 = (Math.max(d.startX, event.clientX) - rect.left - viewport.x) / viewport.zoom
      const y1 = (Math.min(d.startY, event.clientY) - rect.top - viewport.y) / viewport.zoom, y2 = (Math.max(d.startY, event.clientY) - rect.top - viewport.y) / viewport.zoom
      const ids = Math.abs(d.startX - event.clientX) < 4 && Math.abs(d.startY - event.clientY) < 4 ? [] : d.original.nodes.filter((node) => { if (hiddenByCollapsedParent(node, d.original.nodes)) return false; const pos = location(node, d.original); return pos.x < x2 && pos.x + width(node) > x1 && pos.y < y2 && pos.y + height(node) > y1 }).map((node) => node.id)
      setSelection(ids); setSelected(ids.length === 1 ? ids[0] : null); setSelectionBox(null)
    }
    if (previewRef.current) { if (readOnly) setReadOnlyViewport(previewRef.current.viewport); else commit(previewRef.current) }
    previewRef.current = null; setPreview(null)
  }
  function connect(target: string, pending = connection) {
    if (!current || !pending || target === pending.source) return
    const source = current.nodes.find((node) => node.id === pending.source)
    const destination = current.nodes.find((node) => node.id === target)
    if (!source || !destination) return
    const iterationOwner = (node: CanvasNode) => current.nodes.find((item) => item.id === node.parentId && item.data.kind === 'iteration')?.id
    const sourceOwner = iterationOwner(source), targetOwner = iterationOwner(destination)
    const entry = pending.handle === 'iteration-entry'
    if (source.data.containerId !== destination.data.containerId || (sourceOwner && sourceOwner !== targetOwner) || (targetOwner && sourceOwner !== targetOwner && !(source.id === targetOwner && entry)) || (entry && targetOwner !== source.id)) {
      setLinkError('不能连接不同容器中的节点。迭代内部须从迭代入口连接。'); return
    }
    setLinkError('')
    if (!current.edges.some((edge) => edge.source === pending.source && edge.target === target && (edge.sourceHandle ?? '') === pending.handle)) {
      commit({ ...current, edges: [...current.edges, { id: nextId('edge', current.edges.map((edge) => edge.id)), type: 'workflow', source: pending.source, target, ...(pending.handle ? { sourceHandle: pending.handle } : {}) }] })
    }
    setConnection(null)
  }
  function reconnect(edge: Edge, field: 'source' | 'target', id: string) {
    if (!current || readOnly) return
    const next = { ...edge, [field]: id }
    const source = current.nodes.find((node) => node.id === next.source), target = current.nodes.find((node) => node.id === next.target)
    if (!source || !target || source.id === target.id) { setLinkError('连线的起点与终点必须是不同节点。'); return }
    const sourceHandle = next.sourceHandle ?? ''
    const iterationOwner = (node: CanvasNode) => current.nodes.find((item) => item.id === node.parentId && item.data.kind === 'iteration')?.id
    const sourceOwner = iterationOwner(source), targetOwner = iterationOwner(target), entry = sourceHandle === 'iteration-entry'
    const duplicate = current.edges.some((item) => item.id !== edge.id && item.source === next.source && item.target === next.target && (item.sourceHandle ?? '') === sourceHandle)
    if (!ports(source).includes(sourceHandle) || source.data.containerId !== target.data.containerId || (sourceOwner && sourceOwner !== targetOwner) || (targetOwner && sourceOwner !== targetOwner && !(source.id === targetOwner && entry)) || (entry && targetOwner !== source.id) || duplicate) {
      setLinkError('无法重连：端口、容器边界或已有连线不兼容。'); return
    }
    setLinkError(''); commit({ ...current, edges: current.edges.map((item) => item.id === edge.id ? next : item) })
  }
  function restoreHistory(index: number) {
    const history = [...past, graph, ...future]
    if (readOnly || index === past.length || index < 0 || index >= history.length) return
    setPast(history.slice(0, index)); setFuture(history.slice(index + 1)); setSelection([]); setSelected(null); onChange(history[index])
  }
  function updateNode(updated: CanvasNode) {
    if (!current) return
    const previous = current.nodes.find((node) => node.id === updated.id)
    const removedCases = previous?.data.kind === 'condition' && updated.data.kind === 'condition' && Array.isArray(updated.data.cases)
      ? new Set(caseIds(previous).filter((id) => !caseIds(updated).includes(id))) : new Set<string>()
    commit({ ...current, nodes: current.nodes.map((node) => node.id === updated.id ? updated : node), edges: current.edges.filter((edge) => edge.source !== updated.id || !edge.sourceHandle || !removedCases.has(edge.sourceHandle)) })
  }
  return <div ref={shell} className="workflow-canvas-shell">
    <div className="workflow-canvas-bar"><span>画布</span><div className="workflow-canvas-tools"><button type="button" disabled={readOnly || !past.length} onClick={undo} aria-label="撤销"><Undo2 size={16}/></button><button type="button" disabled={readOnly || !future.length} onClick={redo} aria-label="重做"><Redo2 size={16}/></button><details className="workflow-history"><summary>编辑历史</summary><div><p>仅本次会话，可选择任意编辑步骤。</p>{[...past, graph, ...future].map((_, index) => <button type="button" key={index} disabled={readOnly || index === past.length} onClick={() => restoreHistory(index)}>步骤 {index}{index === past.length ? '（当前）' : ''}</button>)}</div></details><button type="button" disabled={readOnly} onClick={arrange} aria-label="整理节点">整理</button><button type="button" disabled={readOnly} aria-label="网格吸附" aria-pressed={snapToGrid} onClick={() => setSnapToGrid(!snapToGrid)}>网格</button><button type="button" aria-label="平移画布" aria-pressed={panMode} onClick={() => setPanMode(!panMode)}><Hand size={16}/></button><button type="button" aria-label="缩小" onClick={() => { const v = { ...viewport, zoom: Math.max(.25, viewport.zoom - .1) }; if (readOnly) setReadOnlyViewport(v); else commit({ ...current, viewport: v }) }}><Minus size={16}/></button><span>{Math.round(viewport.zoom * 100)}%</span><button type="button" onClick={fit} aria-label="适应画布">适应</button><button type="button" aria-label="放大" onClick={() => { const v = { ...viewport, zoom: Math.min(2, viewport.zoom + .1) }; if (readOnly) setReadOnlyViewport(v); else commit({ ...current, viewport: v }) }}><Plus size={16}/></button></div></div>
    <div className="workflow-canvas-body"><div ref={stage} className="workflow-canvas-stage" tabIndex={0} aria-label="工作流画布" onWheel={(event) => { event.preventDefault(); const zoom = Math.max(.25, Math.min(2, viewport.zoom * (event.deltaY < 0 ? 1.1 : 1 / 1.1))); const rect = event.currentTarget.getBoundingClientRect(); const x = event.clientX - rect.left, y = event.clientY - rect.top; const v = { x: x - (x - viewport.x) * zoom / viewport.zoom, y: y - (y - viewport.y) * zoom / viewport.zoom, zoom }; if (readOnly) setReadOnlyViewport(v); else commit({ ...current, viewport: v }) }} onPointerDown={(event) => { if (event.target === event.currentTarget || panMode) pointerDown(event) }} onPointerMove={pointerMove} onPointerUp={pointerUp} onPointerCancel={() => { drag.current = null; previewRef.current = null; setPreview(null); setSelectionBox(null) }}>
      <div className="workflow-canvas-world" style={{ transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.zoom})` }}>
        <svg className="workflow-canvas-edges" aria-label="连线"><defs><marker id="workflow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#87929d" /></marker></defs>{current.edges.map((edge) => { const source = current.nodes.find((n) => n.id === edge.source), target = current.nodes.find((n) => n.id === edge.target); if (!source || !target || hiddenByCollapsedParent(source, current.nodes) || hiddenByCollapsedParent(target, current.nodes)) return null; const a = location(source, current), b = location(target, current); const sourcePorts = ports(source), index = sourcePorts.indexOf(edge.sourceHandle ?? ''); const x1 = a.x + width(source), y1 = a.y + height(source) * (index < 0 ? .5 : (index + 1) / (sourcePorts.length + 1)), x2 = b.x, y2 = b.y + height(target) / 2; return <g key={edge.id}><path d={`M ${x1} ${y1} C ${x1+60} ${y1}, ${x2-60} ${y2}, ${x2} ${y2}`} fill="none" stroke="#87929d" strokeWidth="2" markerEnd="url(#workflow-arrow)"/><text x={(x1+x2)/2} y={(y1+y2)/2-7} fontSize="11" fill="#586273">{edge.sourceHandle}</text></g> })}</svg>
        {notes.map((note) => <div key={note.id} className="workflow-annotation" style={{ left: note.position.x, top: note.position.y, background: ({ yellow: '#fff7cb', blue: '#e0f0ff', green: '#e1f5df', pink: '#ffe6ed', gray: '#eceff1' } as Record<string, string>)[note.data.theme] ?? '#fff7cb' }} onPointerDown={(event) => annotationDown(event, note)} onPointerMove={pointerMove} onPointerUp={pointerUp}><textarea aria-label="注释内容" value={note.data.text} readOnly={readOnly} placeholder="输入注释…" onChange={(event) => commit({ ...current, annotations: notes.map((item) => item.id === note.id ? { ...item, data: { ...item.data, text: event.target.value } } : item) })}/>{!readOnly && <div><select aria-label="注释颜色" value={note.data.theme} onChange={(event) => commit({ ...current, annotations: notes.map((item) => item.id === note.id ? { ...item, data: { ...item.data, theme: event.target.value } } : item) })}><option value="yellow">黄色</option><option value="blue">蓝色</option><option value="green">绿色</option><option value="pink">粉色</option><option value="gray">灰色</option></select><button type="button" aria-label="删除注释" onClick={() => commit({ ...current, annotations: notes.filter((item) => item.id !== note.id) })}>删除</button></div>}</div>)}
        {visibleNodes.map((node) => { const pos = location(node, current); return <div key={node.id} className={`workflow-canvas-node ${isContainer(node) ? 'workflow-canvas-container' : ''} ${selection.includes(node.id) ? 'selected' : ''}`} data-workflow-node={node.id} data-selected={selection.includes(node.id)} style={{ left: pos.x, top: pos.y, width: width(node), height: height(node), zIndex: isContainer(node) ? 1 : 2 }} onPointerDown={(event) => pointerDown(event, node)} onPointerMove={pointerMove} onPointerUp={pointerUp}>
          <div className="workflow-node-heading">{node.data.kind === 'iteration' && <button type="button" aria-label={node.data.collapsed ? '展开迭代' : '折叠迭代'} onClick={() => { updateNode({ ...node, data: { ...node.data, collapsed: !node.data.collapsed } }); setSelected(node.id); setSelection([node.id]) }}>{node.data.collapsed ? '▸' : '▾'}</button>}<span className={`workflow-node-icon kind-${String(node.data.kind)}`}>{({ start: '▷', agent: 'ϟ', condition: '⑂', aggregator: '≫', loop: '↻', loopExit: '⇥', iteration: '▤', output: '→' } as Record<string, string>)[String(node.data.kind)] ?? '·'}</span><strong>{String(node.data.title || label(node.data.kind))}</strong><small>{String(node.data.kind ?? '')}</small></div><p>{String(node.data.description || label(node.data.kind))}</p>
          {node.data.kind === 'condition' && ports(node).map((port, index, all) => <span key={port} className="workflow-branch-label" style={{ top: `${((index+1)/(all.length+1))*100}%` }}>{port === 'else' ? 'ELSE' : index === 0 ? 'IF' : 'ELIF'}</span>)}
          {!readOnly && <><button type="button" className="workflow-port input" data-workflow-input={node.id} aria-label={`连接到${String(node.data.title || node.id)}`} title="连接到此节点" onClick={() => connect(node.id)}><ArrowDownLeft size={12}/></button>{ports(node).map((port, index, all) => <button key={port} type="button" className={`workflow-port output ${connection?.source === node.id && connection.handle === port ? 'active' : ''}`} style={{ top: `${((index+1)/(all.length+1))*100}%` }} aria-label={`${String(node.data.title || node.id)} 输出 ${port || '默认'}`} title={port === 'else' ? 'ELSE' : port.startsWith('case-') ? 'IF / ELIF' : port === 'iteration-entry' ? '迭代入口' : '开始连线'} onPointerDown={(event) => { event.stopPropagation(); event.currentTarget.setPointerCapture?.(event.pointerId); portDrag.current = { source: node.id, handle: port, x: event.clientX, y: event.clientY }; const rect = stage.current?.getBoundingClientRect(); if (rect) setWirePreview({ x1: event.clientX - rect.left, y1: event.clientY - rect.top, x2: event.clientX - rect.left, y2: event.clientY - rect.top }) }} onPointerMove={(event) => { if (!portDrag.current) return; const rect = stage.current?.getBoundingClientRect(); if (rect) setWirePreview({ x1: portDrag.current.x - rect.left, y1: portDrag.current.y - rect.top, x2: event.clientX - rect.left, y2: event.clientY - rect.top }) }} onPointerCancel={() => { portDrag.current = null; setWirePreview(null) }} onPointerUp={(event) => { const dragPort = portDrag.current; if (!dragPort) return; portDrag.current = null; setWirePreview(null); if (Math.hypot(event.clientX - dragPort.x, event.clientY - dragPort.y) < 4) return; suppressPortClick.current = true; const target = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>('[data-workflow-input]')?.dataset.workflowInput; if (target) connect(target, dragPort); else { setConnection({ source: dragPort.source, handle: dragPort.handle }); setLinkError('拖到目标节点的输入端口，或点击目标输入端口。') } }} onClick={() => { if (suppressPortClick.current) { suppressPortClick.current = false; return } setLinkError(''); setConnection({ source: node.id, handle: port }) }}><ArrowUpRight size={12}/></button>)}</>}
        </div> })}
      </div>
      {selectionBox && <div className="workflow-selection-box" style={{ left: selectionBox.x, top: selectionBox.y, width: selectionBox.width, height: selectionBox.height }} />}
      {wirePreview && <svg className="workflow-wire-preview" aria-label="连线预览"><path d={`M ${wirePreview.x1} ${wirePreview.y1} C ${wirePreview.x1 + 60} ${wirePreview.y1}, ${wirePreview.x2 - 60} ${wirePreview.y2}, ${wirePreview.x2} ${wirePreview.y2}`} /></svg>}
      <svg className="workflow-minimap" aria-label="画布缩略图" viewBox={`${minX - 40} ${minY - 40} ${maxX - minX + 80} ${maxY - minY + 80}`} preserveAspectRatio="xMidYMid meet" onPointerDown={(event) => event.stopPropagation()}>{bounds.map((box, index) => <rect key={visibleNodes[index].id} x={box.x} y={box.y} width={box.width} height={box.height} rx="8" fill="#aeb8c3" />)}</svg>
      {connection && <div className="workflow-connect-hint">{linkError || '选择目标节点的输入端口以连线'} <button type="button" onClick={() => setConnection(null)}>取消</button></div>}
    </div>{selectedNode && <aside className="workflow-inspector">{renderInspector ? renderInspector({ node: selectedNode, nodes: current.nodes, edges: current.edges, onChange: updateNode, onClose: () => setSelected(null) }) : <><div className="workflow-inspector-title"><strong>{String(selectedNode.data.title || label(selectedNode.data.kind))}</strong><button type="button" aria-label="关闭节点详情" onClick={() => setSelected(null)}><X size={16}/></button></div><p>选择节点以编辑配置。</p></>}
      {!readOnly && isContainer(selectedNode) && selectedNode.data.collapsed !== true && <div className="workflow-container-size"><strong>容器尺寸</strong>{(['width', 'height'] as const).map((axis) => {
        const minimum = Math.max(axis === 'width' ? 320 : 180, ...current.nodes.filter((node) => node.parentId === selectedNode.id).map((node) => axis === 'width' ? node.position.x + width(node) + 20 : node.position.y + height(node) + 20))
        const value = axis === 'width' ? width(selectedNode) : height(selectedNode)
        return <label key={selectedNode.id + axis + value}>{axis === 'width' ? '宽度' : '高度'}<input type="number" aria-label={axis === 'width' ? '容器宽度' : '容器高度'} min={minimum} step={20} defaultValue={value} onBlur={(event) => { const next = Number(event.target.value); if (Number.isFinite(next) && next >= minimum) updateNode({ ...selectedNode, [axis === 'width' ? 'initialWidth' : 'initialHeight']: next, ...(selectedNode[axis] === undefined ? {} : { [axis]: next }) }); else event.target.value = String(value) }} /></label>
      })}</div>}
      {!readOnly && !isContainer(selectedNode) && selectedNode.data.kind !== 'start' && <label>所在容器<select value={selectedNode.parentId ?? ''} onChange={(event) => attach(selectedNode, event.target.value)}>{allowsScope(selectedNode.data.kind, 'workflow') && <option value="">无</option>}{current.nodes.filter((node) => isContainer(node) && allowsScope(selectedNode.data.kind, node.data.kind === 'loop' ? 'loop' : 'iteration')).map((node) => <option value={node.id} key={node.id}>{String(node.data.title || node.id)}</option>)}</select></label>}
      {containerError && <p role="alert">{containerError}</p>}
      {!readOnly && <button type="button" disabled={!!selectedNode.parentId || isContainer(selectedNode) || selectedNode.data.kind === 'start' || selectedNode.data.kind === 'loopExit'} title={selectedNode.parentId || isContainer(selectedNode) ? '容器及其成员暂不支持复制' : ''} onClick={() => copyNode(selectedNode)}>复制节点</button>}
      {!readOnly && selectedNode.deletable !== false && <button type="button" className="workflow-delete" onClick={() => removeNodes([selectedNode.id])}>删除节点</button>}
    </aside>}</div>
    {pendingDelete && <div className="workflow-delete-overlay" role="alertdialog" aria-label="确认删除容器"><div><strong>删除包含节点的容器？</strong><p>容器内的节点及其连线也会删除，可以通过撤销恢复。</p><button type="button" onClick={() => removeNodes(pendingDelete, true)}>确认删除</button><button type="button" onClick={() => setPendingDelete(null)}>取消</button></div></div>}
    {!annotationsReadable && <p role="status" className="workflow-annotation-warning">当前注释格式无法在画布显示，原始数据已保留。</p>}
    {!readOnly && <div className="workflow-palette"><span>{connection ? '从输出添加节点' : '添加节点'}</span>{selection.length > 1 && <button type="button" onClick={() => removeNodes(selection)}>删除选中节点 ({selection.length})</button>}<button type="button" disabled={!annotationsReadable} onClick={addAnnotation}>添加注释</button>{kinds.map(([kind, title]) => <button type="button" key={kind} disabled={!allowsScope(kind, connectionSource && connection ? nodeScope(connectionSource, current, connection.handle) : 'workflow') || (kind === 'start' && (!!connection || current.nodes.some((node) => node.data.kind === 'start')))} onClick={() => add(kind)}>{title}</button>)}</div>}
    {current.edges.length > 0 && !readOnly && <details className="workflow-edges-list"><summary>连线 ({current.edges.length})</summary>{linkError && !connection && <p role="alert">{linkError}</p>}{current.edges.map((edge) => <div key={edge.id}><label>起点<select aria-label={`连线 ${edge.id} 起点`} value={edge.source} onChange={(event) => reconnect(edge, 'source', event.target.value)}>{current.nodes.map((node) => <option key={node.id} value={node.id}>{String(node.data.title || node.id)} ({node.id})</option>)}</select></label>{edge.sourceHandle && <span>端口 {edge.sourceHandle}</span>}<span>→</span><label>终点<select aria-label={`连线 ${edge.id} 终点`} value={edge.target} onChange={(event) => reconnect(edge, 'target', event.target.value)}>{current.nodes.map((node) => <option key={node.id} value={node.id}>{String(node.data.title || node.id)} ({node.id})</option>)}</select></label><button type="button" aria-label={`删除连线 ${edge.id}`} onClick={() => commit({ ...current, edges: current.edges.filter((item) => item.id !== edge.id) })}>删除</button></div>)}</details>}
  </div>
}
