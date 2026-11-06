/** Turns the board's task list into CSV for the weekly report. */
export function toCsv(tasks) {
  const header = 'id,title,status,assignee'
  const rows = tasks.map((task) => [task.id, task.title, task.status, task.assignee ?? ''].map(escape).join(','))
  return [header, ...rows].join('\n')
}

function escape(value) {
  const text = String(value)
  return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}
