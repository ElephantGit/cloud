import { AbsoluteFill, Easing, interpolate } from 'remotion'
import { PANEL_ENTER, PANEL_EXIT } from '../record/timing.ts'
import type { ExplainOverlay, MarkOverlay, PanelItem, SlideOverlay } from '../timeline.ts'
import { Grain } from './brand/Grain.tsx'
import { Network } from './brand/Network.tsx'
import { FPS } from './layout.ts'

const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const
const ease = Easing.bezier(0.16, 1, 0.3, 1)
const FONT = '"Geist Variable", "Noto Sans SC Variable", system-ui, sans-serif'
const INK = '#15151d'
const INDIGO = '#4a4c86'
const MUTED = '#5b6375'

/** How far a panel has come in, from 0 to 1, `age` output milliseconds into its `ms`. */
export function panelProgress(age: number, ms: number): number {
  const enter = interpolate(age, [0, PANEL_ENTER], [0, 1], { ...clamp, easing: ease })
  return enter * interpolate(age, [ms - PANEL_EXIT, ms], [1, 0], { ...clamp, easing: Easing.bezier(0.65, 0, 0.35, 1) })
}

function Card({ item, age, index, big }: { item: PanelItem; age: number; index: number; big?: boolean }) {
  const p = interpolate(age, [item.at, item.at + 500], [0, 1], { ...clamp, easing: ease })
  return <div style={{ opacity: p, transform: `translateY(${(1 - p) * 28}px)`, display: 'flex', gap: 40, alignItems: 'flex-start' }}>
    <div style={{ fontSize: big ? 44 : 38, fontWeight: 500, color: INDIGO, lineHeight: big ? '84px' : '72px', fontVariantNumeric: 'tabular-nums' }}>
      {String(index + 1).padStart(2, '0')}
    </div>
    <div style={{ flex: 1, borderTop: `2px solid rgba(74, 76, 134, ${0.28 * p})`, paddingTop: 22 }}>
      <div style={{ fontSize: big ? 68 : 58, fontWeight: 600, color: INK, lineHeight: 1.3 }}>{item.title}</div>
      {item.body && <div style={{ fontSize: big ? 46 : 42, color: MUTED, lineHeight: 1.5, marginTop: 14 }}>{item.body}</div>}
    </div>
  </div>
}

/** A full-screen chapter page in the opening's language: pale ground, dot grid, thin indigo lines. */
export function SlidePanel({ overlay, age, opaqueEntry }: { overlay: SlideOverlay; age: number; opaqueEntry: boolean }) {
  const p = panelProgress(age, overlay.ms)
  const leaving = age > overlay.ms / 2
  const text = interpolate(age, [120, 820], [0, 1], { ...clamp, easing: ease })
  const frame = Math.round((age / 1000) * FPS)
  return <AbsoluteFill style={{ opacity: opaqueEntry && !leaving ? 1 : p, fontFamily: FONT }}>
    <AbsoluteFill style={{ background: '#f5f6fa' }} />
    <AbsoluteFill style={{ opacity: 0.45 }}><Network t={age / 1000} ground={1} build={1} recede={1} /></AbsoluteFill>
    <AbsoluteFill style={{
      padding: overlay.items.length ? '0 360px' : '0 480px', justifyContent: 'center',
      opacity: text, transform: `translateY(${(1 - text) * 30}px)`,
    }}>
      {overlay.eyebrow && <div style={{ fontSize: 50, fontWeight: 500, color: INDIGO, letterSpacing: 4, marginBottom: 40 }}>{overlay.eyebrow}</div>}
      <div style={{ fontSize: overlay.title.length > 18 ? 112 : 148, fontWeight: 650, color: INK, lineHeight: 1.25 }}>{overlay.title}</div>
      {overlay.subtitle && <div style={{ fontSize: 62, color: MUTED, lineHeight: 1.45, marginTop: 44 }}>{overlay.subtitle}</div>}
      {overlay.items.length > 0 && <div style={{ display: 'flex', flexDirection: 'column', gap: 44, marginTop: 90 }}>
        {overlay.items.map((item, i) => <Card key={i} item={item} age={age} index={i} big />)}
      </div>}
    </AbsoluteFill>
    <Grain frame={frame} />
  </AbsoluteFill>
}

/** Where the window goes while it steps aside: this much smaller, centred this far from the panel's side. */
export const ASIDE_SCALE = 0.6
export const ASIDE_CENTER = 0.329

/** The cards beside the window. */
export function ExplainPanel({ overlay, age }: { overlay: ExplainOverlay; age: number }) {
  const p = panelProgress(age, overlay.ms)
  const onRight = overlay.side === 'right'
  return <div style={{
    position: 'absolute', top: 0, bottom: 0, width: '31%', [onRight ? 'right' : 'left']: 130,
    display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: 46, fontFamily: FONT,
    opacity: p, transform: `translateX(${(1 - p) * (onRight ? 60 : -60)}px)`,
  }}>
    {overlay.title && <div style={{ fontSize: 76, fontWeight: 650, color: INK, lineHeight: 1.3, marginBottom: 14 }}>{overlay.title}</div>}
    {overlay.items.map((item, i) => <Card key={i} item={item} age={age} index={i} />)}
  </div>
}

/** A highlight drawn in canvas space, so it stays on its element as the camera moves. */
export function MarkBox({ overlay, age, k, origin }: { overlay: MarkOverlay; age: number; k: number; origin: { x: number; y: number } }) {
  const p = interpolate(age, [0, 320], [0, 1], { ...clamp, easing: ease }) * interpolate(age, [overlay.ms - 320, overlay.ms], [1, 0], clamp)
  const pad = 14
  const { box } = overlay
  return <div style={{
    position: 'absolute', left: origin.x + box.x * k - pad, top: origin.y + box.y * k - pad,
    width: box.width * k + pad * 2, height: box.height * k + pad * 2, borderRadius: 20, opacity: p,
    border: `6px solid ${INDIGO}`, boxShadow: '0 0 0 10px rgba(74, 76, 134, 0.14)', transform: `scale(${1.03 - 0.03 * p})`,
  }}>
    {overlay.label && <div style={{
      position: 'absolute', right: -6, bottom: '100%', marginBottom: 12, padding: '10px 24px', borderRadius: 12, whiteSpace: 'nowrap',
      background: INDIGO, color: '#fff', fontSize: 36, fontWeight: 600, fontFamily: FONT,
    }}>{overlay.label}</div>}
  </div>
}
