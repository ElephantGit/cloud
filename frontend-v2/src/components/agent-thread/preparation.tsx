import { CircleCheckIcon, LoaderCircleIcon } from 'lucide-react'
import { useState } from 'react'
import {
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from '@/components/ui/trace'
import { cn } from '@/lib/utils'
import type { SessionPreparation, SessionPreparationStage } from './model/wire'

const STAGES: { id: SessionPreparationStage['id']; label: string }[] = [
  { id: 'runtime', label: '启动运行环境' },
  { id: 'workspace', label: '准备工作目录' },
  { id: 'plugin', label: '安装插件' },
  { id: 'agent', label: '启动 Agent' },
  { id: 'response', label: '等待首次输出' },
]

/** Stages this short had nothing to wait for, such as a runtime that was already running. */
const NEGLIGIBLE_MS = 50

const seconds = (ms: number) => `${(Math.max(ms, 0) / 1000).toFixed(1)} 秒`

const durationOf = (stage: SessionPreparationStage) =>
  stage.endedAt ? Date.parse(stage.endedAt) - Date.parse(stage.startedAt) : undefined

/** What a stage's row says on the right, given whether the wait is still going on. */
function stateOf(stage: SessionPreparationStage | undefined, preparing: boolean): string {
  if (!stage) return '等待中'
  const duration = durationOf(stage)
  if (duration === undefined) return preparing ? '进行中' : '未完成'
  return duration < NEGLIGIBLE_MS ? '已就绪' : seconds(duration)
}

/**
 * The wait before a session's first output. While it runs every stage is listed, so the person
 * can see what is left; afterwards it folds to the total and opens into every stage it reached.
 */
export function PreparationRow({ preparation, live }: { preparation: SessionPreparation; live: boolean }) {
  const { stages } = preparation
  const last = stages.at(-1)
  const preparing = live && last !== undefined && !last.endedAt
  const [open, setOpen] = useState<boolean>()
  const total = last?.endedAt ? Date.parse(last.endedAt) - Date.parse(stages[0]!.startedAt) : undefined
  const step = STAGES.findIndex((stage) => stage.id === last?.id)
  const rows = STAGES.flatMap(({ id, label }) => {
    const stage = stages.find((candidate) => candidate.id === id)
    return preparing || stage ? [{ id, label, stage, running: preparing && stage === last }] : []
  })
  return (
    <TraceCollapsible open={open ?? preparing} onOpenChange={setOpen}>
      <TraceCollapsibleTrigger>
        <TraceLine status={preparing ? 'running' : undefined}>
          <TraceIcon>{preparing ? <LoaderCircleIcon className="animate-spin" /> : <CircleCheckIcon />}</TraceIcon>
          <TraceTitle>
            {preparing
              ? `正在${STAGES[step]?.label ?? '准备'}…（第 ${step + 1}/${STAGES.length} 步）`
              : total !== undefined ? `首次输出用时 ${seconds(total)}` : '准备'}
          </TraceTitle>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent>
        <ul className="flex flex-col gap-1">
          {rows.map(({ id, label, stage, running }) => (
            <li
              key={id}
              className={cn('flex justify-between gap-6 tabular-nums', !stage && 'opacity-50', running && 'font-medium text-foreground')}
            >
              <span>
                {label}
                {stage?.reused ? '（已预热）' : ''}
              </span>
              <span>{stateOf(stage, preparing)}</span>
            </li>
          ))}
        </ul>
      </TraceCollapsibleContent>
    </TraceCollapsible>
  )
}
