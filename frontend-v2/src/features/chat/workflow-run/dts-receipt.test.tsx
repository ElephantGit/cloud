import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import { DtsReceipt } from './dts-receipt'

const mock = vi.hoisted(() => ({ enabled: true, submit: vi.fn() }))
vi.mock('@/lib/demo/flags', () => ({ demoMockEnabled: () => mock.enabled }))
vi.mock('@/lib/demo/dts', () => ({ simulateDtsSubmission: mock.submit }))

beforeEach(() => { mock.enabled = true; mock.submit.mockReset() })

function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(<QueryClientProvider client={client}><MemoryRouter><DtsReceipt taskId="task" title="问题" description="证据" severity="中" module="服务" runId="run" /></MemoryRouter></QueryClientProvider>)
}

it('keeps the Cloud result visible when simulated delivery fails and can retry it', async () => {
  mock.submit.mockRejectedValueOnce(new Error('demo delivery failed')).mockResolvedValueOnce({ number: 'DTS-DEMO-TASK', taskId: 'task', mode: 'demo' })
  mount()
  expect(await screen.findByRole('alert')).toHaveTextContent('Cloud 任务已保留')
  fireEvent.click(screen.getByRole('button', { name: '重试模拟提交' }))
  expect(await screen.findByRole('button', { name: /DTS-DEMO-TASK/ })).toBeInTheDocument()
  expect(mock.submit).toHaveBeenCalledTimes(2)
  expect(mock.submit).toHaveBeenLastCalledWith('task')
})

it('does not submit or display a fake receipt when demo mode is disabled', () => {
  mock.enabled = false
  const { container } = mount()
  expect(container).toBeEmptyDOMElement()
  expect(mock.submit).not.toHaveBeenCalled()
})
