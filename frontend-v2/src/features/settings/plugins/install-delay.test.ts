import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  INSTALL_DELAY_MAX_MS,
  INSTALL_DELAY_MIN_MS,
  installDelayMs,
  waitForInstallDelay,
} from './install-delay'

afterEach(() => {
  vi.useRealTimers()
})

describe('install delay', () => {
  it('maps the random range inclusively from three to five seconds', () => {
    expect(installDelayMs(() => 0)).toBe(INSTALL_DELAY_MIN_MS)
    expect(installDelayMs(() => 0.999_999)).toBe(INSTALL_DELAY_MAX_MS)
    expect(installDelayMs(() => 0.5)).toBe(4_000)
  })

  it('waits for the selected delay', async () => {
    vi.useFakeTimers()
    let settled = false
    const waiting = waitForInstallDelay(2_000).then(() => {
      settled = true
    })

    await vi.advanceTimersByTimeAsync(1_999)
    expect(settled).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    await waiting
    expect(settled).toBe(true)
  })
})
