import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { expect, it, vi } from 'vitest'
import Page from './page'

vi.mock('./analysis/list', () => ({ default: () => <section aria-label='会话分析'>会话分析内容</section> }))
vi.mock('./real-overview', () => ({ default: () => <div>真实任务概览</div> }))

it('keeps real overview above session analysis', () => {
  render(<MemoryRouter><Page /></MemoryRouter>)
  expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  const team = screen.getByRole('region', { name: '团队概览' })
  expect(team).toHaveTextContent('真实任务概览')
  const analysis = screen.getByRole('region', { name: '会话分析' })
  expect(team.compareDocumentPosition(analysis) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
