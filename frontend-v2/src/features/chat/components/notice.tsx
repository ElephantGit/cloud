import { XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { Notice as NoticeData } from '../store'

/**
 * What went wrong with the last send, above the input. A busy workspace names the person's own
 * conversation holding it, with the two ways out: open it, or end its session to free the
 * workspace (that conversation's Agent then starts over).
 */
export function Notice({
  notice,
  onDismiss,
  onOpen,
  onRelease,
}: {
  notice: NoticeData
  onDismiss: () => void
  onOpen: (id: string) => void
  onRelease: (id: string) => void
}) {
  const occupant = notice.kind === 'busy' ? notice.occupant : undefined
  const text =
    notice.kind === 'busy'
      ? occupant
        ? `工作目录正被「${occupant.title}」使用，同一目录同时只能有一个对话。`
        : '工作目录正被其他任务使用，请稍后再试，或换一个 worktree。'
      : notice.kind === 'unavailable'
        ? '后端还不能运行 Agent。'
        : notice.message
  return (
    <div role="alert" className="mb-2 flex items-start gap-2 rounded-lg border bg-muted/40 px-3 py-2 text-xs">
      <p className="min-w-0 flex-1 py-0.5">{text}</p>
      {occupant && (
        <div className="flex shrink-0 gap-1">
          <Button size="xs" variant="outline" onClick={() => onOpen(occupant.id)}>
            打开该对话
          </Button>
          <Button size="xs" variant="outline" onClick={() => onRelease(occupant.id)}>
            结束该对话的会话
          </Button>
        </div>
      )}
      <Button size="icon-xs" variant="ghost" aria-label="关闭提示" onClick={onDismiss}>
        <XIcon />
      </Button>
    </div>
  )
}
