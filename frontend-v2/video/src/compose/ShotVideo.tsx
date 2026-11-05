import { useMemo } from 'react'
import { AbsoluteFill, Img, interpolate, staticFile, useCurrentFrame } from 'remotion'
import {
  blend, cameraAt, cuesOnOutput, cursorAt, lastAt, rampAt, toOutputTime, toRecordTime, viewFor,
  type ExplainOverlay, type MarkOverlay, type Recording, type SlideOverlay,
} from '../timeline.ts'
import { Cursor, Ripple, RIPPLE_MS } from './Cursor.tsx'
import { FPS, HEIGHT, SHOT_BACKGROUND, stageFor, TITLE_BAR, WIDTH } from './layout.ts'
import { ASIDE_CENTER, ASIDE_SCALE, ExplainPanel, MarkBox, panelProgress, SlidePanel } from './Panels.tsx'

/** Props of a shot composition: the recording, loaded from `out/<shot>/timeline.json`. */
export type ShotProps = { recording: Recording }

/** One recorded shot: the screencast inside a window, filmed by the camera, with cursor, panels and subtitles. */
export function ShotVideo({ recording }: ShotProps) {
  const frame = useCurrentFrame()
  const out = (frame / FPS) * 1000
  const t = toRecordTime(recording, out)
  const stage = useMemo(() => stageFor(recording.viewport), [recording.viewport])
  const keys = useMemo(() => recording.camera.map(k => ({
    t: toOutputTime(recording, k.t),
    view: viewFor(stage, recording.viewport, k.box, k.zoom),
    move: k.move,
  })), [recording, stage])
  const cues = useMemo(() => cuesOnOutput(recording), [recording])
  const overlays = useMemo(() => recording.overlays.map(o => ({ ...o, at: toOutputTime(recording, o.t) })), [recording])
  const live = overlays.filter(o => out >= o.at && out < o.at + o.ms)
  const slide = live.find(o => o.kind === 'slide') as (SlideOverlay & { at: number }) | undefined
  const explain = live.find(o => o.kind === 'explain') as (ExplainOverlay & { at: number }) | undefined
  const marks = live.filter(o => o.kind === 'mark') as (MarkOverlay & { at: number })[]

  const aside = explain ? panelProgress(out - explain.at, explain.ms) : 0
  const filmed = cameraAt(keys, out, viewFor(stage, recording.viewport, null))
  // Stepping aside frames the whole window, so it fills its place instead of leaving a margin of background.
  const wholeWindow = {
    cx: WIDTH / 2,
    cy: stage.content.y + (stage.content.height - TITLE_BAR) / 2,
    scale: Math.min((WIDTH * 0.98) / stage.content.width, (HEIGHT * 0.98) / (stage.content.height + TITLE_BAR)),
  }
  const view = explain ? blend(filmed, explain.box ? viewFor(stage, recording.viewport, explain.box, explain.zoom) : wholeWindow, aside) : filmed
  const screen = lastAt(recording.frames, t) ?? recording.frames[0]
  const url = lastAt(recording.urls, t)?.url ?? ''
  const pointer = cursorAt(recording.cursor, t)
  const k = stage.content.width / recording.viewport.width
  const ramp = rampAt(recording, out)
  const clicks = recording.clicks.filter(c => out >= toOutputTime(recording, c.t) && out < toOutputTime(recording, c.t) + RIPPLE_MS)
  const cue = lastAt(cues, out)
  const subtitle = cue?.narration || cue?.caption

  const { content } = stage
  const scale = 1 + (ASIDE_SCALE - 1) * aside
  const centerX = WIDTH / 2 + ((explain?.side === 'left' ? 1 - ASIDE_CENTER : ASIDE_CENTER) * WIDTH - WIDTH / 2) * aside
  return <AbsoluteFill style={{ background: SHOT_BACKGROUND }}>
    {explain && <ExplainPanel overlay={explain} age={out - explain.at} />}
    <AbsoluteFill style={{
      overflow: 'hidden', borderRadius: 40 * aside,
      transform: `translateX(${centerX - WIDTH / 2}px) scale(${scale})`,
    }}>
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
        {marks.map(m => <MarkBox key={m.t} overlay={m} age={out - m.at} k={k} origin={content} />)}
        {clicks.map(c => <Ripple key={c.t} x={content.x + c.x * k} y={content.y + c.y * k}
          age={out - toOutputTime(recording, c.t)} />)}
        {pointer && <Cursor x={content.x + pointer.x * k} y={content.y + pointer.y * k}
          pressed={clicks.some(c => out - toOutputTime(recording, c.t) < 140)} />}
      </AbsoluteFill>
    </AbsoluteFill>
    {ramp && <SpeedBadge rate={ramp.rate} />}
    {slide && <SlidePanel overlay={slide} age={out - slide.at} opaqueEntry={slide.at < 50} />}
    {subtitle && cue && <Subtitle key={cue.t} text={subtitle} age={out - cue.t} />}
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

function Subtitle({ text, age }: { text: string; age: number }) {
  const enter = interpolate(age, [0, 250], [0, 1], { extrapolateRight: 'clamp' })
  return <div style={{
    position: 'absolute', left: 0, right: 0, bottom: 96, display: 'flex', justifyContent: 'center',
    opacity: enter, transform: `translateY(${(1 - enter) * 16}px)`,
  }}>
    <div style={{
      maxWidth: 2900, padding: '20px 48px', borderRadius: 20, background: 'rgba(17, 24, 39, 0.82)', color: '#fff', textAlign: 'center',
      fontSize: 56, fontWeight: 500, lineHeight: 1.4, fontFamily: '"Geist Variable", "Noto Sans SC Variable", system-ui, sans-serif',
      boxShadow: '0 20px 60px rgba(0, 0, 0, 0.25)',
    }}>{text}</div>
  </div>
}
