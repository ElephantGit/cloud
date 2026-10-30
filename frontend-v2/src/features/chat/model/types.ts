import type * as acp from '@agentclientprotocol/sdk'

/** Provider metadata kept on an item without being interpreted, except where a renderer needs it. */
export type Meta = Record<string, unknown>

/** One assembled run of agent text. Consecutive chunks with the same message id grow it. */
export interface MessageItem {
  kind: 'message'
  id: string
  text: string
  meta?: Meta
}

/** The agent's visible reasoning, rendered secondary to the answer. */
export interface ThoughtItem {
  kind: 'thought'
  id: string
  text: string
  startedAt: number
  /** Set when the next item arrives or the turn ends, so the row can say how long it thought. */
  endedAt?: number
  meta?: Meta
}

/** The latest plan snapshot for a turn; ACP sends the whole plan each time. */
export interface PlanItem {
  kind: 'plan'
  id: string
  entries: acp.PlanEntry[]
  meta?: Meta
}

/**
 * ACP has no status for a tool whose outcome was never reported; a cancelled or broken turn
 * leaves exactly that. `cancelled` keeps it apart from a tool the agent said failed.
 */
export type ToolStatus = acp.ToolCallStatus | 'cancelled'

/** One tool call with its latest ACP lifecycle fields. */
export interface ToolItem {
  kind: 'tool'
  id: string
  title: string
  toolKind?: acp.ToolKind
  status: ToolStatus
  content: acp.ToolCallContent[]
  locations: acp.ToolCallLocation[]
  rawInput?: unknown
  rawOutput?: unknown
  startedAt: number
  endedAt?: number
  meta?: Meta
}

/** A non-text block the agent sent in its message stream (resource link, image, ...). */
export interface ContentItem {
  kind: 'content'
  id: string
  block: Exclude<acp.ContentBlock, { type: 'text' }>
  meta?: Meta
}

export type TurnItem = MessageItem | ThoughtItem | PlanItem | ToolItem | ContentItem

export type TurnStatus = 'streaming' | 'completed' | 'cancelled' | 'failed'

/** A file the person attached to a prompt. `url` is a local object URL or a data URL. */
export interface Attachment {
  id: string
  name: string
  mimeType: string
  size: number
  url?: string
}

/** A plugin applied to one prompt. */
export interface PluginRef {
  id: string
  name: string
}

/** One prompt and everything the agent did in answer to it. */
export interface Turn {
  id: string
  prompt: { text: string; attachments: Attachment[]; plugins: PluginRef[] }
  items: TurnItem[]
  status: TurnStatus
  stopReason?: acp.StopReason
  error?: string
  /** Token usage the agent reported for this turn, when it reports any. */
  usage?: acp.Usage
  startedAt: number
  endedAt?: number
}

/** A permission the agent is waiting on. Rendered inline at the tool call it gates. */
export interface PermissionRequest {
  id: string
  toolCall: acp.ToolCallUpdate
  options: acp.PermissionOption[]
  /** Set once answered; the request stays visible as a record of the decision. */
  outcome?: { optionId: string } | { cancelled: true }
}

/** Why an Agent session behind a conversation ended; the next prompt starts a fresh one. */
export type SessionEndReason = 'stopped' | 'idle' | 'deleted' | 'interrupted'

/**
 * Something between turns: a model switch, a restorable checkpoint, or the end of an Agent
 * session, after which the Agent no longer remembers the turns above.
 */
export type Marker =
  | { kind: 'model'; id: string; afterTurn: number; model: string }
  | { kind: 'checkpoint'; id: string; afterTurn: number; label: string }
  | { kind: 'session'; id: string; afterTurn: number; reason: SessionEndReason }

/** The context window as last reported by the agent; not part of the durable history. */
export interface ContextUsage {
  used: number
  size: number
  cost?: acp.Cost | null
}

export interface Conversation {
  turns: Turn[]
  markers: Marker[]
  permissions: PermissionRequest[]
  context?: ContextUsage
}

/**
 * Everything that changes a conversation. ACP session updates pass through untouched; the rest
 * are the turn boundaries and requests that ACP carries outside `session/update`.
 */
export type ChatEvent =
  | { type: 'prompt'; turnId: string; text: string; attachments?: Attachment[]; plugins?: PluginRef[] }
  | { type: 'update'; update: acp.SessionUpdate }
  | { type: 'permission'; request: Omit<PermissionRequest, 'outcome'> }
  | { type: 'permissionResolved'; id: string; optionId: string }
  | { type: 'turnEnd'; stopReason: acp.StopReason; usage?: acp.Usage }
  | { type: 'turnError'; message: string }
  | { type: 'model'; model: string }
  | { type: 'checkpoint'; label: string }

export const emptyConversation: Conversation = { turns: [], markers: [], permissions: [] }
