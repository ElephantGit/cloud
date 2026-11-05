/**
 * Records one shot against a running demo environment: `npm run record -- <shot>`. Writes the
 * screencast frames, `timeline.json` and the transcript to `out/<shot>/`, replacing the last take.
 */
import { mkdir, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'
import { shots } from '../shots/index.ts'
import type { Recording } from '../timeline.ts'
import { Director } from './director.ts'
import { scenePorts } from './shot.ts'
import { lines, markdown, srt } from './transcript.ts'

/**
 * The page lays out at 1280×720, so the UI reads at video size, and renders `scale` device pixels
 * per CSS pixel. The scale has to be the browser's own (`--force-device-scale-factor` on a real
 * window): Chromium's screencast captures emulated viewports at CSS size whatever their emulated
 * scale factor, and CSS `zoom` would also stretch every `vh`-based layout. 2.5 gives 3200×1800 at
 * about 45 fps on a CPU-rendered headless shell.
 */
const viewport = { width: 1280, height: 720 }
const scale = Number(process.env.VIDEO_SCALE ?? 2.5)

const name = process.argv[2]
const shot = shots[name ?? '']
if (!shot) {
  console.error(`usage: npm run record -- <shot>; shots: ${Object.keys(shots).join(', ')}`)
  process.exit(2)
}
const base = process.env.VIDEO_BASE_URL ?? `http://127.0.0.1:${scenePorts[shot.scene]}`
const out = path.resolve(import.meta.dirname, '../../out', shot.name)
await rm(out, { recursive: true, force: true })
await mkdir(path.join(out, 'frames'), { recursive: true })

const browser = await chromium.launch({
  executablePath: process.env.PW_CHROMIUM_PATH || undefined,
  args: [`--force-device-scale-factor=${scale}`, `--window-size=${viewport.width},${viewport.height}`],
})
try {
  // No viewport emulation: emulating one would bring back CSS-sized screencast frames.
  const context = await browser.newContext({ viewport: null, locale: 'zh-CN' })
  const page = await context.newPage()
  await page.goto(`${base}${shot.start}`)
  await page.getByLabel('邮箱').fill(shot.account)
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByRole('button', { name: '退出登录', exact: true }).waitFor()
  await page.goto(`${base}${shot.start}`)
  await shot.ready(page)
  const actual = await page.evaluate(() => ({ width: innerWidth, height: innerHeight, scale: devicePixelRatio }))
  if (actual.width !== viewport.width || actual.height !== viewport.height || actual.scale !== scale) {
    throw new Error(`Browser window is ${JSON.stringify(actual)}, expected ${JSON.stringify({ ...viewport, scale })}.`)
  }
  // Let fonts and the last query settle, so the first frame is the page as it will stay.
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(500)

  const frames: Recording['frames'] = []
  const writes: Promise<void>[] = []
  const urls: Recording['urls'] = []
  const cdp = await context.newCDPSession(page)
  const start = Date.now()
  cdp.on('Page.screencastFrame', frame => {
    const file = `frames/${String(frames.length).padStart(6, '0')}.jpg`
    frames.push({ t: frame.metadata.timestamp ? frame.metadata.timestamp * 1000 - start : Date.now() - start, file })
    writes.push(writeFile(path.join(out, file), Buffer.from(frame.data, 'base64')))
    void cdp.send('Page.screencastFrameAck', { sessionId: frame.sessionId }).catch(() => {})
  })
  page.on('framenavigated', frame => {
    if (frame === page.mainFrame()) urls.push({ t: Date.now() - start, url: frame.url() })
  })
  urls.push({ t: 0, url: page.url() })
  await cdp.send('Page.startScreencast', {
    format: 'jpeg',
    quality: 92,
    maxWidth: viewport.width * scale,
    maxHeight: viewport.height * scale,
  })

  const director = new Director(page, start, viewport)
  director.park()
  await shot.play(director, page)
  const duration = director.now()
  await cdp.send('Page.stopScreencast')
  await Promise.all(writes)

  const recording: Recording = {
    shot: shot.name,
    viewport: { ...viewport, scale },
    duration,
    // Screencast timestamps can arrive slightly out of order; the composition expects time order.
    frames: frames.sort((a, b) => a.t - b.t),
    cursor: director.cursor,
    clicks: director.clicks,
    camera: director.camera,
    speed: director.speed,
    cues: director.cues,
    urls,
  }
  await writeFile(path.join(out, 'timeline.json'), JSON.stringify(recording))
  const script = lines(recording)
  await writeFile(path.join(out, 'transcript.md'), markdown(shot.name, script))
  await writeFile(path.join(out, 'transcript.srt'), srt(script))
  console.log(`Recorded ${shot.name}: ${frames.length} frames, ${(duration / 1000).toFixed(1)}s → ${out}`)
} finally {
  await browser.close()
}
