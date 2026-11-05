import { demoMockEnabled } from './flags'

/** A demo-only snapshot of a workspace aggregation, independent of real session observations. */
export interface SpaceUsage {
  period: string
  inputTokens: number
  outputTokens: number
  requests: number
  tools: { kind: string; count: number }[]
}

// Replace this module when the backend exposes an authorized, time-bounded workspace token/tool
// aggregation. Never derive workspace totals from the current user's paginated session list.
const snapshot: SpaceUsage = {
  period: '本周 · 11-02 至 11-06',
  inputTokens: 960_000,
  outputTokens: 128_000,
  requests: 42,
  tools: [
    { kind: 'read', count: 84 },
    { kind: 'edit', count: 36 },
    { kind: 'execute', count: 30 },
    { kind: 'search', count: 18 },
  ],
}

/** Returns demonstration usage only with the shared explicit opt-in. */
export function spaceUsageDemo(): SpaceUsage | null {
  return demoMockEnabled() ? snapshot : null
}
