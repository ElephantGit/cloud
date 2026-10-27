import type {
  WorkflowNodeSession,
  WorkflowNodeSessionExecutionState,
  WorkflowNodeSessionRecord,
} from '@/api/generated.schemas'
import { asJsonRecord } from '@/features/workflows/runtime/json-record'

/**
 * How one stored receipt reads in the inspector; anything unrecognised falls back to `raw`.
 *
 * `user` is the turn the Node echoed for the prompt Cloud dispatched — the attempt's own user
 * input, which every workflow node session records even though the inspector shows no prompt.
 */
export type NodeSessionRowKind = 'user' | 'message' | 'thought' | 'tool' | 'turnEnded' | 'raw'

/** One row of a node attempt's conversation, already ordered oldest first. */
export interface NodeSessionRow {
  /** Receipt sequence, unique within the attempt: the render key and the reader's position. */
  key: number
  kind: NodeSessionRowKind
  /** The row's readable text; a `turnEnded` row carries its stop reason. */
  text: string
  /** True when Cloud stored only part of the record, so `text` is a partial reading. */
  truncated: boolean
}

/** What the inspector shows for one node attempt, from every page loaded so far. */
export interface NodeSessionView {
  /** The attempt every loaded page was pinned to. */
  attempt: number
  /** The read state the newest page reported for the whole attempt. */
  state: WorkflowNodeSessionExecutionState
  rows: NodeSessionRow[]
  /** At least one loaded page observed missing or unreadable receipts. */
  incomplete: boolean
  /** The oldest loaded page has receipts older than it. */
  hasMore: boolean
  /** The node's committed end reason, when its state describes the attempt being read. */
  endReason: string | null
}

/**
 * Projects the loaded pages of a node session walk into what the inspector renders.
 *
 * Pages arrive newest-first, while each page's records arrive oldest-first, so the conversation is
 * rebuilt by reversing only the page order. `hasMore` belongs to the oldest page, because that is the page
 * whose `nextBefore` the walk would continue from; `incomplete` is the union over every page, since
 * a gap one page observed is not something a later page can retract. The node's end reason is shown
 * only when the node's state describes the attempt being read: after a retry it belongs to the new
 * attempt, and presenting it under the old one would attribute a different attempt's ending to it.
 *
 * @param pages - The walk's loaded pages, newest first.
 * @returns The view, or `undefined` before the first page exists.
 */
export function nodeSessionView(
  pages: readonly WorkflowNodeSession[],
): NodeSessionView | undefined {
  const newest = pages[0]
  if (newest === undefined) return undefined
  const oldest = pages[pages.length - 1] ?? newest
  const records = pages.toReversed().flatMap((page) => page.execution.records)
  return {
    attempt: newest.execution.attempt,
    state: newest.execution.state,
    rows: nodeSessionRows(records),
    incomplete: pages.some((page) => page.execution.incomplete),
    hasMore: oldest.execution.hasMore,
    endReason:
      newest.node.attempt === newest.execution.attempt ? newest.node.sessionEndReason : null,
  }
}

/**
 * Projects stored receipts into rows, preserving the order it is given.
 *
 * A receipt Cloud did not shape like the history record it claims to be never reaches here: the
 * reader reports it as `incomplete` instead of inventing a row for it.
 *
 * @param records - The receipts of one page or of a whole attempt, oldest first.
 * @returns One row per receipt.
 */
export function nodeSessionRows(records: readonly WorkflowNodeSessionRecord[]): NodeSessionRow[] {
  return records.map((record) => {
    // Either marker means Cloud kept only part of what it received, so the row says so rather than
    // letting a clipped message read as the complete one.
    const truncated = record.truncated === true || record.visibleTextTruncated === true
    return { key: record.sequence, truncated, ...rowOf(record) }
  })
}

/** The kind and text of one receipt, decided by the event it carries. */
function rowOf(record: WorkflowNodeSessionRecord): Pick<NodeSessionRow, 'kind' | 'text'> {
  const payload = record.record
  if (payload['type'] === 'turnEnded') {
    const reason = payload['stopReason']
    return { kind: 'turnEnded', text: typeof reason === 'string' ? reason : '' }
  }
  const update = asJsonRecord(payload['update'])
  if (payload['type'] === 'update' && update !== undefined) {
    return updateRow(update, record.visibleText)
  }
  return { kind: 'raw', text: JSON.stringify(payload) }
}

/** The kind and text of a session `update`, falling back to the raw event for unknown kinds. */
function updateRow(
  update: Record<string, unknown>,
  visibleText: string | undefined,
): Pick<NodeSessionRow, 'kind' | 'text'> {
  const kind = update['sessionUpdate']
  if (
    kind === 'user_message_chunk' ||
    kind === 'agent_message_chunk' ||
    kind === 'agent_thought_chunk'
  ) {
    // The recorded content is authoritative; `visibleText` is Cloud's bounded projection of the
    // same chunk, used only when the content carries no text block this view can read.
    const text = contentText(update['content']) || (visibleText ?? '')
    if (text !== '') {
      return { kind: chunkKind(kind), text }
    }
  }
  if (kind === 'tool_call' || kind === 'tool_call_update') {
    return { kind: 'tool', text: toolText(update) }
  }
  return { kind: 'raw', text: JSON.stringify(update) }
}

/**
 * The row kind of one text chunk. The user chunk is the prompt Cloud dispatched, echoed back by
 * the Node; the inspector shows it because the conversation is unreadable without the turn it
 * answers, and because it is the attempt's own record of what it was asked.
 *
 * @param sessionUpdate - The chunk's `sessionUpdate` value, already known to be a text chunk.
 * @returns The row kind the chunk reads as.
 */
function chunkKind(sessionUpdate: string): NodeSessionRowKind {
  if (sessionUpdate === 'user_message_chunk') return 'user'
  return sessionUpdate === 'agent_message_chunk' ? 'message' : 'thought'
}

/** The text blocks of an ACP content value; blocks that are not text contribute nothing. */
function contentText(content: unknown): string {
  const blocks: unknown[] = Array.isArray(content) ? content : [content]
  return blocks
    .map((block) => {
      const record = asJsonRecord(block)
      if (record === undefined || record['type'] !== 'text') return ''
      return typeof record['text'] === 'string' ? record['text'] : ''
    })
    .filter((text) => text !== '')
    .join('\n')
}

/** A tool call as one line: its title and the status it last reported, else its raw event. */
function toolText(update: Record<string, unknown>): string {
  const parts = [update['title'], update['status']].filter(
    (part): part is string => typeof part === 'string' && part !== '',
  )
  return parts.length === 0 ? JSON.stringify(update) : parts.join(' · ')
}
