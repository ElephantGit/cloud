import type { Locator, Page } from 'playwright'
import { stageFor } from '../compose/layout.ts'
import {
  narrationMs, needsPan, PAN_MOVE, toOutputTime, viewFor, type Box, type CameraKey, type Cue, type Hold, type Overlay,
  type PanelItem, type Sample, type SpeedRamp, type View,
} from '../timeline.ts'
import { PANEL_ENTER, PANEL_EXIT } from './timing.ts'

/** What a shot can ask the camera to look at: an element, a fixed box, or the whole page. */
export type Focus = Locator | Box | 'page'

/** Options shared by the director's actions. */
export interface ActionOptions {
  /** Where the camera goes when the action starts; by default it pans only if the target is out of frame. `false` keeps it still. */
  camera?: Focus | false
  zoom?: number
}

/** One card of a slide or an explanation; it comes in when its line of narration starts. */
export interface Beat { title: string; body?: string; narration?: string }

/** A full-screen chapter page. */
export interface SlideOptions { eyebrow?: string; title: string; subtitle?: string; narration?: string; items?: Beat[] }

/** The window steps aside and cards come in beside it; `focus` pushes in inside the smaller window so its text stays readable. */
export interface ExplainOptions {
  title?: string
  narration?: string
  items: Beat[]
  side?: 'left' | 'right'
  focus?: Focus
  zoom?: number
}

/** Freezes shorter than this go unnoticed, so the transcript does not flag them. */
const NOTICEABLE_PAD = 300
const PANEL_DWELL = 500
const MIN_PANEL = 2400
const MARK_MS = 2600
/** A ramp this fast removes a span from the video, without a speed badge: it plays in microseconds. */
const CUT_RATE = 1e6

/** How a shot signs in; the recorder and {@link Director.signIn} both go through it. */
export async function signIn(page: Page, base: string, account: string, path: string): Promise<void> {
  await page.context().clearCookies()
  await page.goto(`${base}${path}`)
  await page.evaluate(() => { localStorage.clear(); sessionStorage.clear() })
  await page.goto(`${base}${path}`)
  await page.getByLabel('邮箱').fill(account)
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByRole('button', { name: '退出登录', exact: true }).waitFor()
  await page.goto(`${base}${path}`)
}

/**
 * The API a shot script performs with. Every action drives the real page and logs what the
 * composition needs later: cursor samples, clicks, camera keys, speed ramps, holds, overlays and
 * text cues, all in milliseconds since recording started.
 *
 * Narration sets the pace: a line keeps the screen until it has been read. When the picture runs
 * out first, the video freezes on the last frame until it has, so nothing has to wait for real.
 * Actions only pan the camera; the zoom changes only when a shot says so with {@link focus}.
 */
export class Director {
  readonly cursor: Sample[] = []
  readonly clicks: Sample[] = []
  readonly camera: CameraKey[] = []
  readonly speed: SpeedRamp[] = []
  readonly holds: Hold[] = []
  readonly cues: Cue[] = []
  readonly overlays: Overlay[] = []
  private at = { x: 0, y: 0 }
  private view: View
  private floor = 0
  private speaking?: { cue: Cue; until: number }
  private readonly page: Page
  private readonly start: number
  private readonly base: string
  private readonly viewport: { width: number; height: number }
  private readonly stage: ReturnType<typeof stageFor>

  constructor(page: Page, start: number, viewport: { width: number; height: number }, base: string) {
    this.page = page
    this.start = start
    this.base = base
    this.viewport = viewport
    this.stage = stageFor(viewport)
    this.view = viewFor(this.stage, viewport, null)
    // The cursor enters from the lower right, where it would rest before a demo begins.
    this.at = { x: viewport.width * 0.82, y: viewport.height * 0.86 }
  }

  /** Milliseconds since recording started; never repeats a moment that holds. */
  now(): number {
    return Math.max(Date.now() - this.start, this.floor)
  }

  /** Shows `narration` as a subtitle (or a short `caption`; an empty string clears it) and adds it to the transcript. */
  say(cue: { caption?: string; narration?: string }): void {
    this.settle()
    const placed: Cue = { t: this.now(), ...cue }
    this.cues.push(placed)
    this.speaking = cue.narration ? { cue: placed, until: this.output() + narrationMs(cue.narration) } : undefined
  }

  /** A full-screen chapter page. It takes as long as its narration; the recording does not wait for it. */
  slide(options: SlideOptions): void {
    this.panel(options, (t, ms, items) => this.overlays.push({
      kind: 'slide', t, ms, eyebrow: options.eyebrow, title: options.title, subtitle: options.subtitle, items,
    }))
  }

  /** The window steps aside and freezes while cards come in beside it, then returns. */
  async explain(options: ExplainOptions): Promise<void> {
    const box = options.focus ? await boxOf(options.focus) : null
    this.panel(options, (t, ms, items) => this.overlays.push({
      kind: 'explain', t, ms, side: options.side ?? 'right', title: options.title, items, box, zoom: options.zoom,
    }))
  }

  /** Highlights an element with an optional short label. Keep paragraphs for {@link explain}. */
  async mark(target: Locator, options: { label?: string; narration?: string; ms?: number } = {}): Promise<void> {
    if (options.narration) this.say({ narration: options.narration })
    const ms = options.ms ?? Math.max(MARK_MS, options.narration ? narrationMs(options.narration) : 0)
    this.overlays.push({ kind: 'mark', t: this.now(), ms, box: await requireBox(target), label: options.label })
  }

  /**
   * Signs in as someone else behind a chapter page: the page covers the whole time it takes, and that
   * time is cut from the video, so the shot picks up on the new account's settled page.
   */
  async switchAccount(account: string, path: string, slide: SlideOptions, ready?: () => Promise<void>): Promise<void> {
    let from = 0
    let total = 0
    this.panel(slide, (t, ms, items) => {
      from = t
      total = ms
      this.overlays.push({ kind: 'slide', t, ms, eyebrow: slide.eyebrow, title: slide.title, subtitle: slide.subtitle, items })
    })
    // Everything but the fade-out holds on the old page; the fade-out reveals the new one.
    this.holds[this.holds.length - 1]!.ms = total - PANEL_EXIT
    await signIn(this.page, this.base, account, path)
    await ready?.()
    await this.page.waitForLoadState('networkidle')
    await this.page.waitForTimeout(500)
    const after = this.now()
    this.speed.push({ from, to: after, rate: CUT_RATE })
    this.holds.push({ t: after, ms: PANEL_EXIT })
    this.floor = after + 1
    this.view = viewFor(this.stage, this.viewport, null)
    this.camera.push({ t: after, box: null, zoom: 1 })
  }

  /** Points the camera at `target`, the one way to change the zoom; the composition eases into it. */
  async focus(target: Focus, zoom?: number): Promise<void> {
    const box = await boxOf(target)
    this.camera.push({ t: this.now(), box, zoom })
    this.view = viewFor(this.stage, this.viewport, box, zoom)
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

  /** Gives the last line its time and returns the recording's duration. */
  finish(): number {
    this.settle()
    return this.now()
  }

  private output(): number {
    return toOutputTime(this, this.now())
  }

  /** Freezes the video until the line being spoken has had its time. */
  private settle(): void {
    if (!this.speaking) return
    const short = this.speaking.until - this.output()
    if (short > 0) {
      const t = this.now()
      this.holds.push({ t, ms: short })
      if (short >= NOTICEABLE_PAD) this.speaking.cue.padded = short
      this.floor = t + 1
    }
    this.speaking = undefined
  }

  /** Lays the narration out over a new hold, cards following their lines, and hands the result to `place`. */
  private panel(options: { narration?: string; items?: Beat[] }, place: (t: number, ms: number, items: PanelItem[]) => void): void {
    this.settle()
    const t = this.now()
    let at = PANEL_ENTER
    const line = (narration: string) => {
      this.cues.push({ t, offset: at, narration })
      at += narrationMs(narration)
    }
    if (options.narration) line(options.narration)
    const items = (options.items ?? []).map(({ title, body, narration }) => {
      const item: PanelItem = { title, body, at }
      if (narration) line(narration)
      else at += MIN_PANEL / 2
      return item
    })
    at = Math.max(at + PANEL_DWELL, MIN_PANEL)
    this.cues.push({ t, offset: at, caption: '' })
    const ms = at + PANEL_EXIT
    this.holds.push({ t, ms })
    place(t, ms, items)
    this.floor = t + 1
  }

  private ramp(from: number, to: number, rate: number): void {
    if (to - from > 300) this.speed.push({ from, to, rate })
  }

  private async aim(target: Locator, options: ActionOptions): Promise<void> {
    if (options.camera === false) return
    if (options.camera) return this.focus(options.camera, options.zoom)
    const box = await requireBox(target)
    const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
    if (!needsPan(this.stage, this.viewport, this.view, point)) return
    const center = { ...point, width: 0, height: 0 }
    this.camera.push({ t: this.now(), box: center, zoom: this.view.scale, move: PAN_MOVE })
    this.view = viewFor(this.stage, this.viewport, center, this.view.scale)
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
