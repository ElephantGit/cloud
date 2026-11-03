import '@fontsource-variable/noto-sans-sc'
import { Composition, staticFile } from 'remotion'
import { shots } from '../shots/index.ts'
import { toOutputTime, type Recording } from '../timeline.ts'
import { FPS, HEIGHT, WIDTH } from './layout.ts'
import { ShotVideo, type ShotProps } from './ShotVideo.tsx'

/** One composition per shot, sized from its latest recording in `out/<shot>/`. */
export function Root() {
  return <>
    {Object.keys(shots).map(name => <Composition key={name} id={name} component={ShotVideo}
      width={WIDTH} height={HEIGHT} fps={FPS} durationInFrames={1}
      defaultProps={{ recording: undefined as unknown as Recording } satisfies ShotProps}
      calculateMetadata={async () => {
        const response = await fetch(staticFile(`${name}/timeline.json`))
        if (!response.ok) throw new Error(`No recording for ${name}: run \`npm run record -- ${name}\` first.`)
        const recording = await response.json() as Recording
        const duration = toOutputTime(recording.speed, recording.duration)
        return { props: { recording }, durationInFrames: Math.ceil((duration / 1000) * FPS) }
      }} />)}
  </>
}
