import '@fontsource-variable/geist'
import '@fontsource-variable/noto-sans-sc'
import { Composition, staticFile } from 'remotion'
import { shots } from '../shots/index.ts'
import { toOutputTime, type Recording } from '../timeline.ts'
import { Intro } from './Intro.tsx'
import { FPS, HEIGHT, INTRO_FRAMES, OUTRO_FRAMES, WIDTH } from './layout.ts'
import { Outro } from './Outro.tsx'
import { ShotVideo, type ShotProps } from './ShotVideo.tsx'

/** The opening and closing, and one composition per shot, sized from its latest recording in `out/<shot>/`. */
export function Root() {
  return <>
    <Composition id="intro" component={Intro} width={WIDTH} height={HEIGHT} fps={FPS} durationInFrames={INTRO_FRAMES} />
    <Composition id="outro" component={Outro} width={WIDTH} height={HEIGHT} fps={FPS} durationInFrames={OUTRO_FRAMES} />
    {Object.keys(shots).map(name => <Composition key={name} id={name} component={ShotVideo}
      width={WIDTH} height={HEIGHT} fps={FPS} durationInFrames={1}
      defaultProps={{ recording: undefined as unknown as Recording } satisfies ShotProps}
      calculateMetadata={async () => {
        const response = await fetch(staticFile(`${name}/timeline.json`))
        if (!response.ok) throw new Error(`No recording for ${name}: run \`npm run record -- ${name}\` first.`)
        const recording = await response.json() as Recording
        const duration = toOutputTime(recording, recording.duration)
        return { props: { recording }, durationInFrames: Math.ceil((duration / 1000) * FPS) }
      }} />)}
  </>
}
