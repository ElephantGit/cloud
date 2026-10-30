import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { Notice as NoticeData } from '../store'

/** What the page says about each kind of failed send. */
const text = (notice: NoticeData) => {
  switch (notice.kind) {
    case 'running':
      return '上一轮还没结束，这条消息会在它结束后发送。'
    case 'ended':
      return '这个对话已结束，不能继续追问。请新开对话。'
    case 'unavailable':
      return '后端还不能运行 Agent。'
    case 'error':
      return notice.message
  }
}

/** What went wrong with the last send, above the input. */
export function Notice({ notice, onDismiss }: { notice: NoticeData; onDismiss: () => void }) {
  return (
    <div role="alert" className="mb-2 flex items-start gap-2 rounded-lg border bg-muted/40 px-3 py-2 text-xs">
      <p className="min-w-0 flex-1 py-0.5">{text(notice)}</p>
      <Button size="icon-xs" variant="ghost" aria-label="关闭提示" onClick={onDismiss}>
        <XIcon />
      </Button>
    </div>
  )
}
