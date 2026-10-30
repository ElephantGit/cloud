import type { KeyboardEvent, PointerEvent } from 'react'

/** A page's left column: its width, and whether it shows or is folded away. */
export interface SidebarLayout {
  width: number
  open: boolean
}

const minWidth = 200
const maxWidth = 480
/** Dragging the edge this close to the icon rail folds the column away. */
const foldBelow = 120

/**
 * A left column's right edge: drag (or arrow keys) to resize, past the rail to fold, Enter to
 * toggle. It stays in place while the column is folded, so dragging it out opens the column again.
 * The page owns the layout and receives each change as a partial update.
 */
export function SidebarResizeHandle({
  layout,
  onChange,
  label,
}: {
  layout: SidebarLayout
  onChange: (layout: Partial<SidebarLayout>) => void
  label: string
}) {
  const { width, open } = layout
  const resizeTo = (next: number) =>
    onChange(next < foldBelow ? { open: false } : { open: true, width: Math.min(maxWidth, Math.max(minWidth, next)) })

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return
    e.preventDefault()
    const handle = e.currentTarget
    const startX = e.clientX
    const start = open ? width : 0
    handle.setPointerCapture(e.pointerId)
    document.body.style.cursor = 'col-resize'
    const move = (ev: globalThis.PointerEvent) => resizeTo(start + ev.clientX - startX)
    const stop = () => {
      document.body.style.cursor = ''
      handle.removeEventListener('pointermove', move)
      handle.removeEventListener('pointerup', stop)
      handle.removeEventListener('pointercancel', stop)
    }
    handle.addEventListener('pointermove', move)
    handle.addEventListener('pointerup', stop)
    handle.addEventListener('pointercancel', stop)
  }
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = { ArrowLeft: -16, ArrowRight: 16 }[e.key]
    if (step) resizeTo(open ? (width === minWidth && step < 0 ? 0 : width + step) : minWidth)
    else if (e.key === 'Enter') onChange({ open: !open })
    else return
    e.preventDefault()
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={maxWidth}
      aria-valuenow={open ? width : 0}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      className="relative z-10 -mx-1 w-2 shrink-0 cursor-col-resize touch-none outline-none after:absolute after:inset-y-0 after:left-1/2 after:w-0.5 after:-translate-x-1/2 after:transition-colors hover:after:bg-primary/40 focus-visible:after:bg-primary active:after:bg-primary/60"
    />
  )
}
