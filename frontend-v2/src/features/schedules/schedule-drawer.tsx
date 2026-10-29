import { useEffect, useRef, type ReactNode } from 'react'
import { X } from 'lucide-react'

/** Native modal supplies focus containment, Escape and return-to-trigger behavior. */
export function ScheduleDrawer({ title, subtitle, onClose, children, wide = false }: { title: string; subtitle?: string; onClose: () => void; children: ReactNode; wide?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = ref.current!
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
    dialog.showModal()
    dialog.querySelector<HTMLInputElement>('input')?.focus()
    return () => {
      dialog.close()
      // React may remove the dialog before native focus restoration runs.
      queueMicrotask(() => { if (trigger?.isConnected) trigger.focus() })
    }
  }, [])
  return <dialog ref={ref} className={`sp-dialog schedules-feature ${wide ? 'sp-dialog-wide' : ''}`} aria-labelledby="sp-dialog-title" onCancel={event => { event.preventDefault(); onClose() }}>
    <header className="sp-dialog-header"><div><span className="sp-eyebrow">SCHEDULE</span><h2 id="sp-dialog-title">{title}</h2>{subtitle && <p>{subtitle}</p>}</div><button type="button" className="sp-icon-button" aria-label="关闭面板" onClick={onClose}><X size={19} /></button></header>
    {children}
  </dialog>
}
