import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, expect, it, vi } from 'vitest'
import Page from './page'

vi.mock('./analysis/list', () => ({ default: () => <section aria-label='会话分析'>会话分析内容</section> }))
vi.mock('./real-overview', () => ({ default: () => <div>真实任务概览</div> }))

afterEach(() => vi.unstubAllEnvs())

it('keeps real overview above session analysis', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '')
  render(<MemoryRouter><Page /></MemoryRouter>)
  expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  const team = screen.getByRole('region', { name: '团队概览' })
  expect(team).toHaveTextContent('真实任务概览')
  expect(screen.queryByRole('region', { name: /^空间用量$/ })).not.toBeInTheDocument()
  const analysis = screen.getByRole('region', { name: '会话分析' })
  expect(team.compareDocumentPosition(analysis) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('adds demo space usage without replacing real overview or session analysis', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  render(<MemoryRouter><Page /></MemoryRouter>)
  const usage = screen.getByRole('region', { name: /^空间用量$/ })
  expect(within(usage).getByText('演示数据')).toBeInTheDocument()
  const team = screen.getByRole('region', { name: '团队概览' })
  expect(team).toHaveTextContent('真实任务概览')
  const analysis = screen.getByRole('region', { name: '会话分析' })
  expect(usage.compareDocumentPosition(team) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(team.compareDocumentPosition(analysis) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
