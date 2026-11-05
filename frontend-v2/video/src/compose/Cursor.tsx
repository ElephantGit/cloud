/** How long a click ripple lasts, in output milliseconds. */
export const RIPPLE_MS = 600

/** The synthesized pointer, drawn at the canvas point (x, y) where its tip is. */
export function Cursor({ x, y, pressed }: { x: number; y: number; pressed: boolean }) {
  const size = 64
  return <svg width={size} height={size} viewBox="0 0 24 24" style={{
    position: 'absolute', left: x - 5, top: y - 3, overflow: 'visible',
    transform: `scale(${pressed ? 0.86 : 1})`, transformOrigin: '5px 3px',
    filter: 'drop-shadow(0 4px 6px rgba(0, 0, 0, 0.35))',
  }}>
    <path d="M5 3 L5 19.5 L9.4 15.4 L12.4 22 L15.1 20.8 L12.2 14.4 L18.3 14.4 Z"
      fill="#111827" stroke="#fff" strokeWidth={1.6} strokeLinejoin="round" />
  </svg>
}

/** A ring that grows and fades from a click point; `age` is milliseconds since the click. */
export function Ripple({ x, y, age }: { x: number; y: number; age: number }) {
  const p = Math.min(age / RIPPLE_MS, 1)
  const r = 18 + p * 70
  return <div style={{
    position: 'absolute', left: x - r, top: y - r, width: r * 2, height: r * 2, borderRadius: '50%',
    border: `${6 * (1 - p) + 2}px solid rgba(59, 130, 246, ${0.75 * (1 - p)})`,
    background: `rgba(59, 130, 246, ${0.18 * (1 - p)})`,
  }} />
}
