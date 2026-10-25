import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
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

/**
 * Renders the panel and tracks the latest node it published.
 *
 * `published.data` is what a real editor would persist, so each assertion reads the shape the save
 * path would write instead of a captured mock call.
 */
function renderPanel(variables: string[][]) {
  const published = { data: aggregatorData(variables) }
  const onChange = (next: WorkflowNodeData): void => {
    published.data = next
  }
  const view = renderWithProviders(
    <AggregatorNodeFields data={published.data} onChange={onChange} />,
  )
  return { published, onChange, view }
}

describe('AggregatorNodeFields', () => {
  it('adds a candidate and writes its root selector', () => {
    const { published, onChange, view } = renderPanel([])

    fireEvent.click(screen.getByRole('button', { name: '添加候选' }))
    expect(published.data.aggregatorConfig?.variables).toEqual([[]])

    view.rerender(<AggregatorNodeFields data={published.data} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('候选 1'), { target: { value: 'writer.output' } })
    expect(published.data.aggregatorConfig?.variables).toEqual([['writer', 'output']])
  })

  it('moves a candidate up to change the published priority', () => {
    const { published } = renderPanel([
      ['a', 'output'],
      ['b', 'output'],
    ])

    fireEvent.click(screen.getByRole('button', { name: '上移候选 2' }))

    expect(published.data.aggregatorConfig?.variables).toEqual([
      ['b', 'output'],
      ['a', 'output'],
    ])
  })

  it('keeps the declared order when a candidate is removed', () => {
    const { published } = renderPanel([
      ['a', 'output'],
      ['b', 'output'],
    ])

    fireEvent.click(screen.getByRole('button', { name: '删除候选 1' }))

    expect(published.data.aggregatorConfig?.variables).toEqual([['b', 'output']])
  })
})
