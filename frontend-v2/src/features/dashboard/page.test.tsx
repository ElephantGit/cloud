import { render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { dashboardDemo } from '@/lib/demo/dashboard'
import { MemoryRouter } from 'react-router'
import Page from './page'

vi.mock('./analysis/list', () => ({ default: () => <section aria-label='会话分析'>会话分析内容</section> }))

afterEach(() => vi.unstubAllEnvs())

it('keeps the placeholder while the demo mock is off', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '')
  render(<MemoryRouter><Page /></MemoryRouter>)
  expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
  expect(screen.getByRole('region', { name: '会话分析' })).toBeInTheDocument()
  expect(screen.getByText('开发中')).toBeInTheDocument()
  expect(screen.queryByRole('region', { name: '团队进度' })).not.toBeInTheDocument()
})

it('renders the demo dashboard only when the demo mock is on', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  render(<MemoryRouter><Page /></MemoryRouter>)
  expect(screen.queryByText('演示数据')).not.toBeInTheDocument()

  // Every member row is shown, with the counts it was given.
  const team = screen.getByRole('region', { name: '团队进度' })
  for (const member of dashboardDemo.members) expect(within(team).getByText(member.name)).toBeInTheDocument()
  const lead = within(team).getByText('陈默').closest('li')!
  expect(within(lead).getByText('进行中 1')).toBeInTheDocument()
  expect(within(lead).getByText('本周完成 3')).toBeInTheDocument()
  // Five of six tasks cleared, so the bar and the label agree.
  expect(within(lead).getByRole('progressbar', { name: '陈默 完成率' })).toHaveAttribute('aria-valuenow', '83')

  // Agent output lists both states and the calling agent.
  const outputs = screen.getByRole('region', { name: 'Agent 产出' })
  expect(within(outputs).getAllByRole('listitem')).toHaveLength(dashboardDemo.outputs.length)
  expect(within(outputs).getAllByText('待验收')).toHaveLength(dashboardDemo.outputs.filter(output => output.status === 'review').length)
  expect(within(outputs).getAllByText(/OpenCode ·/)).toHaveLength(dashboardDemo.outputs.filter(output => output.agent === 'OpenCode').length)

  expect(screen.queryByText(/真实统计待任务聚合接口接入后替换/)).not.toBeInTheDocument()
})

it('places session analysis after the team overview', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  render(<MemoryRouter><Page /></MemoryRouter>)
  const team = screen.getByRole('region', { name: '团队概览' })
  const analysis = screen.getByRole('region', { name: '会话分析' })
  expect(team.compareDocumentPosition(analysis) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})
