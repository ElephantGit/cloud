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

/** Where the camera should look from `t` on: a box to frame, or `null` for the whole page. */
export interface CameraKey { t: number; box: Box | null; zoom?: number }

/** A span of recording time played back `rate` times faster. */
export interface SpeedRamp { from: number; to: number; rate: number }

/** Text tied to a moment: `caption` is shown on screen, `narration` only goes to the transcript. */
export interface Cue { t: number; caption?: string; narration?: string }

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
  cues: Cue[]
  urls: { t: number; url: string }[]
}

/** Converts recording time to output time; ramps must be sorted and must not overlap. */
export function toOutputTime(speed: SpeedRamp[], t: number): number {
  let out = t
  for (const s of speed) {
    if (t <= s.from) break
    const covered = Math.min(t, s.to) - s.from
    out -= covered - covered / s.rate
  }
  return out
}

/** Converts output time back to recording time, the inverse of {@link toOutputTime}. */
export function toRecordTime(speed: SpeedRamp[], out: number): number {
  let shift = 0
  for (const s of speed) {
    const outFrom = s.from - shift
    if (out < outFrom) return out + shift
    const outTo = outFrom + (s.to - s.from) / s.rate
    if (out < outTo) return s.from + (out - outFrom) * s.rate
    shift += (s.to - s.from) - (s.to - s.from) / s.rate
  }
  return out + shift
}

/** The ramp playing at output time `out`, if any. */
export function rampAt(speed: SpeedRamp[], out: number): SpeedRamp | undefined {
  const t = toRecordTime(speed, out)
  return speed.find(s => t >= s.from && t < s.to)
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

/** The view that frames `box` (viewport pixels) on the stage, or the whole stage for `null`. */
export function viewFor(stage: Stage, viewport: { width: number }, box: Box | null, zoom?: number): View {
  if (!box) return { cx: stage.width / 2, cy: stage.height / 2, scale: 1 }
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

function easeInOutCubic(x: number): number {
  return x < 0.5 ? 4 * x * x * x : 1 - (-2 * x + 2) ** 3 / 2
}

function blend(a: View, b: View, p: number): View {
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
export function cameraAt(keys: { t: number; view: View }[], t: number, initial: View): View {
  let from = initial
  let target = initial
  let start = -Infinity
  const progress = (dt: number) => easeInOutCubic(Math.min(Math.max(dt / CAMERA_MOVE, 0), 1))
  for (const k of keys) {
    if (k.t > t) break
    from = blend(from, target, progress(k.t - start))
    target = k.view
    start = k.t
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
