import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { CdaseDemo, cdaseInput } from './cdase'
import { demoMockEnabled } from './flags'

afterEach(() => { vi.useRealTimers(); vi.unstubAllEnvs() })
it('is unavailable unless the demo switch is explicitly enabled', () => {
  vi.stubEnv('VITE_DEMO_MOCK', '')
  expect(demoMockEnabled()).toBe(false)
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  expect(demoMockEnabled()).toBe(true)
})
it('reviews the supported input and keeps report, risk counts and verification claims consistent', async () => {
  vi.useFakeTimers()
  render(<CdaseDemo />)
  fireEvent.click(screen.getByRole('button', { name: '选择 cdase SFMEA 评审' }))
  fireEvent.change(screen.getByLabelText('评审任务'), { target: { value: 'different task' } })
  expect(screen.getByRole('button', { name: '开始评审' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: '填入示例任务' }))
  expect(screen.getByLabelText('评审任务')).toHaveValue(cdaseInput)
  fireEvent.click(screen.getByRole('button', { name: '开始评审' }))
  for (let i = 0; i < 4; i++) await act(() => vi.advanceTimersByTimeAsync(1000))
  expect(screen.getByRole('region', { name: '评审结果' })).toHaveTextContent('1 份报告')
  expect(screen.getByText('已形成风险清单与待验证项，尚未执行测试。')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '查看评审报告' }))
  expect(screen.getByRole('article', { name: 'SFMEA 评审报告' })).toHaveTextContent('reports/sfmea-review.md')
  expect(screen.getByRole('article')).toHaveTextContent('并发提交同一退款单')
  expect(screen.getByRole('article')).toHaveTextContent('渠道超时和延迟回执')
})
