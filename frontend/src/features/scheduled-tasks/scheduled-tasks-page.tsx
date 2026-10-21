import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  scheduledTaskFailure,
  scheduleSummary,
  useDeleteScheduledTask,
  useRevokeScheduledTaskConsent,
  useScheduledTasks,
  useSetScheduledTaskEnabled,
  type ScheduledTask,
} from './api'
import { ScheduledTaskForm } from './scheduled-task-form'
import { ScheduledTaskHistory } from './scheduled-task-history'

function ConfirmButton({
  label,
  confirmLabel,
  pending,
  onConfirm,
}: {
  label: string
  confirmLabel: string
  pending: boolean
  onConfirm: () => void
}) {
  const [confirming, setConfirming] = useState(false)
  const variant = confirming ? 'destructive' : 'ghost'
  return (
    <Button
      type="button"
      size="sm"
      variant={variant}
      disabled={pending}
      onClick={() => (confirming ? onConfirm() : setConfirming(true))}
      onBlur={() => setConfirming(false)}
    >
      {confirming ? confirmLabel : label}
    </Button>
  )
}

function ScheduledTaskCard({
  tid,
  task,
  onEdit,
  onHistory,
}: {
  tid: string
  task: ScheduledTask
  onEdit: () => void
  onHistory: () => void
}) {
  const remove = useDeleteScheduledTask(tid)
  const revoke = useRevokeScheduledTaskConsent(tid)
  const toggle = useSetScheduledTaskEnabled(tid, task)
  const failure = toggle.error ?? remove.error ?? revoke.error

  return (
    <article aria-label={task.name} className="space-y-3 rounded-lg border p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="font-medium">{task.name}</h3>
        <Badge variant={task.enabled ? 'default' : 'secondary'}>
          {task.enabled ? '已启用' : '已停用'}
        </Badge>
      </div>
      <p className="text-sm text-muted-foreground">
        {scheduleSummary(task.schedule)} · {task.timezone}
      </p>
      {task.disabledReason && (
        <p className="text-xs text-muted-foreground">停用原因：{task.disabledReason}</p>
      )}
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">启用</span>
        <Switch
          checked={task.enabled}
          disabled={toggle.isPending}
          onCheckedChange={(next) => toggle.mutate(next)}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" onClick={onEdit}>
          编辑
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={onHistory}>
          历史
        </Button>
        {task.unattendedPolicy && (
          <ConfirmButton
            label="撤销授权"
            confirmLabel="确认撤销授权"
            pending={revoke.isPending}
            onConfirm={() => revoke.mutate(task.id)}
          />
        )}
        <ConfirmButton
          label="删除"
          confirmLabel="确认删除"
          pending={remove.isPending}
          onConfirm={() => remove.mutate(task.id)}
        />
      </div>
      {failure && (
        <p role="alert" className="text-xs text-destructive">
          {scheduledTaskFailure(failure)}
        </p>
      )}
    </article>
  )
}

export function ScheduledTasksPage({ slug }: { slug: string }) {
  const tasks = useScheduledTasks(slug)
  const [editor, setEditor] = useState<{ kind: 'new' } | { kind: 'existing'; id: string } | null>(
    null,
  )
  const [historyId, setHistoryId] = useState<string | null>(null)
  const editing =
    editor?.kind === 'existing' ? tasks.data?.tasks.find((t) => t.id === editor.id) : undefined
  const historyTask = historyId ? tasks.data?.tasks.find((t) => t.id === historyId) : undefined

  return (
    <div className="mx-auto max-w-2xl space-y-4 p-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">定时任务</h2>
          <p className="text-sm text-muted-foreground">
            到达计划时刻时在绑定 issue 上自动发起一次无人值守运行。
          </p>
        </div>
        <Button type="button" onClick={() => setEditor({ kind: 'new' })}>
          新建定时任务
        </Button>
      </div>

      {tasks.isError && (
        <p role="alert" className="text-sm text-destructive">
          定时任务加载失败
        </p>
      )}
      {tasks.isPending && <Skeleton className="h-24 w-full" />}
      {tasks.data && tasks.data.tasks.length === 0 && (
        <p className="text-sm text-muted-foreground">还没有定时任务</p>
      )}

      {editor !== null && (
        <ScheduledTaskForm
          key={editing ? `${editing.id}:${editing.version}` : 'new'}
          tid={slug}
          task={editing}
          onClose={() => setEditor(null)}
        />
      )}

      {historyTask && (
        <ScheduledTaskHistory
          tid={slug}
          taskId={historyTask.id}
          title={historyTask.name}
          onClose={() => setHistoryId(null)}
        />
      )}

      {tasks.data?.tasks.map((task) => (
        <ScheduledTaskCard
          key={task.id}
          tid={slug}
          task={task}
          onEdit={() => setEditor({ kind: 'existing', id: task.id })}
          onHistory={() => setHistoryId(task.id)}
        />
      ))}
    </div>
  )
}
