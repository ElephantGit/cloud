import type { Conversation } from '@/components/agent-thread/model/types'

/** Tool kinds that can leave the checkout different: edits, and commands that may write files. */
const writing = new Set(['edit', 'delete', 'move', 'execute'])

/**
 * A key that changes whenever the Agent may have written to its checkout: a writing tool call
 * finished, or a turn ended. The workspace panel reads the checkout again when it changes, the way
 * desktop refreshes its diff on the same events, instead of watching the directory.
 */
export function conversationRevision(conversation: Conversation): string {
  let finishedWrites = 0
  for (const turn of conversation.turns) {
    for (const item of turn.items) {
      if (item.kind !== 'tool' || (item.status !== 'completed' && item.status !== 'failed')) continue
      if ((item.toolKind && writing.has(item.toolKind)) || item.content.some((c) => c.type === 'diff')) finishedWrites++
    }
  }
  const last = conversation.turns.at(-1)
  return `${conversation.turns.length}:${last?.status ?? ''}:${finishedWrites}`
}
