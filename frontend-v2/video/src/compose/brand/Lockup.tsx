import { useLayoutEffect, useRef, useState } from 'react'
import { continueRender, delayRender } from 'remotion'
import { HEIGHT, WIDTH } from '../layout.ts'
import { OraLogo } from './OraLogo.tsx'

/** The lockup's proportions on the 4K canvas. */
export const LOGO_SIZE = 400
const GAP = 100
const FONT_SIZE = 250

const WORDS = [
  { text: 'Ora', weight: 640, color: '#15151d' },
  { text: ' ', weight: 400, color: 'transparent' },
  { text: 'Cloud', weight: 380, color: '#4a4c86' },
]
const LETTERS = WORDS.flatMap(w => [...w.text].map(ch => ({ ch, weight: w.weight, color: w.color })))

/** How far along each part of the lockup is; every number runs from 0 (absent) to 1 (in place). */
export interface LockupState {
  logo: number
  /** 0 keeps the logo alone in the centre of the frame, 1 moves it to its place beside the name. */
  shift: number
  /** How far the letter at `index` (spaces count) has arrived. */
  letter: (index: number) => number
  /** Extra letter spacing in em, which closes to 0 as the name settles. */
  tracking?: number
  blink?: number
  /** Vertical offset of the whole lockup from the frame's centre, in canvas pixels. */
  y?: number
}

/**
 * The Ora mark and the name "Ora Cloud", laid out side by side and centred as a group. The logo
 * starts centred alone and slides left as the name comes out from behind it, so the name's width
 * has to be measured once the font has loaded.
 */
export function Lockup({ state: s }: { state: LockupState }) {
  const [measure, textWidth] = useTextWidth()
  const row = LOGO_SIZE + GAP + textWidth
  const left = (WIDTH - row) / 2
  const top = HEIGHT / 2 + (s.y ?? 0) - LOGO_SIZE / 2
  const slide = ((GAP + textWidth) / 2) * (1 - s.shift)
  const logoScale = 0.96 + 0.04 * s.logo

  return <div style={{ position: 'absolute', left, top, width: row, height: LOGO_SIZE }}>
    <div ref={measure} style={{ position: 'absolute', visibility: 'hidden' }}><Name /></div>
    <div style={{
      position: 'absolute', left: LOGO_SIZE + GAP, top: 0, width: textWidth, height: LOGO_SIZE,
      display: 'flex', alignItems: 'center', clipPath: 'inset(-40% -40% -40% 0)',
    }}>
      <Name style={{ letterSpacing: `${s.tracking ?? 0}em` }} letter={i => {
        const p = s.letter(i)
        return {
          opacity: p, filter: p < 1 ? `blur(${(1 - p) * 18}px)` : undefined,
          transform: `translateX(${(1 - p) * -0.45}em)`,
        }
      }} />
    </div>
    <div style={{ position: 'absolute', left: 0, top: 0, width: LOGO_SIZE, height: LOGO_SIZE, transform: `translateX(${slide}px)` }}>
      <div style={{
        position: 'relative', borderRadius: '24.3%', opacity: s.logo,
        transform: `scale(${logoScale})`, filter: s.logo < 1 ? `blur(${(1 - s.logo) * 10}px)` : undefined,
        boxShadow: `0 ${36 * s.logo}px 100px rgba(40, 40, 90, ${0.18 * s.logo}), 0 10px 28px rgba(20, 20, 40, ${0.1 * s.logo})`,
      }}>
        <OraLogo size={LOGO_SIZE} blink={s.blink} />
      </div>
    </div>
  </div>
}

function Name({ style, letter }: { style?: React.CSSProperties; letter?: (index: number) => React.CSSProperties }) {
  return <div style={{
    fontFamily: '"Geist Variable", sans-serif', fontSize: FONT_SIZE, lineHeight: 1, whiteSpace: 'pre',
    letterSpacing: '0em', transform: 'translateY(-0.03em)', ...style,
  }}>
    {LETTERS.map((l, i) => <span key={i} style={{
      display: 'inline-block', fontWeight: l.weight, color: l.color, ...letter?.(i),
    }}>{l.ch}</span>)}
  </div>
}

/**
 * The name's settled width, read off a hidden copy once Geist has loaded; rendering waits until it
 * is known, so no frame lays the lockup out with a fallback font.
 */
function useTextWidth(): [React.RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  const [handle] = useState(() => delayRender('Measuring the lockup name'))
  useLayoutEffect(() => {
    let live = true
    void Promise.all(WORDS.map(w => document.fonts.load(`${w.weight} ${FONT_SIZE}px "Geist Variable"`))).then(() => {
      if (!live) return
      setWidth(ref.current!.getBoundingClientRect().width)
      continueRender(handle)
    })
    return () => { live = false }
  }, [handle])
  return [ref, width]
}
