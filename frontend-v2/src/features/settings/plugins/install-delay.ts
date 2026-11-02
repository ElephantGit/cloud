export const INSTALL_DELAY_MIN_MS = 1_000
export const INSTALL_DELAY_MAX_MS = 5_000

/** A visible installation takes between one and five seconds, inclusive. */
export function installDelayMs(random: () => number = Math.random): number {
  const span = INSTALL_DELAY_MAX_MS - INSTALL_DELAY_MIN_MS + 1
  return INSTALL_DELAY_MIN_MS + Math.floor(random() * span)
}

export function waitForInstallDelay(delayMs: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, delayMs))
}
