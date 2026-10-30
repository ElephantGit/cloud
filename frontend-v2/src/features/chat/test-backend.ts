import type { InternalAxiosRequestConfig } from 'axios'
import { demoUser, fakeBackend } from '@/test/backend'
import type { ChatConversation, ChatEvent, ChatExecutor, ChatHistoryRecord } from './api/generated'

export const tenantId = '00000000-0000-4000-8000-0000000000aa'

const at = '2026-10-30T09:00:00Z'
const project = (id: string, name: string) => ({
  id, name, createdAt: at, credentialRefId: null, defaultBranch: 'main', deletedAt: null, lifecycle: 'active',
  ownerUserId: demoUser.id, repositoryUrl: `https://git.example/${name}`, spaceId: 's', tenantId, version: 1,
})

/** One followed event stream; tests push events into it as the backend would. */
export class FakeEventSource {
  static all: FakeEventSource[] = []
  onmessage: ((m: MessageEvent<string>) => void) | null = null
  closed = false
  private listeners: Record<string, (() => void)[]> = {}
  url: string
  constructor(url: string) {
    this.url = url
    FakeEventSource.all.push(this)
  }
  addEventListener(name: string, fn: () => void) {
    ;(this.listeners[name] ??= []).push(fn)
  }
  close() {
    this.closed = true
  }
  emit(event: Partial<ChatEvent>) {
    this.onmessage?.({ data: JSON.stringify({ sequence: 1, occurredAt: at, turnId: 't', ...event }) } as MessageEvent<string>)
  }
  fire(name: 'end' | 'failure') {
    for (const fn of this.listeners[name] ?? []) fn()
  }
  /** The stream following session `sid`. */
  static of(sid: string) {
    const s = FakeEventSource.all.filter((x) => x.url.endsWith(`/sessions/${sid}/events`)).at(-1)
    if (!s) throw new Error(`no stream for ${sid}`)
    return s
  }
}

/** A settled history line, as the fixed Node records it. */
export const line = {
  user: (seq: number, text: string): ChatHistoryRecord => ({
    sequence: seq,
    data: { at, seq, type: 'update', update: { sessionUpdate: 'user_message_chunk', content: { type: 'text', text } } },
  }),
  say: (seq: number, text: string): ChatHistoryRecord => ({
    sequence: seq,
    data: { at, seq, type: 'update', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } } },
  }),
  ended: (seq: number): ChatHistoryRecord => ({ sequence: seq, data: { at, seq, type: 'turnEnded', stop_reason: 'end_turn' } }),
}

export const executors: ChatExecutor[] = [
  { id: 'oc-fixture', name: 'OpenCode · Fixture', agentName: 'OpenCode', modelName: 'Fixture' },
  { id: 'oc-sonnet', name: 'OpenCode · Claude Sonnet 5.5', agentName: 'OpenCode', modelName: 'Claude Sonnet 5.5' },
  { id: 'cc-sonnet', name: 'Claude Code · Claude Sonnet 5.5', agentName: 'Claude Code', modelName: 'Claude Sonnet 5.5' },
]

const conversation = (id: string, title: string, workspaceId = '', sessions = 1): ChatConversation => ({
  id, projectId: 'p-cloud', workspaceId, executorId: 'oc-fixture', title, createdAt: at, updatedAt: at,
  sessions: Array.from({ length: sessions }, (_, i) => ({ id: `${id}-s${i + 1}`, ordinal: i + 1, startedAt: at, endedAt: null, endReason: '' as const })),
})

/** What the fake chat backend holds; tests read and change it. */
export interface ChatBackend {
  conversations: ChatConversation[]
  projectOrder: string[]
  /** Makes the next admission answer this instead of admitting. */
  refuse?: { status: number; data: unknown }
  /** Requests seen, as "METHOD /path". */
  seen: string[]
  bodies: unknown[]
}

/**
 * Fakes the chat backend for one test: two projects (one with a worktree), three executors and
 * two conversations in ora-space/cloud. With `unavailable` every chat route answers 503, as
 * v2-server does without an execution runtime.
 */
export function chatBackend(options: { unavailable?: boolean } = {}): ChatBackend {
  globalThis.EventSource = FakeEventSource as never
  FakeEventSource.all = []
  const state: ChatBackend = {
    conversations: [conversation('c-main', '梳理 v2 接口缺口'), conversation('c-wt', '对话页样式', 'w-chat-1')],
    projectOrder: [],
    seen: [],
    bodies: [],
  }
  let next = 0
  const base = `/api/v2/tenants/${tenantId}/chat`
  const route = (config: InternalAxiosRequestConfig) => {
    const url = config.url ?? ''
    const method = config.method?.toUpperCase()
    const body = typeof config.data === 'string' ? JSON.parse(config.data) : config.data
    if (body !== undefined) state.bodies.push(body)
    if (url === '/api/v1/me') return { status: 200, data: demoUser }
    if (url === '/api/v1/me/tenants') return { status: 200, data: { items: [{ id: tenantId, name: '演示团队', status: 'active', role: 'owner' }], nextCursor: '' } }
    if (url === `/api/v1/tenants/${tenantId}/projects`) return { status: 200, data: { items: [project('p-cloud', 'ora-space/cloud'), project('p-desktop', 'ora-space/desktop')], nextCursor: '' } }
    if (url.endsWith('/projects/p-cloud/workspaces')) return { status: 200, data: { items: [{ id: 'w-chat-1', kind: 'isolated', branchName: 'demo/chat-1', projectId: 'p-cloud' }], nextCursor: '' } }
    if (url.endsWith('/workspaces')) return { status: 200, data: { items: [], nextCursor: '' } }
    if (!url.startsWith(base)) return undefined
    if (options.unavailable) return { status: 503, data: { code: 'execution_unavailable', params: {}, requestId: 'r' } }
    const path = url.slice(base.length)
    const find = (id: string) => state.conversations.find((c) => c.id === id)
    if (path === '/executors') return { status: 200, data: { items: executors } }
    if (path === '/project-order') {
      if (method === 'PUT') state.projectOrder = body.projectIds
      return { status: 200, data: { projectIds: state.projectOrder } }
    }
    if (path === '/conversations' && method === 'GET') return { status: 200, data: { items: state.conversations, nextCursor: '' } }
    if (method === 'POST' && state.refuse && (path === '/conversations' || path.endsWith('/messages'))) {
      const refused = state.refuse
      state.refuse = undefined
      return refused
    }
    if (path === '/conversations' && method === 'POST') {
      const c = { ...conversation(`c-new-${++next}`, String(body.prompt).slice(0, 24), body.workspaceId), projectId: body.projectId, executorId: body.executorId }
      state.conversations.push(c)
      return { status: 201, data: { conversation: c, turn: { sessionId: c.sessions[0].id, turnId: 't1' } } }
    }
    const [, id, action] = path.match(/^\/conversations\/([^/]+)\/?(.*)$/) ?? []
    const c = id ? find(id) : undefined
    if (!c) return { status: 404, data: { code: 'not_found', params: {}, requestId: 'r' } }
    const last = c.sessions.at(-1)!
    if (action === 'messages') {
      if (last.endedAt) c.sessions.push({ id: `${c.id}-s${c.sessions.length + 1}`, ordinal: c.sessions.length + 1, startedAt: at, endedAt: null, endReason: '' })
      return { status: 201, data: { conversation: c, turn: { sessionId: c.sessions.at(-1)!.id, turnId: 't2' } } }
    }
    if (action === 'stop' || action === 'end') {
      Object.assign(last, { endedAt: at, endReason: action === 'stop' ? 'stopped' : 'released' })
      return action === 'stop'
        ? { status: 200, data: { result: { status: 'cancelled', answer: '', answerComplete: false, stopReason: 'cancelled', failureCode: '' } } }
        : { status: 200, data: c }
    }
    if (method === 'PATCH') return { status: 200, data: Object.assign(c, { title: body.title }) }
    if (method === 'DELETE') {
      state.conversations = state.conversations.filter((x) => x !== c)
      return { status: 204 }
    }
    return undefined
  }
  const seen = fakeBackend(route)
  state.seen = seen
  return state
}
