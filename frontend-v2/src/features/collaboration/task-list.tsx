import { Bot } from 'lucide-react'
import type { Task } from './tasks-api/generated'
import { assigneeKindLabel, assigneeLabel, projectLabel, taskStatusLabel } from './task-model'
import { BOARD_COLUMNS, isAgentRunning } from './task-views'

type Projects = { id: string; name: string }[]

/** Who is working on the task: the running Agent and who started it, or the assignee's kind. */
function TaskWorker({ task }: { task: Task }) {
  if (isAgentRunning(task)) {
    return <span className="mc-running" title={`${task.activeRun?.startedByName} 发起`}><i className="mc-pulse" aria-hidden="true" />{task.activeRun?.executorName.split(' / ')[0] ?? 'Agent'} 执行中</span>
  }
  if (task.activeRun?.state === 'idle') return <span className="mc-task-hint"><Bot size={12} />等待追问</span>
  return <span className="mc-task-hint">{task.assignee?.type === 'squad' ? '小队负责，不自动分派' : ''}</span>
}

/** Tasks as rows. */
export function TaskRows({ tasks, projects, onOpen }: { tasks: Task[]; projects: Projects; onOpen: (task: Task) => void }) {
  return <ul className="mc-tasks">{tasks.map(item => <li key={item.id}>
    <button type="button" className="mc-task-row" aria-label={`查看任务 ${item.title}`} onClick={() => onOpen(item)}>
      <span className={`mc-status ${item.status}`}>{taskStatusLabel(item.status)}</span>
      <span className="mc-task-main">
        <strong>{item.title}</strong>
        <small>{projectLabel(projects, item.projectId)} · {assigneeLabel(item)}{item.assignee && `（${assigneeKindLabel(item)}）`}</small>
      </span>
      <TaskWorker task={item} />
    </button>
  </li>)}</ul>
}

/**
 * Tasks as a board, one column per status. Cards do not drag: the lifecycle moves one step at a
 * time and an Agent run owns 进行中 and 待验收, so moves stay on the task's own buttons.
 */
export function TaskBoardColumns({ tasks, projects, showCancelled, onOpen }: { tasks: Task[]; projects: Projects; showCancelled: boolean; onOpen: (task: Task) => void }) {
  const columns: string[] = showCancelled ? [...BOARD_COLUMNS, 'cancelled'] : [...BOARD_COLUMNS]
  return <div className="mc-board" role="list" aria-label="任务看板">
    {columns.map(status => {
      const items = tasks.filter(t => t.status === status)
      return <section key={status} className="mc-column" role="listitem" aria-label={`${taskStatusLabel(status)}列`}>
        <header><span className={`mc-status ${status}`}>{taskStatusLabel(status)}</span><b>{items.length}</b></header>
        <ul>
          {items.map(item => <li key={item.id}>
            <button type="button" className="mc-card-task" aria-label={`查看任务 ${item.title}`} onClick={() => onOpen(item)}>
              <strong>{item.title}</strong>
              <small>{projectLabel(projects, item.projectId)}</small>
              <span className="mc-card-foot"><span>{assigneeLabel(item)}</span><TaskWorker task={item} /></span>
            </button>
          </li>)}
          {items.length === 0 && <li className="mc-column-empty">暂无</li>}
        </ul>
      </section>
    })}
  </div>
}
