import { timecode, toOutputTime, type Recording } from '../timeline.ts'

/** One line of the voice-over script, at its output (video) time. */
export interface Line { start: number; end: number; caption?: string; narration?: string }

/**
 * The script lines of a recording on the video's timeline: each cue lasts until the next one, the
 * last until the end. Cues that only clear the caption carry nothing to read and are dropped.
 */
export function lines(recording: Recording): Line[] {
  const end = toOutputTime(recording.speed, recording.duration)
  const timed = recording.cues.map(c => ({ ...c, at: toOutputTime(recording.speed, c.t) }))
  return timed
    .map((c, i) => ({ start: c.at, end: timed[i + 1]?.at ?? end, caption: c.caption || undefined, narration: c.narration }))
    .filter(l => l.caption || l.narration)
}

/** The transcript the voice-over artist reads from: timecode, on-screen text, and narration. */
export function markdown(shot: string, script: Line[]): string {
  const rows = script.map(l => `| ${timecode(l.start)} – ${timecode(l.end)} | ${l.caption ?? ''} | ${l.narration ?? ''} |`)
  return [`# ${shot} 文字稿`, '', '| 时间 | 画面文字 | 旁白 |', '|---|---|---|', ...rows, ''].join('\n')
}

/** The same lines as SubRip subtitles, for lining the narration up in an editor. */
export function srt(script: Line[]): string {
  const stamp = (ms: number) => {
    const total = Math.round(ms)
    const h = Math.floor(total / 3_600_000)
    const m = Math.floor(total / 60_000) % 60
    const s = Math.floor(total / 1000) % 60
    const pad = (n: number, w = 2) => String(n).padStart(w, '0')
    return `${pad(h)}:${pad(m)}:${pad(s)},${pad(total % 1000, 3)}`
  }
  return script
    .filter(l => l.narration)
    .map((l, i) => `${i + 1}\n${stamp(l.start)} --> ${stamp(l.end)}\n${l.narration}\n`)
    .join('\n')
}
