import { describe, expect, it } from 'vitest'
import { entryDate, linkState } from '@/features/members/browser'

describe('Link metadata presentation', () => {
  it('distinguishes used, revoked, expired and active, with terminal state precedence', () => {
    const now = Date.parse('2026-10-27T00:00:00Z')
    const expired = { expiresAt: '2026-10-26T00:00:00Z' }
    expect(linkState(expired, now)).toBe('expired')
    expect(linkState({ ...expired, revokedAt: expired.expiresAt }, now)).toBe('revoked')
    expect(linkState({ ...expired, consumedAt: expired.expiresAt }, now)).toBe('used')
    expect(linkState({ expiresAt: '2026-10-28T00:00:00Z' }, now)).toBe('active')
    expect(entryDate(undefined, 'en-US')).toBe('—')
    expect(entryDate('not-a-date', 'zh-CN')).toBe('—')
    expect(entryDate(expired.expiresAt, 'en-US')).toContain('2026')
  })
})
