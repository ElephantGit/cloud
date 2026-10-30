import { useImperativeHandle, useRef, type Ref } from 'react'
import { cn } from '@/lib/utils'

/** The part of desktop's composer handle the HITL composer uses. */
export interface ComposerEditorHandle {
  getText: () => string
  focus: (options?: { preventScroll?: boolean }) => void
}

interface ComposerEditorProps {
  ref?: Ref<ComposerEditorHandle>
  id?: string
  initialText?: string
  placeholder?: string
  disabled?: boolean
  /** "submit": Enter sends and Shift+Enter breaks the line; "newline": Enter breaks the line. */
  enterKey?: 'submit' | 'newline'
  ariaLabel?: string
  className?: string
  onSubmit: () => void
  onTextChange?: (text: string) => void
}

/**
 * A plain-text stand-in for desktop's TipTap composer in HITL answers. Answers here are plain text,
 * so file chips and prompt tokens are left out; the box grows with its content.
 */
export function ComposerEditor({ ref, id, initialText = '', placeholder, disabled, enterKey = 'submit', ariaLabel, className, onSubmit, onTextChange }: ComposerEditorProps) {
  const textarea = useRef<HTMLTextAreaElement>(null)
  useImperativeHandle(ref, () => ({
    getText: () => textarea.current?.value ?? '',
    focus: (options) => textarea.current?.focus(options),
  }))
  return (
    <textarea
      ref={textarea}
      id={id}
      rows={1}
      defaultValue={initialText}
      placeholder={placeholder}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn('field-sizing-content max-h-48 min-h-9 w-full resize-none bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted-foreground disabled:opacity-50', className)}
      onChange={(e) => onTextChange?.(e.currentTarget.value)}
      onKeyDown={(e) => {
        if (enterKey === 'submit' && e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
          e.preventDefault()
          onSubmit()
        }
      }}
    />
  )
}
