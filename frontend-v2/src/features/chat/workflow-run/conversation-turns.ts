import type { GraphWorkflowNodeStatus, WorkflowNodeConversationItem } from '@/lib/workflows/runtime'
import type { Conversation, Turn, TurnItem } from '../model/types'

const time = (iso: string) => new Date(iso).getTime()

/**
 * Reads an Agent node's conversation as the chat page's conversation, so the node session renders
 * with the chat page's own turns. Each message sent to the Agent opens a turn; the Agent's replies,
 * thoughts and tool calls that follow belong to it. The last turn is still streaming while the
 * node works, and ends the way the node ended.
 */
export function nodeConversationTurns(items: readonly WorkflowNodeConversationItem[], nodeStatus: GraphWorkflowNodeStatus, errorMessage?: string): Conversation {
  const turns: Turn[] = []
  const open = (id: string, text: string, at: number): Turn => {
    const turn: Turn = { id, prompt: { text, attachments: [], plugins: [] }, items: [], status: 'completed', startedAt: at, endedAt: at }
    turns.push(turn)
    return turn
  }
  for (const item of items) {
    const at = time(item.createdAt)
    if (item.kind === 'message' && item.role === 'user') {
      open(item.id, item.markdown, at)
      continue
    }
    // Output that arrives before anything was sent still needs a turn to live in.
    const turn = turns.at(-1) ?? open(`${item.id}-turn`, '', at)
    turn.items.push(turnItem(item))
    turn.endedAt = Math.max(turn.endedAt ?? at, time(item.updatedAt))
  }
  const last = turns.at(-1)
  if (last) {
    const status = lastTurnStatus(nodeStatus)
    last.status = status
    if (status === 'streaming') delete last.endedAt
    if (status === 'failed') last.error = errorMessage
  }
  return { turns, markers: [], permissions: [] }
}

function lastTurnStatus(nodeStatus: GraphWorkflowNodeStatus): Turn['status'] {
  switch (nodeStatus) {
    case 'running':
    case 'awaiting_input':
    case 'retry_waiting':
      return 'streaming'
    case 'failed':
      return 'failed'
    case 'cancelled':
      return 'cancelled'
    default:
      return 'completed'
  }
}

function turnItem(item: WorkflowNodeConversationItem): TurnItem {
  if (item.kind === 'message') return { kind: 'message', id: item.id, text: item.markdown }
  const startedAt = time(item.createdAt)
  const endedAt = item.status === 'complete' ? time(item.updatedAt) : undefined
  if (item.activityKind === 'thought') return { kind: 'thought', id: item.id, text: item.detail ?? item.summary, startedAt, endedAt }
  return {
    kind: 'tool',
    id: item.id,
    title: item.summary,
    toolKind: 'other',
    status: item.status === 'complete' ? 'completed' : 'in_progress',
    content: item.detail ? [{ type: 'content', content: { type: 'text', text: item.detail } }] : [],
    locations: [],
    startedAt,
    endedAt,
  }
}
