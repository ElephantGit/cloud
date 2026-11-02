import type { Task } from './tasks-api/generated'

/** The task scopes the sidebar offers; each one is a filter over the same team-wide list. */
export type TaskView = 'mine' | 'todo' | 'all'

export const TASK_VIEWS: { id: TaskView; label: string; hint: string }[] = [
  { id: 'mine', label: '我的任务', hint: '指派给我的任务' },
  { id: 'todo', label: '待我处理', hint: '指派给我且未结束的、我建的等着验收的，以及有人 @ 我还没回复的' },
  { id: 'all', label: '全部任务', hint: '工作区里所有任务，全团队可见' },
]

/** The view a URL names; anything else is the whole list. */
export const viewOf = (raw: string | null): TaskView => (TASK_VIEWS.some(v => v.id === raw) ? (raw as TaskView) : 'all')

/** Whether the task's Agent is running a turn right now. */
export const isAgentRunning = (task: Task): boolean => task.activeRun?.state === 'running'

const finished = (task: Task) => task.status === 'done' || task.status === 'cancelled'

/**
 * The tasks of a view, for the person `me`; a squad narrows any view to the tasks it owns.
 * `mentioned` holds the tasks where someone @-mentioned `me` and is still waiting on a reply.
 */
export function filterTasks(tasks: Task[], view: TaskView, me: string, squadId?: string | null, mentioned: ReadonlySet<string> = new Set()): Task[] {
  const scoped = squadId ? tasks.filter(t => t.assignee?.type === 'squad' && t.assignee.id === squadId) : tasks
  switch (view) {
    case 'mine':
      return scoped.filter(t => t.assignee?.type === 'human' && t.assignee.id === me)
    case 'todo':
      return scoped.filter(t => (t.assignee?.type === 'human' && t.assignee.id === me && !finished(t)) || (t.status === 'review' && t.createdBy === me) || mentioned.has(t.id))
    default:
      return scoped
  }
}

/** The board's columns in lifecycle order; cancelled work is hidden unless asked for. */
export const BOARD_COLUMNS = ['pending', 'running', 'review', 'done'] as const

/** Whether the caller may send the Agent a turn now, and why not. */
export function agentReach(task: Task, me: string): { ok: boolean; reason: string } {
  if (task.assignee?.type !== 'agent') return { ok: false, reason: '任务没有指派给 Agent。' }
  if (task.status === 'done' || task.status === 'cancelled') return { ok: false, reason: '任务已结束。' }
  const run = task.activeRun
  if (run?.state === 'running') return { ok: false, reason: 'Agent 正在执行，等这一轮结束。' }
  if (run && run.startedBy !== me) return { ok: false, reason: `当前运行由 ${run.startedByName} 发起，只有发起人能追问；可以在右侧重新执行。` }
  return { ok: true, reason: run ? '作为同一次运行的下一轮发给 Agent。' : '用你的身份开一次新运行。' }
}
