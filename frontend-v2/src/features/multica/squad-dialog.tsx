import { useEffect, useRef, type ReactNode } from 'react'
import { Users, X } from 'lucide-react'
import { Button } from '@/components/ui/button'

/**
 * Modal panel for creating, editing and reading a squad. The native dialog supplies focus
 * containment, Escape and return-to-trigger behavior without extra state.
 */
export function SquadDialog({ title, subtitle, onClose, children }: { title: string; subtitle: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = ref.current!
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
    dialog.showModal()
    dialog.querySelector<HTMLElement>('input, textarea, button')?.focus()
    return () => {
      dialog.close()
      // React may remove the dialog before native focus restoration runs.
      queueMicrotask(() => { if (trigger?.isConnected) trigger.focus() })
    }
  }, [])
  return <dialog ref={ref} className="mc-dialog multica-feature" aria-labelledby="mc-dialog-title" onCancel={event => { event.preventDefault(); onClose() }}>
    <header className="mc-dialog-header">
      <div>
        <span className="mc-eyebrow"><Users size={14} />小队</span>
        <h2 id="mc-dialog-title">{title}</h2>
        <p>{subtitle}</p>
      </div>
      <Button type="button" variant="ghost" size="icon-sm" aria-label="关闭面板" onClick={onClose}><X /></Button>
    </header>
    {children}
  </dialog>
}
