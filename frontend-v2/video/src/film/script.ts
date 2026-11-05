import { FPS, INTRO_FRAMES, OUTRO_FRAMES } from '../compose/layout.ts'
import { lines, type Line } from '../record/transcript.ts'
import { toOutputTime, type Recording } from '../timeline.ts'

/** The film's order: the opening, every shot in the order `shots/index.ts` registers them, the closing. */
export function filmOrder(shots: string[]): string[] {
  return ['intro', ...shots, 'outro']
}

/** How long a rendered part lasts: compositions round up to whole frames. */
function partMs(frames: number): number {
  return (frames / FPS) * 1000
}

/** A shot's length in the video, as its render rounds it. */
export function shotMs(recording: Recording): number {
  return partMs(Math.ceil((toOutputTime(recording, recording.duration) / 1000) * FPS))
}

/** The whole film's script: each shot's lines moved to where the shot starts in the film. */
export function filmScript(order: string[], recordings: Record<string, Recording>): Line[] {
  let at = 0
  const script: Line[] = []
  for (const part of order) {
    if (part === 'intro') { at += partMs(INTRO_FRAMES); continue }
    if (part === 'outro') { at += partMs(OUTRO_FRAMES); continue }
    const recording = recordings[part]
    if (!recording) throw new Error(`No recording for ${part}.`)
    for (const l of lines(recording)) script.push({ ...l, start: l.start + at, end: l.end + at })
    at += shotMs(recording)
  }
  return script
}
