import { cuesOnOutput, timecode, toOutputTime, type Recording } from '../timeline.ts'

/** One line of the voice-over script, at its output (video) time. */
export interface Line { start: number; end: number; caption?: string; narration?: string; padded?: number }

/**
 * The script lines of a recording on the video's timeline: each cue lasts until the next one, the
 * last until the end. Cues that only clear the caption carry nothing to read and are dropped.
 */
export function lines(recording: Recording): Line[] {
  const end = toOutputTime(recording, recording.duration)
  const timed = cuesOnOutput(recording)
  return timed
    .map((c, i) => ({ start: c.t, end: timed[i + 1]?.t ?? end, caption: c.caption || undefined, narration: c.narration, padded: c.padded }))
    .filter(l => l.caption || l.narration)
}

/** The transcript: timecode, on-screen text, narration, and how long a line outran its picture. */
export function markdown(shot: string, script: Line[]): string {
  const note = (l: Line) => (l.padded ? `画面不够，定格补了 ${(l.padded / 1000).toFixed(1)} 秒` : '')
  const rows = script.map(l => `| ${timecode(l.start)} – ${timecode(l.end)} | ${l.caption ?? ''} | ${l.narration ?? ''} | ${note(l)} |`)
  return [`# ${shot} 文字稿`, '', '| 时间 | 画面文字 | 旁白 | 节奏 |', '|---|---|---|---|', ...rows, ''].join('\n')
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
