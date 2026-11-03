import type { Stage } from '../timeline.ts'

/** 4K UHD at 60 fps. */
export const WIDTH = 3840
export const HEIGHT = 2160
export const FPS = 60

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
  const top = (HEIGHT - height - TITLE_BAR) / 2
  return {
    width: WIDTH,
    height: HEIGHT,
    content: { x: (WIDTH - width) / 2, y: top + TITLE_BAR, width, height },
  }
}
