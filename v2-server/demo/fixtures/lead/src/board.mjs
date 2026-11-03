export const STATUSES = ['pending', 'running', 'review', 'done']

const NEXT = { pending: 'running', running: 'review', review: 'done' }

/** Groups tasks into one column per status, keeping each column in input order. */
export function columns(tasks) {
  const board = Object.fromEntries(STATUSES.map((status) => [status, []]))
  for (const task of tasks) {
    if (board[task.status]) board[task.status].push(task)
  }
  return board
}

/** Returns a copy of the task moved to the next status; done and cancelled tasks cannot move. */
export function advance(task) {
  const next = NEXT[task.status]
  if (!next) throw new Error(`cannot advance a ${task.status} task`)
  return { ...task, status: next }
}

/** Returns a copy of the task cancelled; a finished task stays finished. */
export function cancel(task) {
  if (task.status === 'done' || task.status === 'cancelled') throw new Error(`cannot cancel a ${task.status} task`)
  return { ...task, status: 'cancelled' }
}
