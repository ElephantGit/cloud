import { AbsoluteFill, Easing, interpolate, useCurrentFrame } from 'remotion'
import { Grain } from './brand/Grain.tsx'
import { Lockup } from './brand/Lockup.tsx'
import { SHOT_BACKGROUND } from './layout.ts'

/** The closing, 5 s at 60 fps. */
export const OUTRO_FRAMES = 300

const TAGLINE = '协同办公新常态'
const LIFT = -90

const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const
const easeOut = Easing.bezier(0.16, 1, 0.3, 1)

/**
 * The closing: the shots' background clears to a calm plain, the lockup fades in where the opening
 * left it (raised a little to make room), and the tagline settles beneath a hairline. No light
 * field and no flourish: the opening already made the entrance.
 */
export function Outro() {
  const f = useCurrentFrame()
  const fade = (from: number, frames: number) => interpolate(f, [from, from + frames], [0, 1], { ...clamp, easing: easeOut })
  const spacing = interpolate(f, [80, 170], [0.62, 0.38], { ...clamp, easing: easeOut })

  return <AbsoluteFill style={{ background: 'radial-gradient(ellipse 80% 75% at 50% 45%, #ffffff 0%, #f5f5fa 62%, #eceef6 100%)' }}>
    <AbsoluteFill style={{ background: SHOT_BACKGROUND, opacity: 1 - fade(0, 40) }} />
    <Lockup state={{
      logo: fade(18, 48), shift: 1, y: LIFT,
      letter: i => fade(30 + i * 2, 44),
      tracking: interpolate(f, [30, 110], [0.05, 0], { ...clamp, easing: easeOut }),
    }} />
    <div style={{
      position: 'absolute', left: '50%', top: 1080 + 175, height: 3, width: 640 * fade(70, 50), transform: 'translateX(-50%)',
      background: 'linear-gradient(90deg, rgba(90, 86, 200, 0), rgba(90, 86, 200, 0.45), rgba(90, 86, 200, 0))',
    }} />
    <div style={{
      position: 'absolute', left: 0, right: 0, top: 1080 + 250, display: 'flex', justifyContent: 'center',
      fontFamily: '"Noto Sans SC Variable", sans-serif', fontSize: 104, fontWeight: 500, color: '#4a4d66',
      letterSpacing: `${spacing}em`, marginRight: `-${spacing}em`, whiteSpace: 'pre',
    }}>
      {[...TAGLINE].map((ch, i) => {
        const p = fade(84 + i * 6, 46)
        return <span key={i} style={{
          display: 'inline-block', opacity: p, transform: `translateY(${(1 - p) * 26}px)`,
          filter: p < 1 ? `blur(${(1 - p) * 10}px)` : undefined,
        }}>{ch}</span>
      })}
    </div>
    <Grain frame={f} opacity={0.04} />
  </AbsoluteFill>
}
