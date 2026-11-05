import { describe, expect, it } from 'vitest'
import type { Recording } from '../timeline.ts'
import { filmOrder, filmScript } from './script.ts'

const shot = (narration: string, duration: number) => ({
  duration, speed: [], holds: [], cues: [{ t: 0, narration }],
}) as unknown as Recording

describe('filmScript', () => {
  it('starts after the opening and moves each shot past the ones before it', () => {
    const script = filmScript(filmOrder(['a', 'b']), { a: shot('甲', 1000), b: shot('乙', 2000) })
    expect(script.map(l => [l.narration, l.start, l.end])).toEqual([['甲', 6000, 7000], ['乙', 7000, 9000]])
  })

  it('names the shot it has no recording for', () => {
    expect(() => filmScript(filmOrder(['a']), {})).toThrow('No recording for a.')
  })
})
