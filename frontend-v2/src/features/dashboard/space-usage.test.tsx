import { render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { SpaceUsageSection } from './space-usage'

afterEach(() => vi.unstubAllEnvs())

it.each(['', '0', 'true'])('does not expose synthetic workspace usage with flag %s', flag => {
  vi.stubEnv('VITE_DEMO_MOCK', flag)
  render(<SpaceUsageSection />)
  expect(screen.queryByRole('region', { name: '空间用量' })).not.toBeInTheDocument()
  expect(screen.queryByText('960,000')).not.toBeInTheDocument()
})

it('labels the mock and keeps aggregate tool calls equal to its category counts', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  render(<SpaceUsageSection />)
  const usage = screen.getByRole('region', { name: '空间用量' })
  expect(within(usage).getByText('演示数据')).toBeInTheDocument()
  expect(within(usage).getByText('当前空间 · 本周 · 11-02 至 11-06')).toBeInTheDocument()
  expect(within(usage).getByRole('article', { name: '输入 Token' })).toHaveTextContent('960,000')
  expect(within(usage).getByRole('article', { name: '输出 Token' })).toHaveTextContent('128,000')
  const counts = within(screen.getByRole('region', { name: '空间工具调用分布' }))
    .getAllByRole('listitem').map(row => Number(row.textContent?.match(/(\d+) 次/)?.[1]))
  expect(within(usage).getByRole('article', { name: '工具调用' })).toHaveTextContent(String(counts.reduce((a, b) => a + b, 0)))
  expect(within(usage).getByText(/下方会话分析展示真实运行记录/)).toBeInTheDocument()
})
