import { AbsoluteFill, Easing, interpolate, spring, useCurrentFrame } from 'remotion'
import { Grain } from './brand/Grain.tsx'
import { Lockup } from './brand/Lockup.tsx'
import { Network } from './brand/Network.tsx'
import { FPS, SHOT_BACKGROUND } from './layout.ts'

const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const
const easeOut = Easing.bezier(0.16, 1, 0.3, 1)
const easeInOut = Easing.bezier(0.65, 0, 0.35, 1)

/**
 * The opening, kept plain: a dot grid surfaces and a sparse network links up around the edges, the
 * Ora mark fades into the centre as the network recedes, slides left as "Ora Cloud" comes out from
 * behind it, blinks once, and the frame dissolves into the shots' background.
 */
export function Intro() {
  const f = useCurrentFrame()
  const t = f / FPS
  const settle = (from: number, frames: number) => spring({ frame: f - from, fps: FPS, config: { damping: 200 }, durationInFrames: frames })
  const logo = spring({ frame: f - 100, fps: FPS, config: { damping: 200 }, durationInFrames: 50 })
  const leave = interpolate(f, [316, 356], [0, 1], { ...clamp, easing: easeInOut })
  const blink = Math.sin(Math.PI * interpolate(f, [290, 304], [0, 1], clamp))

  return <AbsoluteFill>
    <Network t={t} ground={interpolate(f, [0, 50], [0, 1], { ...clamp, easing: easeOut })}
      build={interpolate(f, [20, 130], [0, 1], clamp)} recede={interpolate(f, [110, 190], [0, 1], { ...clamp, easing: easeInOut })} />
    <AbsoluteFill style={{
      opacity: 1 - leave, transform: `scale(${1 + 0.03 * leave})`,
      filter: leave > 0 ? `blur(${leave * 14}px)` : undefined,
    }}>
      <Lockup state={{
        logo,
        shift: settle(160, 56),
        letter: i => settle(172 + i * 4, 40),
        tracking: interpolate(f, [172, 260], [0.12, 0], { ...clamp, easing: easeOut }),
        blink,
      }} />
    </AbsoluteFill>
    <AbsoluteFill style={{ background: SHOT_BACKGROUND, opacity: interpolate(f, [320, 358], [0, 1], { ...clamp, easing: easeInOut }) }} />
    <Grain frame={f} />
  </AbsoluteFill>
}
