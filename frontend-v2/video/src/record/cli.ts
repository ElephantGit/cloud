/**
 * Records one shot against a running demo environment: `npm run record -- <shot>`. Writes the
 * screencast frames, `timeline.json` and the transcript to `out/<shot>/`, replacing the last take.
 */
import { mkdir, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'
import { stageFor } from '../compose/layout.ts'
import { shots } from '../shots/index.ts'
import { restlessZooms, timecode, toOutputTime, viewFor, type Recording } from '../timeline.ts'
import { Director, signIn } from './director.ts'
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
const base = process.env.VIDEO_BASE_URL ?? (shot.scene ? `http://127.0.0.1:${scenePorts[shot.scene]}` : '')
const out = path.resolve(import.meta.dirname, '../../out', shot.name)
await rm(out, { recursive: true, force: true })
await mkdir(path.join(out, 'frames'), { recursive: true })

/**
 * The app's font stack falls through to whatever Chinese font the machine has, so the same page
 * would look different from one person's recording to the next. Every recording uses the font the
 * titles use instead, served from this package.
 */
const fontsDir = path.resolve(import.meta.dirname, '../../node_modules/@fontsource-variable')
const fontStack = `'Geist Variable', 'Noto Sans SC Variable', sans-serif`

const browser = await chromium.launch({
  executablePath: process.env.PW_CHROMIUM_PATH || undefined,
  args: [`--force-device-scale-factor=${scale}`, `--window-size=${viewport.width},${viewport.height}`],
})
try {
  // No viewport emulation: emulating one would bring back CSS-sized screencast frames.
  const context = await browser.newContext({ viewport: null, locale: 'zh-CN' })
  await context.route('**/__video_fonts/**', route => {
    const file = new URL(route.request().url()).pathname.replace('/__video_fonts/', '')
    return route.fulfill({ path: path.join(fontsDir, file) })
  })
  await context.addInitScript(stack => {
    document.addEventListener('DOMContentLoaded', () => {
      const style = document.createElement('style')
      style.textContent = `@import '/__video_fonts/noto-sans-sc/index.css';
        :root { --font-sans: ${stack} !important; } body { font-family: ${stack}; }`
      document.head.append(style)
    })
  }, fontStack)
  const page = await context.newPage()
  if (shot.scene) {
    await signIn(page, base, shot.account, shot.start)
    await shot.ready(page)
  }
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

  const director = new Director(page, start, viewport, base)
  director.park()
  await shot.play(director, page)
  const duration = director.finish()
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
    holds: director.holds,
    cues: director.cues,
    overlays: director.overlays,
    urls,
  }
  await writeFile(path.join(out, 'timeline.json'), JSON.stringify(recording))
  const script = lines(recording)
  await writeFile(path.join(out, 'transcript.md'), markdown(shot.name, script))
  await writeFile(path.join(out, 'transcript.srt'), srt(script))
  console.log(`Recorded ${shot.name}: ${frames.length} frames, ${(toOutputTime(recording, duration) / 1000).toFixed(1)}s in the video → ${out}`)
  for (const l of script.filter(l => l.padded)) {
    console.warn(`节奏：${timecode(l.start)} 这句画面不够，定格补了 ${(l.padded! / 1000).toFixed(1)} 秒：${l.narration}`)
  }
  const stage = stageFor(viewport)
  const keys = recording.camera.map(k => ({ t: toOutputTime(recording, k.t), view: viewFor(stage, viewport, k.box, k.zoom) }))
  for (const t of restlessZooms(keys, viewFor(stage, viewport, null))) {
    console.warn(`镜头：${timecode(t)} 距离上次变焦不到 3 秒，看着容易晕。这里改成平移？`)
  }
} finally {
  await browser.close()
}
