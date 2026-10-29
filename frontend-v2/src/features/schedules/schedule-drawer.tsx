import { useEffect, useRef, type ReactNode } from 'react'
import { CalendarClock, X } from 'lucide-react'
import { Button } from '@/components/ui/button'

/** Native modal supplies focus containment, Escape and return-to-trigger behavior. */
export function ScheduleDrawer({ title, subtitle, onClose, children }: { title: string; subtitle?: string; onClose: () => void; children: ReactNode }) {
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
  return <dialog ref={ref} className="sp-dialog schedules-feature" aria-labelledby="sp-dialog-title" onCancel={event => { event.preventDefault(); onClose() }}>
    <header className="sp-dialog-header"><div><span className="sp-eyebrow"><CalendarClock size={14} />定时任务</span><h2 id="sp-dialog-title">{title}</h2>{subtitle && <p>{subtitle}</p>}</div><Button type="button" variant="ghost" size="icon-sm" aria-label="关闭面板" onClick={onClose}><X /></Button></header>
    {children}
  </dialog>
}
