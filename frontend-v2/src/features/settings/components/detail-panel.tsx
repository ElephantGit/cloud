import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/** The dialog primitive owns focus trapping, Escape and return focus; only its layout changes. */
export function DetailPanel({
  title,
  description,
  onClose,
  children,
}: {
  title: string
  description: string
  onClose: () => void
  children: ReactNode
}) {
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent showCloseButton={false} className="settings-detail">
        <DialogHeader className="shrink-0 border-b p-6 pr-14">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogClose
          render={
            <Button
              variant="ghost"
              size="icon"
              className="absolute right-4 top-4"
              aria-label="关闭详情"
            />
          }
        >
          <X />
        </DialogClose>
        <div className="settings-detail-body">{children}</div>
      </DialogContent>
    </Dialog>
  )
}
