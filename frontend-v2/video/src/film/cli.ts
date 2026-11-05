/**
 * Joins rendered parts into the film without re-encoding: `npm run film` joins the opening, every
 * shot in `shots/index.ts` and the closing; `npm run film -- a b` joins only the shots named. Each
 * shot needs `out/<shot>.mp4` and `out/<shot>/timeline.json`. Writes `out/film.mp4` and its script.
 */
import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { markdown, srt } from '../record/transcript.ts'
import { shots } from '../shots/index.ts'
import type { Recording } from '../timeline.ts'
import { filmOrder, filmScript } from './script.ts'

const out = path.resolve(import.meta.dirname, '../../out')
const named = process.argv.slice(2)
const order = filmOrder(named.length ? named : Object.keys(shots))

const missing = order.filter(part => !existsSync(path.join(out, `${part}.mp4`)))
if (missing.length) {
  console.error(`Render these first: ${missing.join(', ')}`)
  process.exit(1)
}
const recordings: Record<string, Recording> = {}
for (const part of order) {
  if (part === 'intro' || part === 'outro') continue
  recordings[part] = JSON.parse(readFileSync(path.join(out, part, 'timeline.json'), 'utf8')) as Recording
}
const script = filmScript(order, recordings)

const list = path.join(out, 'film.txt')
writeFileSync(list, order.map(part => `file '${path.join(out, `${part}.mp4`)}'`).join('\n'))
const joined = spawnSync('npx', ['remotion', 'ffmpeg', '-y', '-f', 'concat', '-safe', '0', '-i', list, '-c', 'copy', path.join(out, 'film.mp4')], { stdio: 'inherit' })
if (joined.status !== 0) process.exit(joined.status ?? 1)
writeFileSync(path.join(out, 'film.md'), markdown('整片', script))
writeFileSync(path.join(out, 'film.srt'), srt(script))
console.log(`Joined ${order.join(' + ')} → ${path.join(out, 'film.mp4')}`)
