import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { ScheduleOutput } from './schedule-output'

test('renders report text and all structured fields, retaining the original payload on demand', async () => {
  const text = JSON.stringify({ report: '# 巡检报告\n\n**全部通过**', metrics: { count: 5 }, warning: '演示数据' })
  render(<ScheduleOutput text={text}/> )
  expect(screen.getByRole('heading', { name: '巡检报告' })).toBeInTheDocument()
  expect(screen.getByText('全部通过')).toBeInTheDocument()
  expect(screen.getByText('演示数据')).toBeInTheDocument()
  expect(screen.getByText(/"count": 5/)).toBeInTheDocument()
  expect(screen.queryByText(text)).not.toBeInTheDocument()
  await userEvent.click(screen.getByText('查看原始输出'))
  expect(await screen.findByText(text)).toBeInTheDocument()
})

test('preserves malformed JSON as text and does not execute embedded HTML', () => {
  const { container } = render(<ScheduleOutput text={'{"report": broken\n<script>alert(1)</script>'}/>)
  expect(container.querySelector('script')).toBeNull()
  expect(screen.getByText(/report/)).toBeInTheDocument()
})
