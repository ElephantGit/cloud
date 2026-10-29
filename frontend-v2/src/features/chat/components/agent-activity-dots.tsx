import { cn } from '@/lib/utils'

/**
 * Each cell's row animation, in row-major order. Whole class names, because Tailwind only
 * generates the classes it finds in the source.
 */
const animations = [
  'animate-dot-column-top',
  'animate-dot-column-top',
  'animate-dot-column-top',
  'animate-dot-column-middle',
  'animate-dot-column-middle',
  'animate-dot-column-middle',
  'animate-dot-column-bottom',
  'animate-dot-column-bottom',
  'animate-dot-column-bottom',
]

/** A third of the 1.2s cycle: the keyframes hold 60ms longer, so columns hand off a beat apart. */
const columnDelayMs = 400

/**
 * Desktop's mark for a working agent: a 3x3 grid of squares whose columns light up in turn. The
 * dots take the text colour; show nothing while the agent is idle. Under reduced motion the grid
 * stays still at full strength.
 */
export function AgentActivityDots({ label, className }: { label: string; className?: string }) {
  return (
    <span role="img" aria-label={label} className={cn('grid shrink-0 grid-cols-3 gap-[2.5px]', className)}>
      {animations.map((animation, i) => (
        <span
          key={i}
          className={cn('size-[3px] rounded-[0.5px] bg-current motion-reduce:animate-none', animation)}
          style={{ animationDelay: `${(i % 3) * columnDelayMs}ms` }}
        />
      ))}
    </span>
  )
}
