import type { ToolItem } from '../model/types'

/** The trace status a tool shows: running only while its turn can still settle it. */
export function toolLineStatus(tool: ToolItem, live: boolean): 'running' | 'error' | 'pending' | undefined {
  if (tool.status === 'failed') return 'error'
  if (tool.status === 'pending' || tool.status === 'in_progress') return live ? 'running' : 'pending'
  if (tool.status === 'cancelled') return 'pending'
}

/** Whether text looks like a JavaScript-style stack: an error line followed by `at` frames. */
export function isStackTrace(text: string): boolean {
  return /\n\s+at .+/.test(text)
}
