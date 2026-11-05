import { describe, expect, it } from 'vitest'
import {
  CAMERA_MOVE, cameraAt, clampView, cursorAt, lastAt, MAX_ZOOM, rampAt, timecode, toOutputTime, toRecordTime, viewFor,
  type Stage,
} from './timeline.ts'

const ramps = [{ from: 1000, to: 3000, rate: 4 }, { from: 5000, to: 6000, rate: 2 }]

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
