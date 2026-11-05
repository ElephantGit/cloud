import { demoMockEnabled } from './flags'

/** A browser-only DTS receipt. It never represents a request to Huawei DTS. */
export interface DtsReceipt { number: string; taskId: string; mode: 'demo' }

/** Stable task identity gives repeated reports the same simulated receipt. */
export async function simulateDtsSubmission(taskId: string): Promise<DtsReceipt | null> {
  if (!demoMockEnabled()) return null
  return { number: `DTS-DEMO-${taskId.replaceAll('-', '').slice(0, 12).toUpperCase()}`, taskId, mode: 'demo' }
}
