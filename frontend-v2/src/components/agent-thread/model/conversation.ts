import type * as acp from '@agentclientprotocol/sdk'
import type { ChatEvent, Conversation, Meta, ToolItem, Turn, TurnItem } from './types'

/**
 * Applies one event to a conversation and returns the next one; the input is never mutated, so
 * the result can go straight into a store. `now` stamps the items the event creates or settles.
 */
export function applyEvent(conversation: Conversation, event: ChatEvent, now: number): Conversation {
  switch (event.type) {
    case 'prompt': {
      const turn: Turn = {
        id: event.turnId,
        prompt: { text: event.text, attachments: event.attachments ?? [], plugins: event.plugins ?? [] },
        items: [],
        status: 'streaming',
        startedAt: now,
      }
      return { ...conversation, turns: [...conversation.turns, turn] }
    }
    case 'update':
      if (event.update.sessionUpdate === 'usage_update') {
        const { used, size, cost } = event.update
        return { ...conversation, context: { used, size, cost } }
      }
      return withLiveTurn(conversation, (turn) => applyUpdate(turn, event.update, now))
    case 'permission':
      return { ...conversation, permissions: [...conversation.permissions, event.request] }
    case 'permissionResolved':
      return {
        ...conversation,
        permissions: conversation.permissions.map((p) =>
          p.id === event.id ? { ...p, outcome: { optionId: event.optionId } } : p,
        ),
      }
    case 'turnEnd': {
      const cancelled = event.stopReason === 'cancelled'
      const next = withLiveTurn(conversation, (turn) => ({
        ...settle(turn, now, cancelled ? 'cancelled' : 'completed'),
        status: cancelled ? 'cancelled' : 'completed',
        stopReason: event.stopReason,
        usage: event.usage,
        endedAt: now,
      }))
      return dropOpenPermissions(next)
    }
    case 'turnError': {
      const next = withLiveTurn(conversation, (turn) => ({
        ...settle(turn, now, 'cancelled'),
        status: 'failed',
        error: event.message,
        endedAt: now,
      }))
      return dropOpenPermissions(next)
    }
    case 'model':
    case 'checkpoint': {
      const afterTurn = conversation.turns.length
      const id = `marker-${conversation.markers.length}`
      const marker =
        event.type === 'model'
          ? { kind: 'model' as const, id, afterTurn, model: event.model }
          : { kind: 'checkpoint' as const, id, afterTurn, label: event.label }
      return { ...conversation, markers: [...conversation.markers, marker] }
    }
  }
}

/** Whether the last turn is still receiving agent output. */
export function isResponding(conversation: Conversation): boolean {
  return conversation.turns.at(-1)?.status === 'streaming'
}

/**
 * The child session an item belongs to, from OpenCode's explicit projection in `_meta` (the same
 * key desktop reads). Names and tool titles never imply a child: only this field does.
 */
export function childSessionOf(meta: Meta | undefined): { id: string; parentId: string; title?: string } | undefined {
  const child = meta?.['opencode/child-session']
  if (!child || typeof child !== 'object') return undefined
  const { id, parentID, title } = child as Record<string, unknown>
  if (typeof id !== 'string' || typeof parentID !== 'string') return undefined
  return { id, parentId: parentID, ...(typeof title === 'string' ? { title } : {}) }
}

function withLiveTurn(conversation: Conversation, change: (turn: Turn) => Turn): Conversation {
  const last = conversation.turns.at(-1)
  // Updates outside a live turn (late chunks after cancel, replayed noise) have nowhere to go.
  if (!last || last.status !== 'streaming') return conversation
  return { ...conversation, turns: [...conversation.turns.slice(0, -1), change(last)] }
}

function applyUpdate(turn: Turn, update: acp.SessionUpdate, now: number): Turn {
  switch (update.sessionUpdate) {
    case 'agent_message_chunk':
    case 'agent_thought_chunk':
      return appendChunk(turn, update, now)
    case 'tool_call':
      return upsertTool(turn, update, now)
    case 'tool_call_update':
      return updateTool(turn, update, now)
    case 'plan':
      return replacePlan(turn, update.entries, update._meta ?? undefined)
    default:
      return turn
  }
}

function appendChunk(
  turn: Turn,
  chunk: acp.ContentChunk & { sessionUpdate: 'agent_message_chunk' | 'agent_thought_chunk' },
  now: number,
): Turn {
  const kind = chunk.sessionUpdate === 'agent_message_chunk' ? 'message' : 'thought'
  const meta = chunk._meta ?? undefined
  const source = childSessionOf(meta)?.id

  if (chunk.content.type !== 'text') {
    const items = closeThoughts(turn.items, now, source)
    return { ...turn, items: [...items, { kind: 'content', id: `content-${items.length}`, block: chunk.content, meta }] }
  }

  // Text continues the last item only when it is the same kind of run from the same source;
  // anything in between (a tool, a plan) means the agent moved on and this is a new run.
  const text = chunk.content.text
  const last = turn.items.at(-1)
  if (
    last &&
    (last.kind === 'message' || last.kind === 'thought') &&
    last.kind === kind &&
    childSessionOf(last.meta)?.id === source &&
    (chunk.messageId == null || last.id === `${kind}-${chunk.messageId}`)
  ) {
    return { ...turn, items: [...turn.items.slice(0, -1), { ...last, text: last.text + text }] }
  }
  // Agents often send a blank delta before switching to a tool; opening a run on it would leave
  // an empty block ahead of the work.
  if (text.trim() === '') return turn
  const items = closeThoughts(turn.items, now, source)
  const id = chunk.messageId ? `${kind}-${chunk.messageId}` : `${kind}-${items.length}`
  const item: TurnItem =
    kind === 'message' ? { kind, id, text, meta } : { kind, id, text, startedAt: now, meta }
  return { ...turn, items: [...items, item] }
}

/** A thought ends when its source (the parent, or one child session) starts anything after it. */
function closeThoughts(items: TurnItem[], now: number, source: string | undefined | typeof everySource): TurnItem[] {
  return items.map((item) =>
    item.kind === 'thought' &&
    item.endedAt === undefined &&
    (source === everySource || childSessionOf(item.meta)?.id === source)
      ? { ...item, endedAt: now }
      : item,
  )
}

const everySource = Symbol('every source')

function upsertTool(turn: Turn, call: acp.ToolCall, now: number): Turn {
  const existing = turn.items.findIndex((i) => i.kind === 'tool' && i.id === call.toolCallId)
  const tool: ToolItem = {
    kind: 'tool',
    id: call.toolCallId,
    title: call.title,
    toolKind: call.kind,
    status: call.status ?? 'pending',
    content: call.content ?? [],
    locations: call.locations ?? [],
    rawInput: call.rawInput,
    rawOutput: call.rawOutput,
    startedAt: now,
    meta: call._meta ?? undefined,
  }
  if (existing !== -1) {
    const items = [...turn.items]
    items[existing] = { ...tool, startedAt: (items[existing] as ToolItem).startedAt }
    return { ...turn, items }
  }
  return { ...turn, items: [...closeThoughts(turn.items, now, childSessionOf(tool.meta)?.id), tool] }
}

function updateTool(turn: Turn, update: acp.ToolCallUpdate, now: number): Turn {
  const index = turn.items.findIndex((i) => i.kind === 'tool' && i.id === update.toolCallId)
  if (index === -1) {
    // An update for a call we never saw: ACP allows it, so start the call from what it says.
    return upsertTool(turn, { title: update.title ?? '工具调用', ...stripNulls(update) } as acp.ToolCall, now)
  }
  const tool = turn.items[index] as ToolItem
  const status = update.status ?? tool.status
  const settled = status === 'completed' || status === 'failed'
  const next: ToolItem = {
    ...tool,
    title: update.title ?? tool.title,
    toolKind: update.kind ?? tool.toolKind,
    status,
    content: update.content ?? tool.content,
    locations: update.locations ?? tool.locations,
    rawInput: update.rawInput !== undefined ? update.rawInput : tool.rawInput,
    rawOutput: update.rawOutput !== undefined ? update.rawOutput : tool.rawOutput,
    endedAt: settled ? (tool.endedAt ?? now) : undefined,
    meta: update._meta ? { ...tool.meta, ...update._meta } : tool.meta,
  }
  const items = [...turn.items]
  items[index] = next
  return { ...turn, items }
}

function replacePlan(turn: Turn, entries: acp.PlanEntry[], meta: Meta | undefined): Turn {
  const index = turn.items.findIndex((i) => i.kind === 'plan' && childSessionOf(i.meta)?.id === childSessionOf(meta)?.id)
  if (index === -1) {
    return { ...turn, items: [...turn.items, { kind: 'plan', id: `plan-${turn.items.length}`, entries, meta }] }
  }
  const items = [...turn.items]
  items[index] = { ...(items[index] as Extract<TurnItem, { kind: 'plan' }>), entries }
  return { ...turn, items }
}

/** Closes what the agent left open when a turn ends: thoughts stop, unreported tools take `status`. */
function settle(turn: Turn, now: number, status: 'completed' | 'cancelled'): Turn {
  return {
    ...turn,
    items: closeThoughts(turn.items, now, everySource).map((item) =>
      item.kind === 'tool' && (item.status === 'pending' || item.status === 'in_progress')
        ? { ...item, status, endedAt: now }
        : item,
    ),
  }
}

/** A turn that ended no longer waits on anything; its unanswered requests read as cancelled. */
function dropOpenPermissions(conversation: Conversation): Conversation {
  return {
    ...conversation,
    permissions: conversation.permissions.map((p) => (p.outcome ? p : { ...p, outcome: { cancelled: true } })),
  }
}

function stripNulls<T extends object>(value: T): { [K in keyof T]: Exclude<T[K], null> } {
  return Object.fromEntries(Object.entries(value).filter(([, v]) => v !== null)) as never
}
