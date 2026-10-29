import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { WorkflowCanvas } from './workflow-canvas'

const initial = JSON.stringify({ custom: { retained: true }, nodes: [{ id: 'start', type: 'workflow', position: { x: 40, y: 40 }, deletable: false, data: { kind: 'start', title: '开始', custom: 'keep' } }], edges: [], viewport: { x: 0, y: 0, zoom: 1 } })
function Harness({ initialValue = initial }: { initialValue?: string }) {
  const [graph, setGraph] = useState(initialValue)
  return <><WorkflowCanvas graph={graph} onChange={setGraph} /><output data-testid="graph">{graph}</output></>
}
const graph = () => JSON.parse(screen.getByTestId('graph').textContent ?? '')

describe('workflow canvas', () => {
  it('adds nodes and connects branch ports while retaining unknown data, and can undo and redo', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('button', { name: '条件分支' }))
    await userEvent.click(screen.getByRole('button', { name: '输出' }))
    await userEvent.click(screen.getByRole('button', { name: '条件分支 输出 case-1' }))
    await userEvent.click(screen.getByRole('button', { name: '连接到输出' }))
    expect(graph().edges).toEqual([{ id: 'edge-1', type: 'workflow', source: 'condition-1', sourceHandle: 'case-1', target: 'output-1' }])
    expect(graph().custom).toEqual({ retained: true })
    expect(graph().nodes[0].data.custom).toBe('keep')
    await userEvent.click(screen.getByRole('button', { name: '撤销' }))
    expect(graph().edges).toEqual([])
    await userEvent.click(screen.getByRole('button', { name: '重做' }))
    expect(graph().edges).toHaveLength(1)
  })

  it('keeps loop children in relative coordinates and removes their edges with the container', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('button', { name: '循环' }))
    await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
    await userEvent.selectOptions(screen.getByLabelText('所在容器'), 'loop-1')
    const child = graph().nodes.find((node: { id: string }) => node.id === 'agent-1')
    expect(child.parentId).toBe('loop-1')
    expect(child.data.containerId).toBe('loop-1')
    expect(child.position.x).toBeGreaterThanOrEqual(20)
    await userEvent.click(screen.getByText('循环', { selector: 'strong' }))
    await userEvent.click(screen.getByRole('button', { name: '删除节点' }))
    await userEvent.click(screen.getByRole('button', { name: '确认删除' }))
    expect(graph().nodes.map((node: { id: string }) => node.id)).toEqual(['start'])
  })
})

it('creates a loop with its executable children and protects its internal links', async () => {
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: '循环' }))
  const value = graph()
  const loop = value.nodes.find((node: { id: string }) => node.id === 'loop-1')
  expect(loop.data.loopConfig.variables[0].feedback).toEqual(['loop-1-agent', 'output'])
  expect(value.nodes.find((node: { id: string }) => node.id === 'loop-1-start')).toMatchObject({ parentId: 'loop-1', deletable: false, data: { containerId: 'loop-1' } })
  expect(value.edges).toContainEqual({ id: 'edge-1', type: 'workflow', source: 'loop-1-start', target: 'loop-1-agent' })
  await userEvent.click(screen.getByRole('button', { name: '轮次开始 输出 默认' }))
  await userEvent.click(screen.getByRole('button', { name: '连接到开始' }))
  expect(screen.getByText(/不能连接不同容器/)).toBeInTheDocument()
  expect(graph().edges).toHaveLength(1)
})


it('connects an output to an input by dragging', async () => {
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
  const output = screen.getByRole('button', { name: '开始 输出 默认' })
  const input = screen.getByRole('button', { name: '连接到Agent' })
  const oldPointerEvent = window.PointerEvent
  window.PointerEvent = MouseEvent as typeof PointerEvent
  const oldElementFromPoint = Object.getOwnPropertyDescriptor(document, 'elementFromPoint')
  Object.defineProperty(document, 'elementFromPoint', { configurable: true, value: vi.fn(() => input) })
  try {
    fireEvent.pointerDown(output, { clientX: 10, clientY: 10, pointerId: 1 })
    fireEvent.pointerMove(output, { clientX: 80, clientY: 80, pointerId: 1 })
    expect(screen.getByLabelText('连线预览').querySelector('path')).toHaveAttribute('d', expect.stringContaining('80 80'))
    fireEvent.pointerUp(output, { clientX: 100, clientY: 100, pointerId: 1 })
    expect(screen.queryByLabelText('连线预览')).not.toBeInTheDocument()
    expect(graph().edges).toMatchObject([{ source: 'start', target: 'agent-1' }])
  } finally { window.PointerEvent = oldPointerEvent; if (oldElementFromPoint) Object.defineProperty(document, 'elementFromPoint', oldElementFromPoint); else Reflect.deleteProperty(document, 'elementFromPoint') }
})


it('selects several nodes with a marquee and moves them together', async () => {
  const { container } = render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
  const stage = container.querySelector('.workflow-canvas-stage') as HTMLElement
  const oldPointerEvent = window.PointerEvent
  window.PointerEvent = MouseEvent as typeof PointerEvent
  try {
    fireEvent.pointerDown(stage, { clientX: 0, clientY: 0, button: 0 })
    fireEvent.pointerMove(stage, { clientX: 400, clientY: 300 })
    expect(container.querySelector('.workflow-selection-box')).toBeInTheDocument()
    fireEvent.pointerUp(stage, { clientX: 400, clientY: 300 })
    const start = container.querySelector('[data-workflow-node="start"]') as HTMLElement
    const agent = container.querySelector('[data-workflow-node="agent-1"]') as HTMLElement
    expect(start).toHaveAttribute('data-selected', 'true')
    expect(agent).toHaveAttribute('data-selected', 'true')
    const before = graph().nodes.map((node: { position: unknown }) => node.position)
    fireEvent.pointerDown(start, { clientX: 40, clientY: 40, button: 0 })
    fireEvent.pointerMove(start, { clientX: 60, clientY: 70 })
    fireEvent.pointerUp(start, { clientX: 60, clientY: 70 })
    expect(graph().nodes.map((node: { position: unknown }) => node.position)).toEqual(before.map((pos: { x: number; y: number }) => ({ x: pos.x + 20, y: pos.y + 30 })))
  } finally { window.PointerEvent = oldPointerEvent }
})


it('moves a selected loop and its members without moving members twice', async () => {
  const { container } = render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: '循环' }))
  const before = graph()
  const stage = container.querySelector('.workflow-canvas-stage') as HTMLElement
  const oldPointerEvent = window.PointerEvent
  window.PointerEvent = MouseEvent as typeof PointerEvent
  try {
    fireEvent.pointerDown(stage, { clientX: 0, clientY: 0, button: 0 })
    fireEvent.pointerMove(stage, { clientX: 900, clientY: 500 })
    fireEvent.pointerUp(stage, { clientX: 900, clientY: 500 })
    const loop = container.querySelector('[data-workflow-node="loop-1"]') as HTMLElement
    fireEvent.pointerDown(loop, { clientX: 140, clientY: 140, button: 0 })
    fireEvent.pointerMove(loop, { clientX: 180, clientY: 160 })
    fireEvent.pointerUp(loop, { clientX: 180, clientY: 160 })
    const after = graph()
    const prior = before.nodes.find((node: { id: string }) => node.id === 'loop-1')
    expect(after.nodes.find((node: { id: string }) => node.id === 'loop-1').position).toEqual({ x: prior.position.x + 40, y: prior.position.y + 20 })
    expect(after.nodes.find((node: { id: string }) => node.id === 'loop-1-start')).toEqual(before.nodes.find((node: { id: string }) => node.id === 'loop-1-start'))
  } finally { window.PointerEvent = oldPointerEvent }
})


it('preserves iteration membership that has only parentId during group movement', () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'iter', type: 'workflow', position: { x: 100, y: 100 }, data: { kind: 'iteration', title: '迭代' }, initialWidth: 530, initialHeight: 260 },
    { id: 'member', type: 'workflow', position: { x: 40, y: 80 }, parentId: 'iter', data: { kind: 'agent', title: '内部', custom: 1 } },
  ], edges: [], viewport: { x: 0, y: 0, zoom: 1 } })
  const { container } = render(<Harness initialValue={initialValue} />)
  const stage = container.querySelector('.workflow-canvas-stage') as HTMLElement
  const oldPointerEvent = window.PointerEvent
  window.PointerEvent = MouseEvent as typeof PointerEvent
  try {
    fireEvent.pointerDown(stage, { clientX: 0, clientY: 0, button: 0 })
    fireEvent.pointerMove(stage, { clientX: 800, clientY: 500 })
    fireEvent.pointerUp(stage, { clientX: 800, clientY: 500 })
    const iteration = container.querySelector('[data-workflow-node="iter"]') as HTMLElement
    fireEvent.pointerDown(iteration, { clientX: 110, clientY: 110, button: 0 })
    fireEvent.pointerMove(iteration, { clientX: 130, clientY: 140 })
    fireEvent.pointerUp(iteration, { clientX: 130, clientY: 140 })
    expect(graph().nodes[0].position).toEqual({ x: 120, y: 130 })
    expect(graph().nodes[1]).toMatchObject({ parentId: 'iter', position: { x: 40, y: 80 }, data: { kind: 'agent', custom: 1 } })
    expect(graph().nodes[1].data).not.toHaveProperty('containerId')
  } finally { window.PointerEvent = oldPointerEvent }
})


it.each([
  ['旧对象格式', { legacy: { note: '保持原样' } }],
  ['包含无法识别注释的数组', [{ id: 'old', data: { custom: 42 } }]],
  ['空值', null],
])('preserves %s annotations while editing the graph', async (_description, annotations) => {
  const initialValue = JSON.stringify({ ...JSON.parse(initial), annotations })
  render(<Harness initialValue={initialValue} />)
  expect(screen.getByText(/当前注释格式无法在画布显示/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '添加注释' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
  expect(graph().annotations).toEqual(annotations)
})

it('keeps unknown annotation fields when editing a supported annotation', async () => {
  const initialValue = JSON.stringify({ ...JSON.parse(initial), annotations: [{ id: 'a', type: 'annotation', position: { x: 10, y: 20 }, extra: 42, data: { text: '原文', theme: 'yellow', custom: true } }] })
  render(<Harness initialValue={initialValue} />)
  await userEvent.type(screen.getByRole('textbox', { name: '注释内容' }), '新')
  expect(graph().annotations[0]).toMatchObject({ extra: 42, data: { custom: true, text: '原文新' } })
})

it('keeps separate condition branches to one target and removes only a deleted case edge', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'condition', position: { x: 10, y: 10 }, data: { kind: 'condition', title: '分支', cases: [{ id: 'case-1', conditions: [] }, { id: 'case-2', conditions: [] }] } },
    { id: 'out', position: { x: 400, y: 10 }, data: { kind: 'output', title: '结果' } },
  ], edges: [], viewport: { x: 0, y: 0, zoom: 1 }, custom: 7 })
  function BranchHarness() {
    const [value, setValue] = useState(initialValue)
    return <><WorkflowCanvas graph={value} onChange={setValue} renderInspector={({ node, onChange }) => <button onClick={() => onChange({ ...node, data: { ...node.data, cases: [{ id: 'case-2', conditions: [] }] } })}>删除第一个条件</button>} /><output data-testid="graph">{value}</output></>
  }
  render(<BranchHarness />)
  await userEvent.click(screen.getByRole('button', { name: '分支 输出 case-1' }))
  await userEvent.click(screen.getByRole('button', { name: '连接到结果' }))
  await userEvent.click(screen.getByRole('button', { name: '分支 输出 case-2' }))
  await userEvent.click(screen.getByRole('button', { name: '连接到结果' }))
  await userEvent.click(screen.getByRole('button', { name: '分支 输出 else' }))
  await userEvent.click(screen.getByRole('button', { name: '连接到结果' }))
  expect(graph().edges.map((edge: { sourceHandle: string }) => edge.sourceHandle)).toEqual(['case-1', 'case-2', 'else'])
  await userEvent.click(screen.getByText('分支', { selector: 'strong' }))
  await userEvent.click(screen.getByRole('button', { name: '删除第一个条件' }))
  expect(graph().edges.map((edge: { sourceHandle: string }) => edge.sourceHandle)).toEqual(['case-2', 'else'])
  expect(graph().custom).toBe(7)
})

it('renders nested positions and deletes all descendants with their container', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'outer', position: { x: 100, y: 100 }, data: { kind: 'iteration', title: '外层' } },
    { id: 'inner', parentId: 'outer', position: { x: 30, y: 60 }, data: { kind: 'loop', title: '内层' } },
    { id: 'child', parentId: 'inner', position: { x: 10, y: 55 }, data: { kind: 'agent', title: '成员', containerId: 'inner' } },
  ], edges: [{ id: 'edge', source: 'child', target: 'inner' }], viewport: { x: 0, y: 0, zoom: 1 } })
  const { container } = render(<Harness initialValue={initialValue} />)
  expect(container.querySelector('[data-workflow-node="child"]')).toHaveStyle({ left: '140px', top: '215px' })
  await userEvent.click(screen.getByText('外层', { selector: 'strong' }))
  await userEvent.click(screen.getByRole('button', { name: '删除节点' }))
  await userEvent.click(screen.getByRole('button', { name: '确认删除' }))
  expect(graph().nodes).toEqual([])
  expect(graph().edges).toEqual([])
})

it('copies a standalone node with its unknown fields and safely disables container copies', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'agent-1', type: 'workflow', position: { x: 20, y: 30 }, extra: 'keep', data: { kind: 'agent', title: '自定义', custom: { value: 42 } } },
    { id: 'loop-1', type: 'workflow', position: { x: 400, y: 30 }, data: { kind: 'loop', title: '循环' } },
  ], edges: [], viewport: { x: 0, y: 0, zoom: 1 } })
  render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByText('自定义', { selector: 'strong' }))
  await userEvent.click(screen.getByRole('button', { name: '复制节点' }))
  expect(graph().nodes[2]).toEqual({ id: 'agent-2', type: 'workflow', position: { x: 50, y: 60 }, extra: 'keep', data: { kind: 'agent', title: '自定义', custom: { value: 42 } } })
  expect(graph().edges).toEqual([])
  await userEvent.click(screen.getByText('循环', { selector: 'strong' }))
  expect(screen.getByRole('button', { name: '复制节点' })).toBeDisabled()
})

it('returns to an arbitrary history step and drops future steps after a new edit', async () => {
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
  await userEvent.click(screen.getByRole('button', { name: '输出' }))
  await userEvent.click(screen.getByText('编辑历史'))
  expect(screen.getByText(/仅本次会话/)).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '步骤 0' }))
  expect(graph().nodes).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: '步骤 2' }))
  expect(graph().nodes).toHaveLength(3)
  await userEvent.click(screen.getByRole('button', { name: '步骤 0' }))
  await userEvent.click(screen.getByRole('button', { name: '条件分支' }))
  expect(screen.queryByRole('button', { name: '步骤 2' })).not.toBeInTheDocument()
})

it.each([
  ['损坏节点', { nodes: [null], edges: [] }],
  ['损坏连线', { nodes: [], edges: [{ id: 'bad', source: null, target: 'n' }] }],
  ['损坏视图', { nodes: [], edges: [], viewport: { x: 0, y: 0, zoom: 0 } }],
])('does not crash or change an unsupported graph with %s', (_description, value) => {
  const initialValue = JSON.stringify({ ...value, custom: { retained: true } })
  render(<Harness initialValue={initialValue} />)
  expect(screen.getByRole('alert')).toHaveTextContent('原始数据已保留')
  expect(graph()).toEqual(JSON.parse(initialValue))
})

it('renders an unknown but well-formed node without discarding it', () => {
  const initialValue = JSON.stringify({ nodes: [{ id: 'future', position: { x: 0, y: 0 }, data: { kind: 'future-kind', title: '未来节点', extra: true } }], edges: [] })
  render(<Harness initialValue={initialValue} />)
  expect(screen.getByText('未来节点')).toBeInTheDocument()
  expect(graph().nodes[0].data.extra).toBe(true)
})

it('deletes selected nodes and incident edges while retaining a protected start', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'start', position: { x: 10, y: 10 }, deletable: false, data: { kind: 'start', title: '开始' } },
    { id: 'a', position: { x: 300, y: 10 }, data: { kind: 'agent', title: 'A' } },
    { id: 'b', position: { x: 600, y: 10 }, data: { kind: 'output', title: 'B' } },
  ], edges: [{ id: 'e1', source: 'start', target: 'a' }, { id: 'e2', source: 'a', target: 'b' }], viewport: { x: 0, y: 0, zoom: 1 } })
  const { container } = render(<Harness initialValue={initialValue} />)
  const stage = container.querySelector('.workflow-canvas-stage') as HTMLElement
  const oldPointerEvent = window.PointerEvent; window.PointerEvent = MouseEvent as typeof PointerEvent
  try {
    fireEvent.pointerDown(stage, { clientX: 0, clientY: 0, button: 0 })
    fireEvent.pointerMove(stage, { clientX: 900, clientY: 200 })
    fireEvent.pointerUp(stage, { clientX: 900, clientY: 200 })
    await userEvent.click(screen.getByRole('button', { name: '删除选中节点 (3)' }))
    expect(graph().nodes.map((node: { id: string }) => node.id)).toEqual(['start'])
    expect(graph().edges).toEqual([])
  } finally { window.PointerEvent = oldPointerEvent }
})

it('reconnects an edge without dropping its handles or extensions', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'c1', position: { x: 0, y: 0 }, data: { kind: 'condition', title: '条件一', cases: [{ id: 'case-1', conditions: [] }] } },
    { id: 'c2', position: { x: 0, y: 200 }, data: { kind: 'condition', title: '条件二', cases: [{ id: 'case-1', conditions: [] }] } },
    { id: 'a', position: { x: 400, y: 0 }, data: { kind: 'agent', title: 'A' } },
    { id: 'b', position: { x: 400, y: 200 }, data: { kind: 'output', title: 'B' } },
  ], edges: [{ id: 'e1', source: 'c1', target: 'a', sourceHandle: 'case-1', targetHandle: 'in', custom: { keep: true } }], viewport: { x: 0, y: 0, zoom: 1 } })
  render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByText('连线 (1)'))
  await userEvent.selectOptions(screen.getByLabelText('连线 e1 起点'), 'c2')
  await userEvent.selectOptions(screen.getByLabelText('连线 e1 终点'), 'b')
  expect(graph().edges[0]).toEqual({ id: 'e1', source: 'c2', target: 'b', sourceHandle: 'case-1', targetHandle: 'in', custom: { keep: true } })
  await userEvent.selectOptions(screen.getByLabelText('连线 e1 起点'), 'a')
  expect(screen.getByRole('alert')).toHaveTextContent('无法重连')
  expect(graph().edges[0].source).toBe('c2')
})

it('arranges disconnected roots in two columns without overlapping the next layer or moving children', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'a', position: { x: 0, y: 0 }, data: { kind: 'agent', title: 'A' } },
    { id: 'loop', position: { x: 0, y: 0 }, initialWidth: 650, initialHeight: 300, data: { kind: 'loop', title: '循环' } },
    { id: 'other', position: { x: 0, y: 0 }, data: { kind: 'agent', title: '其他' } },
    { id: 'child', parentId: 'loop', position: { x: 40, y: 145 }, data: { kind: 'agent', title: '子节点', containerId: 'loop' } },
    { id: 'out', position: { x: 0, y: 0 }, data: { kind: 'output', title: '输出' } },
  ], edges: [{ id: 'e', source: 'a', target: 'out' }], viewport: { x: 0, y: 0, zoom: 1 } })
  render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByRole('button', { name: '整理节点' }))
  const find = (id: string) => graph().nodes.find((node: { id: string }) => node.id === id)
  expect(find('a').position.y).toBe(find('loop').position.y)
  expect(find('loop').position.x).toBeGreaterThan(find('a').position.x + 230)
  expect(find('other').position.y).toBeGreaterThan(find('loop').position.y + 300)
  expect(find('out').position.x).toBeGreaterThan(find('loop').position.x + 650)
  expect(find('child').position).toEqual({ x: 40, y: 145 })
})

it('snaps moved nodes to the grid only when enabled', async () => {
  const { container } = render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: '网格吸附' }))
  expect(screen.getByRole('button', { name: '网格吸附' })).toHaveAttribute('aria-pressed', 'true')
  const start = container.querySelector('[data-workflow-node="start"]') as HTMLElement
  const oldPointerEvent = window.PointerEvent; window.PointerEvent = MouseEvent as typeof PointerEvent
  try {
    fireEvent.pointerDown(start, { clientX: 40, clientY: 40, button: 0 })
    fireEvent.pointerMove(start, { clientX: 53, clientY: 67 })
    fireEvent.pointerUp(start, { clientX: 53, clientY: 67 })
    expect(graph().nodes[0].position).toEqual({ x: 60, y: 60 })
  } finally { window.PointerEvent = oldPointerEvent }
})

it('resizes containers without clipping members and keeps their positions', async () => {
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: '循环' }))
  const widthInput = screen.getByLabelText('容器宽度')
  fireEvent.change(widthInput, { target: { value: '800' } }); fireEvent.blur(widthInput)
  expect(graph().nodes.find((node: { id: string }) => node.id === 'loop-1').initialWidth).toBe(800)
  const updatedWidthInput = screen.getByLabelText('容器宽度')
  fireEvent.change(updatedWidthInput, { target: { value: '320' } }); fireEvent.blur(updatedWidthInput)
  expect(updatedWidthInput).toHaveValue(800)
  expect(graph().nodes.find((node: { id: string }) => node.id === 'loop-1').initialWidth).toBe(800)
  expect(graph().nodes.find((node: { id: string }) => node.id === 'loop-1-agent').position).toEqual({ x: 350, y: 145 })
})

it('reports a dangling edge without rewriting the original document', () => {
  const initialValue = JSON.stringify({ nodes: [{ id: 'start', position: { x: 0, y: 0 }, data: { kind: 'start' } }], edges: [{ id: 'e', source: 'start', target: 'missing', extra: true }] })
  render(<Harness initialValue={initialValue} />)
  expect(screen.getByRole('alert')).toHaveTextContent('引用了不存在的节点')
  expect(graph()).toEqual(JSON.parse(initialValue))
})

it('creates connected nodes only in supported loop and iteration scopes', async () => {
  render(<Harness />)
  expect(screen.getByRole('button', { name: '退出循环' })).toBeDisabled()
  await userEvent.click(screen.getByRole('button', { name: '循环' }))
  await userEvent.click(screen.getByRole('button', { name: '循环 Agent 输出 默认' }))
  expect(screen.getByRole('button', { name: '变量聚合' })).toBeDisabled()
  expect(screen.getByRole('button', { name: '退出循环' })).toBeEnabled()
  await userEvent.click(screen.getByRole('button', { name: '退出循环' }))
  expect(graph().nodes.find((node: { id: string }) => node.id === 'loopExit-1')).toMatchObject({ parentId: 'loop-1', data: { containerId: 'loop-1' } })
  expect(graph().edges).toContainEqual(expect.objectContaining({ source: 'loop-1-agent', target: 'loopExit-1' }))
  const loop = graph().nodes.find((node: { id: string }) => node.id === 'loop-1')
  const exit = graph().nodes.find((node: { id: string }) => node.id === 'loopExit-1')
  expect(loop.initialWidth).toBeGreaterThanOrEqual(exit.position.x + 230)
  expect(loop.initialHeight).toBeGreaterThanOrEqual(exit.position.y + 110)
  await userEvent.click(screen.getByRole('button', { name: '迭代' }))
  await userEvent.click(screen.getByRole('button', { name: '迭代 输出 iteration-entry' }))
  expect(screen.getByRole('button', { name: '输出' })).toBeDisabled()
  expect(screen.getByRole('button', { name: '变量聚合' })).toBeEnabled()
  await userEvent.click(screen.getByRole('button', { name: '变量聚合' }))
  const member = graph().nodes.find((node: { id: string }) => node.id === 'aggregator-1')
  expect(member.parentId).toBe('iteration-1')
  expect(member.data).not.toHaveProperty('containerId')
  expect(graph().edges).toContainEqual(expect.objectContaining({ source: 'iteration-1', sourceHandle: 'iteration-entry', target: 'aggregator-1' }))
  const iteration = graph().nodes.find((node: { id: string }) => node.id === 'iteration-1')
  expect(iteration.initialWidth).toBeGreaterThanOrEqual(member.position.x + 230)
  expect(iteration.initialHeight).toBeGreaterThanOrEqual(member.position.y + 110)
})

it('collapses iteration without discarding its members, edges or expanded dimensions and can undo', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'iter', position: { x: 20, y: 20 }, width: 640, height: 340, data: { kind: 'iteration', title: '迭代' } },
    { id: 'a', parentId: 'iter', position: { x: 40, y: 80 }, data: { kind: 'agent', title: '内部' } },
  ], edges: [{ id: 'e', source: 'iter', sourceHandle: 'iteration-entry', target: 'a' }], viewport: { x: 0, y: 0, zoom: 1 } })
  const { container } = render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByRole('button', { name: '折叠迭代' }))
  expect(container.querySelector('[data-workflow-node="a"]')).not.toBeInTheDocument()
  expect(graph().nodes[0]).toMatchObject({ width: 640, height: 340, data: { collapsed: true } })
  expect(graph().edges).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: '撤销' }))
  expect(container.querySelector('[data-workflow-node="a"]')).toBeInTheDocument()
  expect(graph().nodes[0]).toMatchObject({ width: 640, height: 340 })
})

it('asks before deleting a nonempty container and preserves it when cancelled', async () => {
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: '循环' }))
  await userEvent.click(screen.getByRole('button', { name: '删除节点' }))
  expect(screen.getByRole('alertdialog', { name: '确认删除容器' })).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: '取消' }))
  expect(graph().nodes).toHaveLength(4)
})

it('undoes an externally imported graph as one edit', async () => {
  function ImportHarness() {
    const [value, setValue] = useState(initial)
    const [externalEdit, setExternalEdit] = useState<{ before: string; id: number }>()
    return <><button onClick={() => { setExternalEdit({ before: value, id: 1 }); setValue(JSON.stringify({ nodes: [], edges: [] })) }}>替换图</button><WorkflowCanvas graph={value} onChange={setValue} externalEdit={externalEdit} /><output data-testid="graph">{value}</output></>
  }
  render(<ImportHarness />)
  await userEvent.click(screen.getByRole('button', { name: '替换图' }))
  expect(graph().nodes).toHaveLength(0)
  await userEvent.click(screen.getByRole('button', { name: '撤销' }))
  expect(graph()).toEqual(JSON.parse(initial))
})

it('does not undo through keyboard shortcuts when the canvas is inert', async () => {
  const { container } = render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Agent' }))
  const shell = container.querySelector('.workflow-canvas-shell') as HTMLElement
  const stage = screen.getByLabelText('工作流画布')
  shell.setAttribute('inert', '')
  fireEvent.keyDown(stage, { key: 'z', ctrlKey: true })
  expect(graph().nodes).toHaveLength(2)
  shell.removeAttribute('inert')
  fireEvent.keyDown(stage, { key: 'z', ctrlKey: true })
  expect(graph().nodes).toHaveLength(1)
})



it('synchronizes existing runtime and initial dimensions when resizing', async () => {
  const initialValue = JSON.stringify({ nodes: [{ id: 'iter', position: { x: 0, y: 0 }, width: 530, height: 260, initialWidth: 530, initialHeight: 260, data: { kind: 'iteration', title: '迭代' } }], edges: [] })
  render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByText('迭代', { selector: 'strong' }))
  const widthInput = screen.getByLabelText('容器宽度')
  fireEvent.change(widthInput, { target: { value: '800' } }); fireEvent.blur(widthInput)
  const heightInput = screen.getByLabelText('容器高度')
  fireEvent.change(heightInput, { target: { value: '400' } }); fireEvent.blur(heightInput)
  expect(graph().nodes[0]).toMatchObject({ width: 800, initialWidth: 800, height: 400, initialHeight: 400 })
})

it('synchronizes initial and existing runtime size when adding a container member', async () => {
  const initialValue = JSON.stringify({ nodes: [
    { id: 'iter', position: { x: 0, y: 0 }, width: 530, height: 260, initialWidth: 530, initialHeight: 260, data: { kind: 'iteration', title: '迭代' } },
    { id: 'a', parentId: 'iter', position: { x: 350, y: 90 }, data: { kind: 'agent', title: '内部' } },
  ], edges: [] })
  render(<Harness initialValue={initialValue} />)
  await userEvent.click(screen.getByRole('button', { name: '内部 输出 默认' }))
  await userEvent.click(screen.getByRole('button', { name: '变量聚合' }))
  const iteration = graph().nodes.find((node: { id: string }) => node.id === 'iter')
  const member = graph().nodes.find((node: { id: string }) => node.id === 'aggregator-1')
  expect(iteration.width).toBe(iteration.initialWidth)
  expect(iteration.height).toBe(iteration.initialHeight)
  expect(iteration.initialWidth).toBeGreaterThanOrEqual(member.position.x + 230)
  expect(iteration.initialHeight).toBeGreaterThanOrEqual(member.position.y + 110)
})
