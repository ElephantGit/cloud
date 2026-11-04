import { useMemo } from 'react'
import { AbsoluteFill } from 'remotion'
import { seeded } from './seeded.ts'

const TILE = 256

/**
 * Fine film grain over the whole frame. Broad pale gradients band badly once H.264 squeezes them to
 * 8 bits; a little noise dithers the steps away. The tile shifts every frame so the grain lives.
 */
export function Grain({ frame, opacity = 0.06 }: { frame: number; opacity?: number }) {
  const tile = useMemo(() => {
    const canvas = document.createElement('canvas')
    canvas.width = TILE
    canvas.height = TILE
    const context = canvas.getContext('2d')!
    const image = context.createImageData(TILE, TILE)
    const random = seeded(3)
    for (let i = 0; i < image.data.length; i += 4) {
      const v = Math.round(random() * 255)
      image.data[i] = v
      image.data[i + 1] = v
      image.data[i + 2] = v
      image.data[i + 3] = 255
    }
    context.putImageData(image, 0, 0)
    return canvas.toDataURL('image/png')
  }, [])
  const random = seeded(frame + 1)
  return <AbsoluteFill style={{
    backgroundImage: `url(${tile})`, backgroundSize: `${TILE}px ${TILE}px`,
    backgroundPosition: `${Math.floor(random() * TILE)}px ${Math.floor(random() * TILE)}px`,
    mixBlendMode: 'overlay', opacity, pointerEvents: 'none',
  }} />
}
