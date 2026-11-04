import { AbsoluteFill, interpolate } from 'remotion'
import { HEIGHT, WIDTH } from '../layout.ts'
import { seeded } from './seeded.ts'

const PITCH = 80
const COLS = WIDTH / PITCH
const ROWS = HEIGHT / PITCH

/** Grid points standing for people and agents, kept clear of where the lockup will sit. */
const NODES = (() => {
  const random = seeded(11)
  const nodes: { x: number; y: number; at: number }[] = []
  while (nodes.length < 16) {
    const x = (2 + Math.floor(random() * (COLS - 4))) * PITCH
    const y = (2 + Math.floor(random() * (ROWS - 4))) * PITCH
    const clearOfLockup = x < 900 || x > 2940 || y < 760 || y > 1400
    const apart = nodes.every(n => Math.hypot(n.x - x, n.y - y) > 420)
    if (clearOfLockup && apart) nodes.push({ x, y, at: 0 })
  }
  // Light them from the outside in, so the network gathers towards the centre.
  return nodes
    .map(n => ({ ...n, d: Math.hypot(n.x - WIDTH / 2, n.y - HEIGHT / 2) }))
    .sort((a, b) => b.d - a.d)
    .map((n, i) => ({ x: n.x, y: n.y, at: i / 15 }))
})()

/** Each node joins its two nearest neighbours; an edge draws once both of its ends are lit. */
const EDGES = (() => {
  const seen = new Set<string>()
  const edges: { a: number; b: number; at: number }[] = []
  NODES.forEach((n, i) => {
    NODES.map((m, j) => ({ j, d: Math.hypot(m.x - n.x, m.y - n.y) }))
      .filter(o => o.j !== i).sort((p, q) => p.d - q.d).slice(0, 2)
      .forEach(({ j }) => {
        const key = [Math.min(i, j), Math.max(i, j)].join('-')
        if (seen.has(key)) return
        seen.add(key)
        edges.push({ a: i, b: j, at: Math.max(NODES[i].at, NODES[j].at) })
      })
  })
  return edges
})()

/**
 * The opening's backdrop: a plain grey ground with two faint washes of the brand hue, a dot grid
 * like squared paper, and a sparse network that lights up node by node and links up, a quiet image
 * of people and agents joining one team. `build` (0–1) drives the network, `recede` (0–1) fades it
 * back once the logo takes the stage.
 */
export function Network({ t, ground, build, recede }: { t: number; ground: number; build: number; recede: number }) {
  const lineAlpha = 0.34 - 0.2 * recede
  const grid = 'radial-gradient(ellipse 70% 70% at 50% 50%, black 30%, transparent 100%)'
  return <AbsoluteFill style={{ background: '#f4f4f7', overflow: 'hidden' }}>
    <Wash x={0.22 + 0.04 * Math.sin(t * 0.35)} y={0.3} rgb="112, 108, 220" alpha={0.13 * ground} />
    <Wash x={0.8 - 0.04 * Math.sin(t * 0.3)} y={0.74} rgb="96, 150, 230" alpha={0.1 * ground} />
    <AbsoluteFill style={{
      backgroundImage: 'radial-gradient(circle, rgba(52, 52, 84, 0.16) 2.2px, transparent 2.8px)',
      backgroundSize: `${PITCH}px ${PITCH}px`, backgroundPosition: `${PITCH / 2}px ${PITCH / 2}px`,
      maskImage: grid, WebkitMaskImage: grid, opacity: ground,
    }} />
    <svg width={WIDTH} height={HEIGHT} style={{ position: 'absolute', inset: 0 }}>
      {EDGES.map((e, i) => {
        const p = interpolate(build, [e.at * 0.8 + 0.08, e.at * 0.8 + 0.2], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' })
        if (p <= 0) return null
        const a = NODES[e.a]
        const b = NODES[e.b]
        return <line key={i} x1={a.x} y1={a.y} x2={a.x + (b.x - a.x) * p} y2={a.y + (b.y - a.y) * p}
          stroke={`rgba(70, 68, 140, ${lineAlpha})`} strokeWidth={2.5} strokeLinecap="round" />
      })}
      {NODES.map((n, i) => {
        const p = interpolate(build, [n.at * 0.8, n.at * 0.8 + 0.08], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' })
        if (p <= 0) return null
        return <g key={i} opacity={p * (1 - 0.5 * recede)}>
          <circle cx={n.x} cy={n.y} r={22 + 10 * (1 - p)} fill="none" stroke="rgba(70, 68, 140, 0.28)" strokeWidth={2.5} />
          <circle cx={n.x} cy={n.y} r={8} fill="rgba(70, 68, 140, 0.6)" />
        </g>
      })}
    </svg>
  </AbsoluteFill>
}

function Wash({ x, y, rgb, alpha }: { x: number; y: number; rgb: string; alpha: number }) {
  const size = 2600
  return <div style={{
    position: 'absolute', left: x * WIDTH - size / 2, top: y * HEIGHT - size / 2, width: size, height: size, borderRadius: '50%',
    background: `radial-gradient(circle, rgba(${rgb}, ${alpha}) 0%, rgba(${rgb}, ${alpha * 0.4}) 40%, rgba(${rgb}, 0) 70%)`,
  }} />
}
