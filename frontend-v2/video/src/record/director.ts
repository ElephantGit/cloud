import type { Locator, Page } from 'playwright'
import type { Box, CameraKey, Cue, Sample, SpeedRamp } from '../timeline.ts'

/** What a shot can ask the camera to look at: an element, a fixed box, or the whole page. */
export type Focus = Locator | Box | 'page'

/** Options shared by the director's actions. */
export interface ActionOptions {
  /** Where the camera goes when the action starts; defaults to the action's target. `false` keeps it still. */
  camera?: Focus | false
  zoom?: number
}

/**
 * The API a shot script performs with. Every action drives the real page and logs what the
 * composition needs later: cursor samples, clicks, camera keys, speed ramps and text cues, all in
 * milliseconds since recording started.
 */
export class Director {
  readonly cursor: Sample[] = []
  readonly clicks: Sample[] = []
  readonly camera: CameraKey[] = []
  readonly speed: SpeedRamp[] = []
  readonly cues: Cue[] = []
  private at = { x: 0, y: 0 }
  private readonly page: Page
  private readonly start: number

  constructor(page: Page, start: number, viewport: { width: number; height: number }) {
    this.page = page
    this.start = start
    // The cursor enters from the lower right, where it would rest before a demo begins.
    this.at = { x: viewport.width * 0.82, y: viewport.height * 0.86 }
  }

  /** Milliseconds since recording started. */
  now(): number {
    return Date.now() - this.start
  }

  /** Shows `caption` on screen (an empty string clears it) and adds `narration` to the transcript. */
  say(cue: { caption?: string; narration?: string }): void {
    this.cues.push({ t: this.now(), ...cue })
  }

  /** Points the camera at `target`; the composition eases into it. */
  async focus(target: Focus, zoom?: number): Promise<void> {
    this.camera.push({ t: this.now(), box: await boxOf(target), zoom })
  }

  /** Lets the scene breathe for `ms` of recording time. */
  async hold(ms: number): Promise<void> {
    await this.page.waitForTimeout(ms)
  }

  /** Moves the cursor to the target's center along an eased path, then clicks it. */
  async click(target: Locator, options: ActionOptions = {}): Promise<void> {
    await target.scrollIntoViewIfNeeded()
    await this.aim(target, options)
    const box = await requireBox(target)
    await this.move(box.x + box.width / 2, box.y + box.height / 2)
    await this.page.waitForTimeout(120)
    this.clicks.push({ t: this.now(), ...this.at })
    await this.page.mouse.down()
    await this.page.waitForTimeout(70)
    await this.page.mouse.up()
    await this.page.waitForTimeout(250)
  }

  /** Clicks a field and types into it at a human pace; `rate` speeds long text up in the video. */
  async type(target: Locator, text: string, options: ActionOptions & { rate?: number } = {}): Promise<void> {
    await this.click(target, options)
    const from = this.now()
    await this.page.keyboard.type(text, { delay: 45 })
    if (options.rate && options.rate > 1) this.ramp(from, this.now(), options.rate)
    await this.page.waitForTimeout(300)
  }

  /** Picks an option of a native select without opening it, since the OS popup never reaches the screencast. */
  async select(target: Locator, label: string, options: ActionOptions = {}): Promise<void> {
    await this.click(target, options)
    await target.selectOption({ label })
    await this.page.keyboard.press('Escape')
  }

  /**
   * Waits until `until` is visible, playing the wait back `rate` times faster. The camera keeps
   * whatever the shot points it at, so a long run can be watched as it progresses.
   */
  async wait(until: Locator, options: ActionOptions & { rate?: number; timeout?: number } = {}): Promise<void> {
    if (options.camera) await this.focus(options.camera, options.zoom)
    const from = this.now()
    await until.waitFor({ state: 'visible', timeout: options.timeout ?? 300_000 })
    // Ease out of the ramp a little after the result appears, so it lands at normal speed.
    this.ramp(from, Math.max(from, this.now() - 200), options.rate ?? 8)
    await this.page.waitForTimeout(400)
  }

  private ramp(from: number, to: number, rate: number): void {
    if (to - from > 300) this.speed.push({ from, to, rate })
  }

  private async aim(target: Locator, options: ActionOptions): Promise<void> {
    if (options.camera === false) return
    await this.focus(options.camera ?? target, options.zoom)
  }

  /** Moves the real mouse (so hover states show) along an eased path, sampling every frame. */
  private async move(x: number, y: number): Promise<void> {
    const from = this.at
    const distance = Math.hypot(x - from.x, y - from.y)
    const duration = Math.min(Math.max(distance * 0.9, 350), 900)
    const steps = Math.max(Math.round(duration / 16), 1)
    for (let i = 1; i <= steps; i++) {
      const p = easeOutQuart(i / steps)
      const point = { x: from.x + (x - from.x) * p, y: from.y + (y - from.y) * p }
      await this.page.mouse.move(point.x, point.y)
      this.cursor.push({ t: this.now(), ...point })
      await this.page.waitForTimeout(16)
    }
    this.at = { x, y }
  }

  /** Records where the cursor rests before the shot's first move, so it is on screen from the start. */
  park(): void {
    this.cursor.push({ t: 0, ...this.at })
  }
}

function easeOutQuart(x: number): number {
  return 1 - (1 - x) ** 4
}

async function boxOf(target: Focus): Promise<Box | null> {
  if (target === 'page') return null
  if ('boundingBox' in target) return requireBox(target)
  return target
}

async function requireBox(target: Locator): Promise<Box> {
  const box = await target.boundingBox()
  if (!box) throw new Error(`Not on screen: ${target.toString()}`)
  return box
}
