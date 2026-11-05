import { Easing, interpolate } from 'remotion'
import { PANEL_ENTER, PANEL_EXIT } from '../record/timing.ts'

const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const
const ease = Easing.bezier(0.16, 1, 0.3, 1)

/** How far a panel has come in, from 0 to 1, `age` output milliseconds into its `ms`. */
export function panelProgress(age: number, ms: number): number {
  const enter = interpolate(age, [0, PANEL_ENTER], [0, 1], { ...clamp, easing: ease })
  return enter * interpolate(age, [ms - PANEL_EXIT, ms], [1, 0], { ...clamp, easing: Easing.bezier(0.65, 0, 0.35, 1) })
}
