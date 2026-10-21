import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import type { ScheduledTaskOccurrence } from '@/api/generated.schemas'
import { useScheduledTaskHistory } from './api'

const OUTCOME_LABELS: Record<string, string> = {
  admitted: '待执行',
  missed: '已错过',
  skipped_busy: '运行中，跳过',
  failed: '失败',
  queued: '排队中',
  dispatched: '已派发',
  running: '运行中',
  completed: '已完成',
  cancelled: '已取消',
}

const OUTCOME_TONE: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  completed: 'default',
  missed: 'secondary',
  skipped_busy: 'outline',
  failed: 'destructive',
  cancelled: 'secondary',
}

function outcomeLabel(outcome: string): string {
  return OUTCOME_LABELS[outcome] ?? outcome
}

function timestamp(value: string): string {
  return new Date(value).toLocaleString()
}

export function ScheduledTaskHistory({
  tid,
  taskId,
  title,
  onClose,
}: {
  tid: string
  taskId: string
  title: string
  onClose: () => void
}) {
  const history = useScheduledTaskHistory(tid, taskId)
  return (
    <Dialog open onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>运行历史 · {title}</DialogTitle>
          <DialogDescription>每次到期点的接纳结果，按时间倒序。</DialogDescription>
        </DialogHeader>
        {history.isPending && <Skeleton className="h-32 w-full" />}
        {history.isError && (
          <p role="alert" className="text-sm text-destructive">
            历史加载失败
          </p>
        )}
        {history.data && history.data.entries.length === 0 && (
          <p className="text-sm text-muted-foreground">尚无运行记录</p>
        )}
        {history.data && history.data.entries.length > 0 && (
          <ul className="space-y-2">
            {history.data.entries.map((entry: ScheduledTaskOccurrence) => (
              <li
                key={entry.scheduledAt}
                className="flex items-center justify-between gap-2 rounded-md border px-3 py-2"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Badge variant={OUTCOME_TONE[entry.outcome] ?? 'secondary'}>
                      {outcomeLabel(entry.outcome)}
                    </Badge>
                    {entry.failureReason && (
                      <span className="truncate text-xs text-muted-foreground">
                        {entry.failureReason}
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    计划 {timestamp(entry.scheduledAt)} · 观察 {timestamp(entry.observedAt)} · 版本{' '}
                    {entry.taskRevision}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
