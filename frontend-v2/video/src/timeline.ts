/**
 * The recording a shot leaves behind, and the pure math the composition runs on it: mapping output
 * time to recording time through speed ramps, the camera at any moment, and the cursor. Times in a
 * {@link Recording} are milliseconds since the screencast started; everything the composition asks
 * for is in output (video) milliseconds.
 */

/** A rectangle in viewport CSS pixels. */
export interface Box { x: number; y: number; width: number; height: number }

/** A point in viewport CSS pixels at recording time `t`. */
export interface Sample { t: number; x: number; y: number }

/** Where the camera should look from `t` on: a box to frame, or `null` for the whole page. `move` overrides how long the move takes. */
export interface CameraKey { t: number; box: Box | null; zoom?: number; move?: number }

/** A span of recording time played back `rate` times faster. */
export interface SpeedRamp { from: number; to: number; rate: number }

/** A moment of recording time the video stays frozen on for `ms` of output time. */
export interface Hold { t: number; ms: number }

/** Everything that decides how recording time maps to output time. */
export interface Timing { speed: SpeedRamp[]; holds: Hold[] }

/**
 * Text tied to a moment: `narration` is the subtitle and the script line, `caption` is the short
 * label shown when there is no narration. `offset` places the cue that many output milliseconds
 * into the hold at `t`; `padded` is how long the video had to freeze for the line to be read.
 */
export interface Cue { t: number; offset?: number; caption?: string; narration?: string; padded?: number }

/** One card of a slide or an explanation, appearing `at` output milliseconds into it. */
export interface PanelItem { title: string; body?: string; at: number }

/** A full-screen chapter page over a hold. */
export interface SlideOverlay {
  kind: 'slide'; t: number; ms: number; eyebrow?: string; title: string; subtitle?: string; items: PanelItem[]
}

/** The window steps aside over a hold, framing `box` (`null` for the whole page), while cards come in beside it. */
export interface ExplainOverlay {
  kind: 'explain'; t: number; ms: number; side: 'left' | 'right'; title?: string; items: PanelItem[]; box: Box | null; zoom?: number
}

/** A highlight around an element where it was at `t`, with an optional short label, for `ms` of output time. */
export interface MarkOverlay { kind: 'mark'; t: number; ms: number; box: Box; label?: string }

export type Overlay = SlideOverlay | ExplainOverlay | MarkOverlay

/** Everything the recorder writes to `out/<shot>/timeline.json`. */
export interface Recording {
  shot: string
  /** The browser viewport in CSS pixels, which boxes and samples use; frames are `scale` times larger. */
  viewport: { width: number; height: number; scale: number }
  duration: number
  /** Screencast frames, in time order; a frame holds until the next one. */
  frames: { t: number; file: string }[]
  cursor: Sample[]
  clicks: Sample[]
  camera: CameraKey[]
  speed: SpeedRamp[]
  holds: Hold[]
  cues: Cue[]
  overlays: Overlay[]
  urls: { t: number; url: string }[]
}

/**
 * Converts recording time to output time; ramps must be sorted and must not overlap. A moment that
 * holds maps to the start of its hold, and everything after it lands past the hold.
 */
export function toOutputTime(timing: Timing, t: number): number {
  let out = t
  for (const s of timing.speed) {
    if (t <= s.from) break
    const covered = Math.min(t, s.to) - s.from
    out -= covered - covered / s.rate
  }
  for (const h of timing.holds) if (h.t < t) out += h.ms
  return out
}

/** Converts output time back to recording time, the inverse of {@link toOutputTime}. Holds sit between ramps, never inside one. */
export function toRecordTime(timing: Timing, out: number): number {
  const steps = [
    ...timing.holds.map(h => ({ at: h.t, hold: h.ms, ramp: undefined })),
    ...timing.speed.map(s => ({ at: s.from, hold: 0, ramp: s })),
  ].sort((a, b) => a.at - b.at || b.hold - a.hold)
  let shift = 0
  for (const { at, hold, ramp } of steps) {
    const outFrom = at - shift
    if (out < outFrom) return out + shift
    if (ramp) {
      const outTo = outFrom + (ramp.to - ramp.from) / ramp.rate
      if (out < outTo) return ramp.from + (out - outFrom) * ramp.rate
      shift += (ramp.to - ramp.from) - (ramp.to - ramp.from) / ramp.rate
    } else {
      if (out < outFrom + hold) return at
      shift -= hold
    }
  }
  return out + shift
}

/** The ramp playing at output time `out`, if any; nothing plays while the video holds. */
export function rampAt(timing: Timing, out: number): SpeedRamp | undefined {
  const t = toRecordTime(timing, out)
  if (timing.holds.some(h => h.t === t)) return undefined
  return timing.speed.find(s => t >= s.from && t < s.to)
}

/** The cues on the output timeline, in order. */
export function cuesOnOutput(recording: Timing & { cues: Cue[] }): Cue[] {
  return recording.cues
    .map(c => ({ ...c, t: toOutputTime(recording, c.t) + (c.offset ?? 0) }))
    .sort((a, b) => a.t - b.t)
}

/** How fast a line of narration is read, in characters per second. */
export const READING_RATE = 4.5
const MIN_LINE = 1200

/** How long a line of narration needs on screen; punctuation does not count and a Latin word reads like two characters. */
export function narrationMs(text: string): number {
  const words = text.match(/[A-Za-z0-9]+/g)?.length ?? 0
  const chars = text.replace(/[A-Za-z0-9]+/g, '').replace(/[\s\p{P}\p{S}]/gu, '').length
  return Math.max(MIN_LINE, Math.round(((chars + words * 2) / READING_RATE) * 1000) + 300)
}

/** The last item at or before `t` in a list sorted by `t`, or `undefined` before the first. */
export function lastAt<T extends { t: number }>(items: T[], t: number): T | undefined {
  let lo = 0
  let hi = items.length - 1
  let found: T | undefined
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (items[mid].t <= t) { found = items[mid]; lo = mid + 1 } else hi = mid - 1
  }
  return found
}

/**
 * The video canvas and where the browser viewport sits on it, in canvas pixels. The window chrome
 * around the viewport is the composition's business; the camera only needs the viewport's rectangle.
 */
export interface Stage { width: number; height: number; content: Box }

/** A camera: the canvas point at the center of the frame, and the magnification. */
export interface View { cx: number; cy: number; scale: number }

/** How far the camera may push in, and how much margin it leaves around a framed box. */
export const MAX_ZOOM = 2
const FILL = 0.72

/** The view that frames `box` (viewport pixels) on the stage, or the whole page for `null`, pushed in to `zoom` if given. */
export function viewFor(stage: Stage, viewport: { width: number }, box: Box | null, zoom?: number): View {
  if (!box) return clampView(stage, { cx: stage.width / 2, cy: stage.content.y + stage.content.height / 2, scale: Math.max(1, Math.min(zoom ?? 1, MAX_ZOOM)) })
  const k = stage.content.width / viewport.width
  const w = box.width * k
  const h = box.height * k
  const fit = Math.min((stage.width * FILL) / w, (stage.height * FILL) / h)
  const scale = Math.max(1, Math.min(zoom ?? fit, MAX_ZOOM))
  return clampView(stage, {
    cx: stage.content.x + (box.x + box.width / 2) * k,
    cy: stage.content.y + (box.y + box.height / 2) * k,
    scale,
  })
}

/** Keeps the frame inside the stage, so pushing in near an edge never reveals what lies past it. */
export function clampView(stage: Stage, view: View): View {
  const halfW = stage.width / (2 * view.scale)
  const halfH = stage.height / (2 * view.scale)
  return {
    scale: view.scale,
    cx: Math.min(Math.max(view.cx, halfW), stage.width - halfW),
    cy: Math.min(Math.max(view.cy, halfH), stage.height - halfH),
  }
}

/** How long a camera move takes, in output milliseconds. */
export const CAMERA_MOVE = 1000

/** How long a pan that follows an action takes: quick, because the zoom stays put. */
export const PAN_MOVE = 600

/** How much of the frame's edge counts as out of sight when deciding whether an action needs a pan. */
const PAN_MARGIN = 0.15

/**
 * Whether the camera has to pan for a point (viewport pixels) to be comfortably in frame. Actions
 * use this so that quick work on a page leaves the camera still whenever it can.
 */
export function needsPan(stage: Stage, viewport: { width: number }, view: View, point: { x: number; y: number }): boolean {
  if (view.scale <= 1) return false
  const k = stage.content.width / viewport.width
  const halfW = (stage.width / (2 * view.scale)) * (1 - 2 * PAN_MARGIN)
  const halfH = (stage.height / (2 * view.scale)) * (1 - 2 * PAN_MARGIN)
  return Math.abs(stage.content.x + point.x * k - view.cx) > halfW || Math.abs(stage.content.y + point.y * k - view.cy) > halfH
}

/** Zoom changes closer together than this make viewers dizzy. */
const ZOOM_GAP = 3000

/** The output times of zoom changes that come too soon after the previous one. */
export function restlessZooms(keys: { t: number; view: View }[], initial: View): number[] {
  const restless: number[] = []
  let scale = initial.scale
  let changed = -Infinity
  for (const k of keys) {
    if (Math.abs(k.view.scale - scale) < 0.01) continue
    if (k.t - changed < ZOOM_GAP) restless.push(k.t)
    scale = k.view.scale
    changed = k.t
  }
  return restless
}

function easeInOutCubic(x: number): number {
  return x < 0.5 ? 4 * x * x * x : 1 - (-2 * x + 2) ** 3 / 2
}

/** The view `p` of the way from `a` to `b`. */
export function blend(a: View, b: View, p: number): View {
  // Zoom interpolates in log space, so pushing in from 1x to 2x feels as even as 2x to 4x.
  return {
    cx: a.cx + (b.cx - a.cx) * p,
    cy: a.cy + (b.cy - a.cy) * p,
    scale: Math.exp(Math.log(a.scale) + (Math.log(b.scale) - Math.log(a.scale)) * p),
  }
}

/**
 * The camera at output time `t`, given keys already converted to views and output time. A new key
 * starts its move from wherever the previous move had got to, so interrupted moves never jump.
 */
export function cameraAt(keys: { t: number; view: View; move?: number }[], t: number, initial: View): View {
  let from = initial
  let target = initial
  let start = -Infinity
  let move = CAMERA_MOVE
  const progress = (dt: number) => easeInOutCubic(Math.min(Math.max(dt / move, 0), 1))
  for (const k of keys) {
    if (k.t > t) break
    from = blend(from, target, progress(k.t - start))
    target = k.view
    start = k.t
    move = k.move ?? CAMERA_MOVE
  }
  return blend(from, target, progress(t - start))
}

/** The cursor at recording time `t`, interpolated between samples; `undefined` before the first. */
export function cursorAt(samples: Sample[], t: number): { x: number; y: number } | undefined {
  const before = lastAt(samples, t)
  if (!before) return undefined
  const after = samples[samples.indexOf(before) + 1]
  if (!after || after.t === before.t) return { x: before.x, y: before.y }
  const p = Math.min((t - before.t) / (after.t - before.t), 1)
  return { x: before.x + (after.x - before.x) * p, y: before.y + (after.y - before.y) * p }
}

/** Formats output milliseconds as `mm:ss.s`, the timecode the transcript uses. */
export function timecode(ms: number): string {
  const total = Math.max(0, ms) / 1000
  const minutes = Math.floor(total / 60)
  const seconds = total - minutes * 60
  return `${String(minutes).padStart(2, '0')}:${seconds.toFixed(1).padStart(4, '0')}`
}
