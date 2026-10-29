import { useLayoutEffect, useRef, useState } from 'react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/**
 * One line of text that fades out where it overflows its box, with the full text in a tooltip
 * while it is cut off.
 */
export function FadeText({ text, className }: { text: string; className?: string }) {
  const ref = useRef<HTMLSpanElement>(null)
  const [clipped, setClipped] = useState(false)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const check = () => setClipped(el.scrollWidth > el.clientWidth)
    check()
    // jsdom has no ResizeObserver; there nothing ever overflows anyway.
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(check)
    observer.observe(el)
    return () => observer.disconnect()
  }, [text])
  return (
    <Tooltip disabled={!clipped}>
      <TooltipTrigger
        delay={400}
        render={
          <span
            ref={ref}
            className={cn(
              'min-w-0 overflow-hidden whitespace-nowrap',
              clipped && '[mask-image:linear-gradient(to_right,black_calc(100%-2rem),transparent)]',
              className,
            )}
          />
        }
      >
        {text}
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-sm break-all">
        {text}
      </TooltipContent>
    </Tooltip>
  )
}
