import { afterEach, expect, it, vi } from 'vitest'
import { simulateDtsSubmission } from './dts'

afterEach(() => vi.unstubAllEnvs())
it('never returns a simulated success outside demo mode', async () => {
  vi.stubEnv('VITE_DEMO_MOCK', '')
  expect(await simulateDtsSubmission('task')).toBeNull()
})
it('reuses the receipt identity of a task', async () => {
  vi.stubEnv('VITE_DEMO_MOCK', '1')
  expect(await simulateDtsSubmission('abc-123')).toEqual(await simulateDtsSubmission('abc-123'))
  expect((await simulateDtsSubmission('abc-123'))?.number).toBe('DTS-DEMO-ABC123')
})
