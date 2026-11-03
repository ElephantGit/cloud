/** Create an initial task without mutating existing tasks. */
export function addTask(tasks, title) {
  if (!title.trim()) throw new Error('title is required')
  return [...tasks, { id: tasks.length + 1, title: title.trim() }]
}
