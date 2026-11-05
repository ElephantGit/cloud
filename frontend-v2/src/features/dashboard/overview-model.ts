import type { Task, TaskRun, TaskTimeline } from '@/lib/tasks-api/generated'

export interface OverviewSnapshot {
  tasks: Task[]
  timelines: Record<string, TaskTimeline>
  fetchedAt: string
}

/** Current task ownership is counted once; squad work is never duplicated across its members. */
export function overviewModel(snapshot: OverviewSnapshot, now = new Date()) {
  const counts = { pending: 0, running: 0, review: 0, done: 0, cancelled: 0 }
  const owners = new Map<string, { id: string; name: string; kind: string; pending: number; running: number; review: number; done: number }>()
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  start.setDate(start.getDate() - (start.getDay() + 6) % 7)
  const weekRuns: TaskRun[] = []
  const outputs: { task: Task; run: TaskRun }[] = []
  const days = Array.from({ length: 7 }, (_, index) => {
    const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 6 + index)
    return { date, label: `${date.getMonth() + 1}/${date.getDate()}`, count: 0 }
  })
  for (const task of snapshot.tasks) {
    counts[task.status]++
    if (task.status !== 'cancelled') {
      const key = task.assignee ? `${task.assignee.type}:${task.assignee.id}` : 'unassigned'
      const owner = owners.get(key) ?? { id: key, name: task.assignee?.displayName || '未指派', kind: task.assignee?.type || 'unassigned', pending: 0, running: 0, review: 0, done: 0 }
      owner[task.status]++
      owners.set(key, owner)
    }
    const updated = new Date(task.updatedAt)
    const day = days.find(day => day.date.toDateString() === updated.toDateString())
    if (day) day.count++
    const runs = [...(snapshot.timelines[task.id]?.runs ?? [])].sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
    weekRuns.push(...runs.filter(run => Date.parse(run.createdAt) >= start.getTime() && Date.parse(run.createdAt) <= now.getTime()))
    const successful = runs[0]?.lastResult === 'completed' && runs[0].state !== 'running' ? runs[0] : undefined
    if (successful && (task.status === 'review' || task.status === 'done')) outputs.push({ task, run: successful })
  }
  outputs.sort((a, b) => Date.parse(b.task.updatedAt) - Date.parse(a.task.updatedAt))
  return { counts, owners: [...owners.values()], days, weekRuns, outputs, max: Math.max(1, ...days.map(day => day.count)) }
}
