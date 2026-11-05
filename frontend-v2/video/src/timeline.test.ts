import { describe, expect, it } from 'vitest'
import {
  CAMERA_MOVE, cameraAt, clampView, cuesOnOutput, cursorAt, lastAt, MAX_ZOOM, narrationMs, needsPan, rampAt, restlessZooms,
  timecode, toOutputTime, toRecordTime, viewFor,
  type Stage,
} from './timeline.ts'

const ramps = { speed: [{ from: 1000, to: 3000, rate: 4 }, { from: 5000, to: 6000, rate: 2 }], holds: [] }

describe('speed ramps', () => {
  it('compress only the ramped spans', () => {
    expect(toOutputTime(ramps, 500)).toBe(500)
    expect(toOutputTime(ramps, 2000)).toBe(1250)
    expect(toOutputTime(ramps, 3000)).toBe(1500)
    expect(toOutputTime(ramps, 4000)).toBe(2500)
    expect(toOutputTime(ramps, 6000)).toBe(4000)
    expect(toOutputTime(ramps, 7000)).toBe(5000)
  })

  it('invert exactly', () => {
    for (const t of [0, 999, 1000, 1700, 3000, 4321, 5000, 5500, 6000, 9000]) {
      expect(toRecordTime(ramps, toOutputTime(ramps, t))).toBeCloseTo(t)
    }
  })

  it('report the ramp playing at an output time', () => {
    expect(rampAt(ramps, 1200)?.rate).toBe(4)
    expect(rampAt(ramps, 2000)).toBeUndefined()
    expect(rampAt(ramps, 3600)?.rate).toBe(2)
  })
})

describe('holds', () => {
  const timing = { speed: [{ from: 1000, to: 3000, rate: 4 }], holds: [{ t: 1000, ms: 2000 }, { t: 4000, ms: 500 }] }

  it('insert output time after the moment that holds', () => {
    expect(toOutputTime(timing, 1000)).toBe(1000)
    expect(toOutputTime(timing, 2000)).toBe(3250)
    expect(toOutputTime(timing, 4000)).toBe(4500)
    expect(toOutputTime(timing, 4001)).toBe(5001)
  })

  it('freeze recording time while they last', () => {
    expect(toRecordTime(timing, 1000)).toBe(1000)
    expect(toRecordTime(timing, 2999)).toBe(1000)
    expect(toRecordTime(timing, 4600)).toBe(4000)
  })

  it('invert exactly around them', () => {
    for (const t of [0, 999, 1500, 3000, 3999, 4001, 9000]) expect(toRecordTime(timing, toOutputTime(timing, t))).toBeCloseTo(t)
  })

  it('hide the speed badge until the ramp plays', () => {
    expect(rampAt(timing, 2000)).toBeUndefined()
    expect(rampAt(timing, 3100)?.rate).toBe(4)
  })

  it('place cues by their offset into the hold', () => {
    const cues = cuesOnOutput({ ...timing, cues: [{ t: 1000, offset: 800, narration: 'b' }, { t: 1000, narration: 'a' }] })
    expect(cues.map(c => [c.t, c.narration])).toEqual([[1000, 'a'], [1800, 'b']])
  })
})

describe('narrationMs', () => {
  it('reads about 4.5 characters a second, ignoring punctuation', () => {
    expect(narrationMs('一二三四五六七八九。')).toBe(2300)
    expect(narrationMs('一二三四五六七八九')).toBe(2300)
  })

  it('counts a Latin word as two characters and never goes below the minimum', () => {
    expect(narrationMs('好')).toBe(1200)
    expect(narrationMs('一二三四五 Agent 六七')).toBe(narrationMs('一二三四五六七八九'))
  })
})

describe('lastAt', () => {
  const items = [{ t: 0 }, { t: 10 }, { t: 20 }]
  it('finds the item in effect', () => {
    expect(lastAt(items, -1)).toBeUndefined()
    expect(lastAt(items, 0)).toBe(items[0])
    expect(lastAt(items, 15)).toBe(items[1])
    expect(lastAt(items, 99)).toBe(items[2])
  })
})

const stage: Stage = { width: 3840, height: 2160, content: { x: 240, y: 200, width: 3200, height: 1800 } }
const viewport = { width: 1600 }

describe('camera', () => {
  it('shows the whole stage for the page', () => {
    expect(viewFor(stage, viewport, null)).toEqual({ cx: 1920, cy: 1080, scale: 1 })
  })

  it('pushes in on the middle of the page when given a zoom', () => {
    expect(viewFor(stage, viewport, null, 1.5)).toEqual({ cx: 1920, cy: 1100, scale: 1.5 })
  })

  it('pushes in on a box, capped at the maximum zoom', () => {
    const view = viewFor(stage, viewport, { x: 700, y: 400, width: 100, height: 40 })
    expect(view.scale).toBe(MAX_ZOOM)
    expect(view.cx).toBeCloseTo(240 + 750 * 2)
    expect(view.cy).toBeCloseTo(200 + 420 * 2)
  })

  it('never zooms out past the whole stage', () => {
    expect(viewFor(stage, viewport, { x: 0, y: 0, width: 1600, height: 900 }).scale).toBe(1)
  })

  it('keeps the frame inside the stage near an edge', () => {
    const view = clampView(stage, { cx: 0, cy: 2160, scale: 2 })
    expect(view).toEqual({ cx: 960, cy: 1620, scale: 2 })
  })

  it('eases between keys and starts an interrupted move where it was', () => {
    const initial = { cx: 1920, cy: 1080, scale: 1 }
    const a = { cx: 1000, cy: 1000, scale: 2 }
    const b = { cx: 2000, cy: 1000, scale: 2 }
    const keys = [{ t: 0, view: a }, { t: CAMERA_MOVE / 2, view: b }]
    expect(cameraAt([], 500, initial)).toEqual(initial)
    const halfway = cameraAt(keys.slice(0, 1), CAMERA_MOVE / 2, initial)
    expect(halfway.cx).toBeCloseTo((1920 + 1000) / 2)
    // The second key interrupts the first halfway; its move begins from that halfway view.
    expect(cameraAt(keys, CAMERA_MOVE / 2, initial).cx).toBeCloseTo(halfway.cx)
    expect(cameraAt(keys, CAMERA_MOVE * 2, initial)).toEqual(b)
  })
})

describe('panning', () => {
  const pushed = { cx: 1920, cy: 1080, scale: 2 }
  it('leaves the camera still for a point comfortably in frame', () => {
    expect(needsPan(stage, viewport, pushed, { x: 800, y: 450 })).toBe(false)
  })

  it('pans for a point near or past the edge of the frame', () => {
    expect(needsPan(stage, viewport, pushed, { x: 1400, y: 450 })).toBe(true)
    expect(needsPan(stage, viewport, pushed, { x: 800, y: 850 })).toBe(true)
  })

  it('never needs to pan with the whole page in frame', () => {
    expect(needsPan(stage, viewport, { ...pushed, scale: 1 }, { x: 1590, y: 890 })).toBe(false)
  })
})

describe('restlessZooms', () => {
  const at = (t: number, scale: number) => ({ t, view: { cx: 0, cy: 0, scale } })
  const initial = { cx: 0, cy: 0, scale: 1 }
  it('flags a zoom change right after another, and ignores pans', () => {
    expect(restlessZooms([at(0, 1.35), at(1000, 1.35), at(2000, 1), at(6000, 1.35)], initial)).toEqual([2000])
  })
})

describe('cursorAt', () => {
  const samples = [{ t: 0, x: 0, y: 0 }, { t: 100, x: 100, y: 50 }]
  it('interpolates between samples and rests after the last', () => {
    expect(cursorAt(samples, -1)).toBeUndefined()
    expect(cursorAt(samples, 50)).toEqual({ x: 50, y: 25 })
    expect(cursorAt(samples, 500)).toEqual({ x: 100, y: 50 })
  })
})

describe('timecode', () => {
  it('formats minutes and tenths', () => {
    expect(timecode(0)).toBe('00:00.0')
    expect(timecode(83_456)).toBe('01:23.5')
  })
})
