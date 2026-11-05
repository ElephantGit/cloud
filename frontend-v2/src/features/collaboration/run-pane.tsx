import { Bot } from 'lucide-react'
import { FirstTurnWait } from '@/components/agent-thread/preparation'
import { Thread } from '@/components/agent-thread/thread'
import { formatTime } from './task-model'
import { useRunSession } from './run-session'
import { runEndLabel, runResultLabel, runStateLabel } from './task-model'
import type { TaskRun } from './tasks-api/generated'

// A task run never asks for permission in v2, so answering is never offered.
const noAnswer = () => {}

/**
 * The task's Agent runs, newest selectable, and the selected run's live thread: the same prompts,
 * thinking, tool calls and answers the chat page shows, read by every member of the workspace.
 */
export function RunPane({ tenantId, taskId, runs, selected, onSelect }: {
  tenantId: string
  taskId: string
  runs: TaskRun[]
  /** The run shown; the newest when nothing was picked. */
  selected: TaskRun | undefined
  onSelect: (id: string) => void
}) {
  if (!selected) {
    return <div className="mc-run-empty">
      <Bot size={22} aria-hidden="true" />
      <p>还没有运行。把任务指派给 Agent 后点「开始处理」，或在下面的输入框 @ 它；派给人或小队的任务由负责人或队员点「用 Agent 处理」。</p>
    </div>
  }
  return <div className="mc-run-pane">
    <div className="mc-run-tabs" role="tablist" aria-label="运行记录">
      {runs.map((run, i) => <button key={run.id} type="button" role="tab" aria-selected={run.id === selected.id} className="mc-run-tab" onClick={() => onSelect(run.id)}>
        运行 {i + 1}<span className={`mc-run-dot ${run.state}`} aria-hidden="true" />
      </button>)}
    </div>
    <RunHeader run={selected} />
    <RunThread key={selected.id} tenantId={tenantId} taskId={taskId} run={selected} />
  </div>
}

function RunHeader({ run }: { run: TaskRun }) {
  const outcome = run.state === 'ended' ? runEndLabel(run.endReason) : runStateLabel(run.state)
  const result = run.lastResult ? runResultLabel(run.lastResult) : ''
  return <div className="mc-run-head" aria-label="运行概况">
    <span className={`mc-run-state ${run.state}`}>{outcome}</span>
    <span>{run.executorName}</span>
    <span>{run.scheduled?'定时计划触发 · ':''}{run.startedByName} 发起 · {formatTime(run.createdAt)}</span>
    {result && run.state !== 'running' && <span>最近一轮：{result}{run.failureCode && `（${run.failureCode}）`}</span>}
  </div>
}

function RunThread({ tenantId, taskId, run }: { tenantId: string; taskId: string; run: TaskRun }) {
  const { conversation, refused } = useRunSession(tenantId, taskId, run.id)
  if (refused) return <p role="alert" className="mc-run-note">这次运行的过程读不到了，只能看到上面的结果。</p>
  if (!conversation || conversation.turns.length === 0) {
    if (run.state === 'running') return <div role="status" className="mc-run-thread"><FirstTurnWait preparation={conversation?.preparation} /></div>
    return <p role="status" className="mc-run-note">这次运行没有留下过程记录。</p>
  }
  return <div className="mc-run-thread"><Thread conversation={conversation} onAnswer={noAnswer} /></div>
}
