import type { Page } from 'playwright'
import type { Director } from './director.ts'

/** The demo scenes of #882, each served by its own `task v2:demo:<scene>`. */
export type Scene = 'newcomer' | 'lead' | 'ops'

/** The frontend port of each scene's single-scene demo command (v2-server/demo/env.go). */
export const scenePorts: Record<Scene, number> = { newcomer: 5191, lead: 5192, ops: 5193 }

/**
 * One shot of the video: who signs in where, and the script performed once recording starts.
 * Signing in and the first page load happen before recording, so the shot opens on a settled page.
 */
export interface RecordedShot {
  name: string
  scene: Scene
  /** The preset account to sign in as (v2-server/demo/users/<scene>.json). */
  account: string
  /** The path recording starts on. */
  start: string
  /** Waits until the start page is ready to be filmed. */
  ready(page: Page): Promise<void>
  play(d: Director, page: Page): Promise<void>
}

/** A shot made only of slides, which needs no demo environment. */
export interface SlidesShot {
  name: string
  scene: null
  play(d: Director): Promise<void> | void
}

export type Shot = RecordedShot | SlidesShot
