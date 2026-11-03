import { describe, expect, it } from 'vitest'
import type { Recording } from '../timeline.ts'
import { lines, markdown, srt } from './transcript.ts'

const recording = {
  duration: 10_000,
  speed: [{ from: 2000, to: 6000, rate: 4 }],
  cues: [
    { t: 0, caption: '陈默 · Tech Lead', narration: '开场。' },
    { t: 2000, caption: '拆分', narration: '工作流在跑。' },
    { t: 6000, caption: '完成' },
    { t: 9000, caption: '' },
  ],
} as unknown as Recording

describe('transcript', () => {
  it('places every cue on the output timeline and drops caption-clearing cues', () => {
    expect(lines(recording)).toEqual([
      { start: 0, end: 2000, caption: '陈默 · Tech Lead', narration: '开场。' },
      { start: 2000, end: 3000, caption: '拆分', narration: '工作流在跑。' },
      { start: 3000, end: 6000, caption: '完成', narration: undefined },
    ])
  })

  it('renders a table and subtitles of the narration', () => {
    const script = lines(recording)
    expect(markdown('lead', script)).toContain('| 00:02.0 – 00:03.0 | 拆分 | 工作流在跑。 |')
    expect(srt(script)).toBe('1\n00:00:00,000 --> 00:00:02,000\n开场。\n\n2\n00:00:02,000 --> 00:00:03,000\n工作流在跑。\n')
  })
})
