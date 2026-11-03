import { AtSign, Bot, UserRound, Users } from 'lucide-react'
import type { PendingMention, Task } from './tasks-api/generated'
import { assigneeLabel, projectLabel, taskStatusLabel } from './task-model'
import { BOARD_COLUMNS, isAgentRunning } from './task-views'

type Projects = { id: string; name: string }[]
type Mentions = ReadonlyMap<string, PendingMention>

const ASSIGNEE_ICONS = { human: UserRound, agent: Bot, squad: Users }

/** Who owns the task, with an icon for a person, an Agent or a squad; 未指派 otherwise. */
function TaskAssignee({ task }: { task: Task }) {
  const Icon = task.assignee ? ASSIGNEE_ICONS[task.assignee.type] : null
  return <span className={task.assignee ? 'mc-assignee-tag' : 'mc-assignee-tag none'} title="负责">
    {Icon && <Icon size={12} aria-hidden="true" />}{assigneeLabel(task)}
  </span>
}

/** What is happening on the task: the running Agent and who started it, or a run waiting on its starter. */
function TaskWorker({ task }: { task: Task }) {
  const run = task.activeRun
  if (isAgentRunning(task)) {
    return <span className="mc-running"><i className="mc-pulse" aria-hidden="true" />{run?.startedByName} 发起 · 执行中</span>
  }
  if (run?.state === 'idle') return <span className="mc-task-hint"><Bot size={12} />{run.startedByName} 发起 · 等待追问</span>
  return task.assignee?.type === 'squad' ? <span className="mc-task-hint">小队负责，不自动分派</span> : null
}

/** Someone @-mentioned the viewer here and is waiting on their reply. */
function MentionTag({ mention }: { mention?: PendingMention }) {
  if (!mention) return null
  return <span className="mc-mention-tag" title="回复后提醒消失"><AtSign size={11} aria-hidden="true" />{mention.authorName} @ 了你</span>
}

/** Tasks as rows. */
export function TaskRows({ tasks, projects, mentions, onOpen }: { tasks: Task[]; projects: Projects; mentions: Mentions; onOpen: (task: Task) => void }) {
  return <ul className="mc-tasks">{tasks.map(item => <li key={item.id}>
    <button type="button" className="mc-task-row" aria-label={`查看任务 ${item.title}`} onClick={() => onOpen(item)}>
      <span className={`mc-status ${item.status}`}>{taskStatusLabel(item.status)}</span>
      <span className="mc-task-main">
        <strong>{item.title}</strong>
        <small>{projectLabel(projects, item.projectId)} · {item.createdByName} 创建</small>
      </span>
      <MentionTag mention={mentions.get(item.id)} />
      <TaskWorker task={item} />
      <TaskAssignee task={item} />
    </button>
  </li>)}</ul>
}

/**
 * Tasks as a board, one column per status. Cards do not drag: the lifecycle moves one step at a
 * time and an Agent run owns 进行中 and 待验收, so moves stay on the task's own buttons.
 */
export function TaskBoardColumns({ tasks, projects, mentions, showCancelled, onOpen }: { tasks: Task[]; projects: Projects; mentions: Mentions; showCancelled: boolean; onOpen: (task: Task) => void }) {
  const columns: string[] = showCancelled ? [...BOARD_COLUMNS, 'cancelled'] : [...BOARD_COLUMNS]
  return <div className="mc-board" role="list" aria-label="任务看板">
    {columns.map(status => {
      const items = tasks.filter(t => t.status === status)
      return <section key={status} className="mc-column" role="listitem" aria-label={`${taskStatusLabel(status)}列`}>
        <header><span className={`mc-status ${status}`}>{taskStatusLabel(status)}</span><b>{items.length}</b></header>
        <ul>
          {items.map(item => <li key={item.id}>
            <button type="button" className="mc-card-task" aria-label={`查看任务 ${item.title}`} onClick={() => onOpen(item)}>
              <MentionTag mention={mentions.get(item.id)} />
              <strong>{item.title}</strong>
              <small>{projectLabel(projects, item.projectId)} · {item.createdByName} 创建</small>
              <TaskAssignee task={item} />
              <TaskWorker task={item} />
            </button>
          </li>)}
          {items.length === 0 && <li className="mc-column-empty">暂无</li>}
        </ul>
      </section>
    })}
  </div>
}
