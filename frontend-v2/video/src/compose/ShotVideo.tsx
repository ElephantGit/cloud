import { useMemo } from 'react'
import { AbsoluteFill, Img, interpolate, staticFile, useCurrentFrame } from 'remotion'
import {
  cameraAt, cursorAt, lastAt, rampAt, toOutputTime, toRecordTime, viewFor, type Recording,
} from '../timeline.ts'
import { FPS, stageFor, TITLE_BAR } from './layout.ts'
import { Cursor, Ripple, RIPPLE_MS } from './Cursor.tsx'

/** Props of a shot composition: the recording, loaded from `out/<shot>/timeline.json`. */
export type ShotProps = { recording: Recording }

/** One recorded shot: the screencast inside a window, filmed by the camera, with cursor and captions. */
export function ShotVideo({ recording }: ShotProps) {
  const frame = useCurrentFrame()
  const out = (frame / FPS) * 1000
  const t = toRecordTime(recording.speed, out)
  const stage = useMemo(() => stageFor(recording.viewport), [recording.viewport])
  const keys = useMemo(() => recording.camera.map(k => ({
    t: toOutputTime(recording.speed, k.t),
    view: viewFor(stage, recording.viewport, k.box, k.zoom),
  })), [recording, stage])
  const view = cameraAt(keys, out, viewFor(stage, recording.viewport, null))
  const screen = lastAt(recording.frames, t) ?? recording.frames[0]
  const url = lastAt(recording.urls, t)?.url ?? ''
  const pointer = cursorAt(recording.cursor, t)
  const k = stage.content.width / recording.viewport.width
  const ramp = rampAt(recording.speed, out)
  const clicks = recording.clicks.filter(c => out >= toOutputTime(recording.speed, c.t) && out < toOutputTime(recording.speed, c.t) + RIPPLE_MS)
  const caption = lastAt(recording.cues.filter(c => c.caption !== undefined), t)
  const captionAge = caption ? out - toOutputTime(recording.speed, caption.t) : 0

  const { content } = stage
  return <AbsoluteFill style={{ background: 'radial-gradient(120% 120% at 20% 10%, #eef2ff 0%, #dfe7f5 45%, #cfd8ea 100%)' }}>
    <AbsoluteFill style={{
      transformOrigin: '0 0',
      transform: `translate(${stage.width / 2 - view.cx * view.scale}px, ${stage.height / 2 - view.cy * view.scale}px) scale(${view.scale})`,
    }}>
      <div style={{
        position: 'absolute', left: content.x, top: content.y - TITLE_BAR, width: content.width, height: content.height + TITLE_BAR,
        borderRadius: 28, overflow: 'hidden', background: '#fff',
        boxShadow: '0 40px 120px rgba(30, 41, 79, 0.28), 0 0 0 1.5px rgba(30, 41, 79, 0.10)',
      }}>
        <TitleBar url={url} />
        {screen && <Img src={staticFile(`${recording.shot}/${screen.file}`)}
          style={{ position: 'absolute', top: TITLE_BAR, left: 0, width: content.width, height: content.height }} />}
      </div>
      {clicks.map(c => <Ripple key={c.t} x={content.x + c.x * k} y={content.y + c.y * k}
        age={out - toOutputTime(recording.speed, c.t)} />)}
      {pointer && <Cursor x={content.x + pointer.x * k} y={content.y + pointer.y * k}
        pressed={clicks.some(c => out - toOutputTime(recording.speed, c.t) < 140)} />}
    </AbsoluteFill>
    {ramp && <SpeedBadge rate={ramp.rate} />}
    {caption?.caption && <Caption text={caption.caption} age={captionAge} />}
  </AbsoluteFill>
}

function TitleBar({ url }: { url: string }) {
  const shown = url.replace(/^https?:\/\/[^/]+/, 'ora.space')
  return <div style={{
    position: 'absolute', inset: 0, height: TITLE_BAR, background: '#f3f4f7',
    borderBottom: '1.5px solid #e3e6ec', display: 'flex', alignItems: 'center', padding: '0 30px',
  }}>
    <div style={{ display: 'flex', gap: 16 }}>
      {['#ff5f57', '#febc2e', '#28c840'].map(c => <span key={c} style={{ width: 22, height: 22, borderRadius: 11, background: c }} />)}
    </div>
    <div style={{
      margin: '0 auto', width: '38%', height: 44, borderRadius: 12, background: '#fff', color: '#5b6375',
      border: '1.5px solid #e3e6ec', display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontSize: 24, fontFamily: 'system-ui, sans-serif',
    }}>{shown}</div>
  </div>
}

function SpeedBadge({ rate }: { rate: number }) {
  return <div style={{
    position: 'absolute', right: 120, top: 100, padding: '18px 34px', borderRadius: 999,
    background: 'rgba(17, 24, 39, 0.78)', color: '#fff', fontSize: 44, fontWeight: 600,
    fontFamily: 'system-ui, sans-serif', letterSpacing: 2,
  }}>▶▶ ×{rate}</div>
}

function Caption({ text, age }: { text: string; age: number }) {
  const enter = interpolate(age, [0, 350], [0, 1], { extrapolateRight: 'clamp' })
  return <div style={{
    position: 'absolute', left: 0, right: 0, bottom: 150, display: 'flex', justifyContent: 'center',
    opacity: enter, transform: `translateY(${(1 - enter) * 24}px)`,
  }}>
    <div style={{
      padding: '26px 56px', borderRadius: 24, background: 'rgba(17, 24, 39, 0.82)', color: '#fff',
      fontSize: 64, fontWeight: 600, fontFamily: '"Noto Sans SC Variable", system-ui, sans-serif',
      boxShadow: '0 20px 60px rgba(0, 0, 0, 0.25)',
    }}>{text}</div>
  </div>
}
