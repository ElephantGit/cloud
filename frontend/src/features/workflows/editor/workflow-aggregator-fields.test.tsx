import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { WorkflowNodeData } from '@/features/workflows/runtime/types'
import { AggregatorNodeFields } from '@/features/workflows/editor/workflow-aggregator-fields'
import { renderWithProviders } from '@/test/render'

/** The node the panel edits; candidates are the only thing it reads. */
function aggregatorData(variables: string[][]): WorkflowNodeData {
  return {
    kind: 'aggregator',
    title: '变量聚合',
    description: '',
    aggregatorConfig: { variables },
  }
}

describe('AggregatorNodeFields', () => {
  it('adds a candidate and writes its root selector', () => {
    let data = aggregatorData([])
    const onChange = vi.fn<(next: WorkflowNodeData) => void>((next) => {
      data = next
    })
    const view = renderWithProviders(<AggregatorNodeFields data={data} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: '添加候选' }))
    expect(data.aggregatorConfig?.variables).toEqual([[]])

    view.rerender(<AggregatorNodeFields data={data} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('候选 1'), { target: { value: 'writer.output' } })
    expect(data.aggregatorConfig?.variables).toEqual([['writer', 'output']])
  })

  it('keeps the declared order when a candidate is removed', () => {
    let data = aggregatorData([
      ['a', 'output'],
      ['b', 'output'],
    ])
    const onChange = vi.fn<(next: WorkflowNodeData) => void>((next) => {
      data = next
    })
    renderWithProviders(<AggregatorNodeFields data={data} onChange={onChange} />)

    fireEvent.click(screen.getByRole('button', { name: '删除候选 1' }))

    expect(data.aggregatorConfig?.variables).toEqual([['b', 'output']])
  })
})
