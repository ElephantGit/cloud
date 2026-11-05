import type { Page } from 'playwright'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { narrationMs, toOutputTime } from '../timeline.ts'
import { Director } from './director.ts'
import { PANEL_ENTER, PANEL_EXIT } from './timing.ts'

const viewport = { width: 1280, height: 720 }
const LINE = '一二三四五六七八九'

describe('Director', () => {
  let d: Director
  beforeEach(() => {
    vi.useFakeTimers({ now: 0 })
    d = new Director({} as Page, 0, viewport, 'http://x')
  })
  afterEach(() => { vi.useRealTimers() })

  it('freezes the video when the picture runs out before the line is read', () => {
    d.say({ narration: LINE })
    vi.setSystemTime(500)
    d.say({ narration: '下一句' })
    const need = narrationMs(LINE)
    expect(d.holds).toEqual([{ t: 500, ms: need - 500 }])
    expect(d.cues[0]!.padded).toBe(need - 500)
    expect(toOutputTime(d, d.cues[1]!.t)).toBeGreaterThanOrEqual(need)
  })

  it('leaves a line alone when the picture is long enough', () => {
    d.say({ narration: LINE })
    vi.setSystemTime(10_000)
    d.say({ caption: '' })
    expect(d.holds).toEqual([])
    expect(d.cues[0]!.padded).toBeUndefined()
  })

  it('counts a sped-up wait at its speed, so the line still gets its time', () => {
    d.say({ narration: LINE })
    d.speed.push({ from: 0, to: 2000, rate: 10 })
    vi.setSystemTime(2000)
    d.finish()
    expect(d.holds[0]!.ms).toBe(narrationMs(LINE) - 200)
  })

  it('sizes a slide from its narration and brings each card in with its line', () => {
    d.slide({ title: '章节', narration: LINE, items: [{ title: '第一条', narration: LINE }, { title: '第二条', narration: LINE }] })
    const overlay = d.overlays[0]!
    if (overlay.kind !== 'slide') throw new Error('expected a slide')
    expect(overlay.items.map(i => i.at)).toEqual([PANEL_ENTER + narrationMs(LINE), PANEL_ENTER + 2 * narrationMs(LINE)])
    expect(d.holds).toEqual([{ t: 0, ms: overlay.ms }])
    expect(overlay.ms).toBeGreaterThan(PANEL_ENTER + 3 * narrationMs(LINE) + PANEL_EXIT - 1)
  })

  it('clears the subtitle before a panel leaves and continues after its hold', () => {
    d.slide({ title: '章节', narration: LINE })
    const clear = d.cues.at(-1)!
    expect(clear.caption).toBe('')
    expect(clear.offset).toBe(d.holds[0]!.ms - PANEL_EXIT)
    d.say({ narration: '继续' })
    expect(toOutputTime(d, d.cues.at(-1)!.t)).toBeGreaterThan(d.holds[0]!.ms)
  })
})
