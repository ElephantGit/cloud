import type { Stage } from '../timeline.ts'

/** 4K UHD at 60 fps. */
export const WIDTH = 3840
export const HEIGHT = 2160
export const FPS = 60

/** The opening and closing, in frames; the film needs them without loading their components. */
export const INTRO_FRAMES = 360
export const OUTRO_FRAMES = 300

/** The canvas behind the browser window in every shot; the opening dissolves into it. */
export const SHOT_BACKGROUND = 'radial-gradient(120% 120% at 20% 10%, #eef2ff 0%, #dfe7f5 45%, #cfd8ea 100%)'

/** The window's title bar, in canvas pixels. */
export const TITLE_BAR = 76

/**
 * Where the browser window sits on the canvas for a viewport of the given aspect: as wide as it can
 * be while leaving a margin of background on every side.
 */
export function stageFor(viewport: { width: number; height: number }): Stage {
  const maxW = WIDTH * 0.86
  const maxH = HEIGHT * 0.86 - TITLE_BAR
  const width = Math.min(maxW, (maxH * viewport.width) / viewport.height)
  const height = (width * viewport.height) / viewport.width
  // A little above centre, so subtitles sit in the space below the window instead of over it.
  const top = (HEIGHT - height - TITLE_BAR) / 2 - 40
  return {
    width: WIDTH,
    height: HEIGHT,
    content: { x: (WIDTH - width) / 2, y: top + TITLE_BAR, width, height },
  }
}
