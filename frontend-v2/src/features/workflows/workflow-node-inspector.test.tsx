import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { WorkflowNodeInspector } from './workflow-node-inspector'
import type { CanvasNode } from './workflow-canvas'

function Harness({ initial, onUpdate }: { initial: CanvasNode; onUpdate: (node: CanvasNode) => void }) {
  const [node, setNode] = useState(initial)
  return <WorkflowNodeInspector node={node} nodes={[node]} onClose={() => {}} onChange={(next) => { setNode(next); onUpdate(next) }} />
}

describe('workflow node inspector', () => {
  it('edits a condition selector without losing extension fields or a typed dot', async () => {
    const update = vi.fn()
    const node: CanvasNode = { id: 'c', position: { x: 0, y: 0 }, custom: 42, data: { kind: 'condition', title: '条件', description: '', extension: 'keep', cases: [{ id: 'case-1', logic: 'and', extra: 9, conditions: [{ variableSelector: [], operator: 'equals', value: '' }] }] } }
    render(<Harness initial={node} onUpdate={update}/>)
    await userEvent.type(screen.getByLabelText('变量'), 'start.input')
    expect(update.mock.lastCall?.[0]).toMatchObject({ custom: 42, data: { extension: 'keep', cases: [{ id: 'case-1', extra: 9, conditions: [{ variableSelector: ['start', 'input'] }] }] } })
  })

  it('adds output bindings and keeps the nested selector shape', async () => {
    const update = vi.fn()
    render(<Harness initial={{ id: 'out', position: { x: 0, y: 0 }, data: { kind: 'output', title: '输出', description: '', outputs: [] } }} onUpdate={update}/>)
    await userEvent.click(screen.getByRole('button', { name: '添加' }))
    await userEvent.type(screen.getByLabelText('变量'), 'agent.output')
    expect(update.mock.lastCall?.[0].data.outputs).toEqual([{ name: '', variableSelector: ['agent', 'output'] }])
  })

  it('keeps read-only versions uneditable', () => {
    render(<WorkflowNodeInspector readOnly node={{ id: 's', position: { x: 0, y: 0 }, data: { kind: 'start', title: '开始', description: '', inputVariables: [] } }} onChange={() => {}} onClose={() => {}}/>)
    expect(screen.getByLabelText('名称')).toBeDisabled()
    expect(screen.getByRole('button', { name: '添加' })).toBeDisabled()
  })
  it('normalizes a file input into a safe workspace file reference', async () => {
    const update = vi.fn()
    render(<Harness initial={{ id: 's', position: { x: 0, y: 0 }, data: { kind: 'start', title: '开始', description: '', inputVariables: [{ name: 'document', valueType: 'file', fieldType: 'file' }] } }} onUpdate={update}/>)
    const value = screen.getByLabelText('初始值（可选）')
    fireEvent.change(value, { target: { value: 'docs/input.pdf' } })
    fireEvent.blur(value)
    expect(update.mock.lastCall?.[0].data.inputVariables).toEqual([{ name: 'document', valueType: 'file', fieldType: 'file', value: { kind: 'workspace_file', path: 'docs/input.pdf' } }])
    fireEvent.change(screen.getByLabelText('初始值（可选）'), { target: { value: '../secret' } })
    fireEvent.blur(screen.getByLabelText('初始值（可选）'))
    expect(screen.getByRole('alert')).toHaveTextContent('请输入有效的 file 值')
    expect(update.mock.lastCall?.[0].data.inputVariables[0].value).toEqual({ kind: 'workspace_file', path: 'docs/input.pdf' })
  })

  it('migrates legacy branch rules only when edited and keeps the node extensions', async () => {
    const update = vi.fn()
    render(<Harness initial={{ id: 'c', position: { x: 0, y: 0 }, data: { kind: 'condition', title: '条件', description: '', extension: 7, conditionBranches: [{ conditions: [{ variable: 'start.input', operator: 'is_empty', value: '' }] }] } }} onUpdate={update}/>)
    expect(screen.getByLabelText('变量')).toHaveValue('start.input')
    expect(update).not.toHaveBeenCalled()
    await userEvent.selectOptions(screen.getByLabelText('条件关系'), 'or')
    const changed = update.mock.lastCall?.[0]
    expect(changed.data).not.toHaveProperty('conditionBranches')
    expect(changed.data).toMatchObject({ extension: 7, cases: [{ id: 'case-1', logic: 'or', conditions: [{ variableSelector: ['start', 'input'], operator: 'empty' }] }] })
  })

  it('updates loop and iteration configurations without changing ownership', async () => {
    const update = vi.fn()
    const { unmount } = render(<Harness initial={{ id: 'loop', parentId: 'outer', position: { x: 0, y: 0 }, data: { kind: 'loop', title: '循环', description: '', containerId: 'outer', loopConfig: { maxIterations: 3, variables: [], until: { logic: 'and', conditions: [] }, outputs: [], extension: 4 } } }} onUpdate={update}/>)
    await userEvent.clear(screen.getByLabelText('最大循环次数'))
    await userEvent.type(screen.getByLabelText('最大循环次数'), '8')
    expect(update.mock.lastCall?.[0]).toMatchObject({ parentId: 'outer', data: { containerId: 'outer', loopConfig: { maxIterations: 8, extension: 4 } } })
    unmount()
    render(<Harness initial={{ id: 'it', position: { x: 0, y: 0 }, data: { kind: 'iteration', title: '迭代', description: '', iterationConfig: { iteratorSelector: [], collectSelector: [], errorStrategy: 'fail', maxIterations: 50 } } }} onUpdate={update}/>)
    await userEvent.type(screen.getByLabelText('迭代数组'), 'start.items')
    await userEvent.selectOptions(screen.getByLabelText('错误处理'), 'continue')
    expect(update.mock.lastCall?.[0].data.iterationConfig).toMatchObject({ iteratorSelector: ['start', 'items'], errorStrategy: 'continue', maxIterations: 50 })
  })

  it('reorders aggregator candidates and edits agent settings', async () => {
    const update = vi.fn()
    const { unmount } = render(<Harness initial={{ id: 'a', position: { x: 0, y: 0 }, data: { kind: 'aggregator', title: '聚合', description: '', aggregatorConfig: { variables: [['one', 'output'], ['two', 'output']] } } }} onUpdate={update}/>)
    await userEvent.click(screen.getAllByRole('button', { name: '上移' })[1])
    expect(update.mock.lastCall?.[0].data.aggregatorConfig.variables).toEqual([['two', 'output'], ['one', 'output']])
    unmount()
    render(<Harness initial={{ id: 'agent', position: { x: 0, y: 0 }, data: { kind: 'agent', title: 'Agent', description: '', agentConfig: { schemaVersion: 3, executor: { agentCli: 'cli', modelId: 'model', extra: 2 }, roleId: '', skills: [], mcps: [], prompt: '' } } }} onUpdate={update}/>)
    expect(screen.getByRole('checkbox', { name: '启用重试' })).toBeChecked()
    await userEvent.type(screen.getByLabelText('自定义 Prompt'), 'hello')
    expect(update.mock.lastCall?.[0].data.agentConfig).toMatchObject({ executor: { agentCli: 'cli', modelId: 'model', extra: 2 }, prompt: 'hello' })
    await userEvent.selectOptions(screen.getByLabelText('插入变量'), 'sys.workflow_id')
    await userEvent.click(screen.getByRole('button', { name: '插入' }))
    expect(update.mock.lastCall?.[0].data.agentConfig.prompt).toBe('hello{{#sys.workflow_id#}}')
  })

  it('explains loop exit without modifying its node', () => {
    const update = vi.fn()
    render(<Harness initial={{ id: 'exit', parentId: 'loop', position: { x: 0, y: 0 }, data: { kind: 'loopExit', title: '退出循环', description: '', containerId: 'loop' } }} onUpdate={update}/>)
    expect(screen.getByText('到达此节点时退出所在循环。')).toBeInTheDocument()
    expect(update).not.toHaveBeenCalled()
  })

  it('suggests only upstream variables and keeps iteration members private', () => {
    const start: CanvasNode = { id: 'start', position: { x: 0, y: 0 }, data: { kind: 'start', inputVariables: [{ name: 'items', valueType: 'array[string]' }] } }
    const iteration: CanvasNode = { id: 'iter', position: { x: 0, y: 0 }, data: { kind: 'iteration', iterationConfig: { iteratorSelector: ['start', 'items'] } } }
    const child: CanvasNode = { id: 'inner', parentId: 'iter', position: { x: 0, y: 0 }, data: { kind: 'agent' } }
    const peer: CanvasNode = { id: 'peer', parentId: 'iter', position: { x: 0, y: 0 }, data: { kind: 'agent', title: 'Peer', description: '' } }
    const outsider: CanvasNode = { id: 'out', position: { x: 0, y: 0 }, data: { kind: 'agent' } }
    const unrelated: CanvasNode = { id: 'other', position: { x: 0, y: 0 }, data: { kind: 'agent' } }
    const { rerender } = render(<WorkflowNodeInspector node={peer} nodes={[start, iteration, child, peer, outsider, unrelated]} edges={[{ source: 'start', target: 'iter' }, { source: 'iter', target: 'inner' }, { source: 'inner', target: 'peer' }, { source: 'iter', target: 'out' }]} onClose={() => {}} onChange={() => {}} />)
    const options = () => Array.from(screen.getByLabelText('插入变量').querySelectorAll('option')).map((option) => option.value)
    expect(options()).toContain('start.items')
    expect(options()).toContain('iter.item')
    expect(options()).toContain('iter.index')
    expect(options()).toContain('inner.output')
    expect(options()).not.toContain('iter.output')
    expect(options()).not.toContain('other.output')
    rerender(<WorkflowNodeInspector node={outsider} nodes={[start, iteration, child, peer, outsider, unrelated]} edges={[{ source: 'start', target: 'iter' }, { source: 'iter', target: 'inner' }, { source: 'inner', target: 'peer' }, { source: 'iter', target: 'out' }]} onClose={() => {}} onChange={() => {}} />)
    expect(options()).toContain('iter.output')
    expect(options()).toContain('iter.entries')
    expect(options()).not.toContain('iter.item')
    expect(options()).not.toContain('inner.output')
  })

  it('restricts iteration choices to arrays and direct region outputs', () => {
    const start: CanvasNode = { id: 'start', position: { x: 0, y: 0 }, data: { kind: 'start', inputVariables: [{ name: 'items', valueType: 'array[string]' }, { name: 'text', valueType: 'string' }] } }
    const iter: CanvasNode = { id: 'iter', position: { x: 0, y: 0 }, data: { kind: 'iteration', title: '迭代', description: '', iterationConfig: {} } }
    const child: CanvasNode = { id: 'child', parentId: 'iter', position: { x: 0, y: 0 }, data: { kind: 'agent' } }
    render(<WorkflowNodeInspector node={iter} nodes={[start, iter, child]} edges={[{ source: 'start', target: 'iter' }]} onClose={() => {}} onChange={() => {}} />)
    const options = (name: string) => { const input = screen.getByLabelText(name); const list = document.getElementById(input.getAttribute('list')!); return Array.from(list!.querySelectorAll('option')).map((option) => option.value) }
    expect(options('迭代数组')).toContain('start.items')
    expect(options('迭代数组')).not.toContain('start.text')
    expect(options('每轮收集变量')).toEqual(['child.output'])
  })

  it('exposes loop carried variables inside and declared outputs outside', () => {
    const loop: CanvasNode = { id: 'loop', position: { x: 0, y: 0 }, data: { kind: 'loop', loopConfig: { variables: [{ name: 'value', valueType: 'number' }], outputs: [{ name: 'result', variableSelector: ['worker', 'output'] }] } } }
    const start: CanvasNode = { id: 'round', parentId: 'loop', position: { x: 0, y: 0 }, data: { kind: 'start' } }
    const worker: CanvasNode = { id: 'worker', parentId: 'loop', position: { x: 0, y: 0 }, data: { kind: 'agent', title: 'Agent', description: '' } }
    const downstream: CanvasNode = { id: 'downstream', position: { x: 0, y: 0 }, data: { kind: 'agent', title: 'Agent', description: '' } }
    const nodes = [loop, start, worker, downstream]
    const edges = [{ source: 'round', target: 'worker' }, { source: 'loop', target: 'downstream' }]
    const { rerender } = render(<WorkflowNodeInspector node={worker} nodes={nodes} edges={edges} onClose={() => {}} onChange={() => {}} />)
    const options = () => Array.from(screen.getByLabelText('插入变量').querySelectorAll('option')).map((option) => option.value)
    expect(options()).toContain('loop.value')
    expect(options()).not.toContain('loop.result')
    rerender(<WorkflowNodeInspector node={downstream} nodes={nodes} edges={edges} onClose={() => {}} onChange={() => {}} />)
    expect(options()).toContain('loop.result')
    expect(options()).not.toContain('loop.output')
    expect(options()).not.toContain('loop.value')
    expect(options()).not.toContain('worker.output')
  })

  it.each([null, ''])('leaves an unchanged %s initial value untouched on blur', (value) => {
    const update = vi.fn()
    render(<Harness initial={{ id: 'start', position: { x: 0, y: 0 }, data: { kind: 'start', title: '开始', description: '', inputVariables: [{ name: 'input', valueType: 'string', value }] } }} onUpdate={update}/>)
    fireEvent.blur(screen.getByLabelText('初始值（可选）'))
    expect(update).not.toHaveBeenCalled()
  })

  it.each(['string', 'secret'])('retains an intentionally cleared %s as an empty string', (valueType) => {
    const update = vi.fn()
    render(<Harness initial={{ id: 'start', position: { x: 0, y: 0 }, data: { kind: 'start', title: '开始', description: '', inputVariables: [{ name: 'input', valueType, value: 'text' }] } }} onUpdate={update}/>)
    const input = screen.getByLabelText('初始值（可选）')
    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)
    expect(update.mock.lastCall?.[0].data.inputVariables).toEqual([{ name: 'input', valueType, value: '' }])
  })

})
