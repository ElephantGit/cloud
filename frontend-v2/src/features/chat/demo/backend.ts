import { AxiosError, type AxiosAdapter, type AxiosResponse, getAdapter, type InternalAxiosRequestConfig } from 'axios'
import { http } from '@/lib/api-client'
import type { ChatConversation, ChatEvent, ChatExecutor, ChatHistoryRecord, ChatResult, ChatSessionEndReason } from '../api/generated'
import { remote } from '../remote'
import { reply, type Run, settled } from './agent'

/**
 * A chat backend that runs in the browser, for showing the page without an execution runtime.
 * It keeps v2-server's chat rules: one live session per workspace, a stop ends the whole session,
 * an idle session is ended after {@link idleAfter}, ending or deleting frees the workspace, and a
 * session's events replay after a cursor. The Agent is {@link reply}: scripted, and it says so.
 * Projects and workspaces are made up here too; login and everything else still go to v2-server.
 * Reloading the page restarts this backend from its seeded conversations.
 */
export const idleAfter = 90_000

interface Session {
  id: string
  ordinal: number
  startedAt: string
  endedAt: string | null
  endReason: ChatSessionEndReason
  events: ChatEvent[]
  records: Map<number, ChatHistoryRecord>
  /** Next free history position. */
  position: number
  turns: { id: string; attempt: string; result?: ChatResult }[]
  timers: ReturnType<typeof setTimeout>[]
}

interface Conversation {
  id: string
  key: string
  projectId: string
  workspaceId: string
  executorId: string
  title: string
  createdAt: string
  updatedAt: string
  sessions: Session[]
}

const projects = [
  { id: 'demo-cloud', name: 'ora-space/cloud', worktrees: [{ id: 'demo-cloud-chat-1', branch: 'demo/chat-1' }, { id: 'demo-cloud-chat-2', branch: 'demo/chat-2' }] },
  { id: 'demo-desktop', name: 'ora-space/desktop', worktrees: [] },
]

const executors: ChatExecutor[] = [
  { id: 'demo-opencode-fixture', name: 'OpenCode · Fixture', agentName: 'OpenCode', modelName: 'Fixture（演示）' },
  { id: 'demo-opencode-sonnet', name: 'OpenCode · Claude Sonnet 5.5', agentName: 'OpenCode', modelName: 'Claude Sonnet 5.5（演示）' },
  { id: 'demo-claude-sonnet', name: 'Claude Code · Claude Sonnet 5.5', agentName: 'Claude Code', modelName: 'Claude Sonnet 5.5（演示）' },
]

/** The backend's whole state; {@link resetDemoBackend} seeds it again. */
const db = { conversations: [] as Conversation[], order: [] as string[], next: 0, listeners: new Map<string, Set<(e: ChatEvent) => void>>() }
const id = (prefix: string) => `${prefix}-${(++db.next).toString(36)}`
const nowIso = () => new Date().toISOString()

function publish(s: Session, event: Omit<ChatEvent, 'sequence' | 'occurredAt'>) {
  const e: ChatEvent = { ...event, sequence: s.events.length + 1, occurredAt: nowIso() }
  s.events.push(e)
  for (const fn of db.listeners.get(s.id) ?? []) fn(e)
}

function snapshot(s: Session, turnId: string) {
  const records = [...s.records.values()].sort((a, b) => a.sequence - b.sequence)
  publish(s, { kind: 'history_snapshot', turnId, history: { records, incomplete: records.some((r) => r.preview) } })
}

const live = (c: Conversation) => {
  const last = c.sessions.at(-1)
  return last && !last.endedAt ? last : undefined
}
const running = (s: Session) => s.turns.some((t) => !t.result)

function newSession(c: Conversation): Session {
  const s: Session = { id: id('session'), ordinal: c.sessions.length + 1, startedAt: nowIso(), endedAt: null, endReason: '', events: [], records: new Map(), position: 1, turns: [], timers: [] }
  c.sessions.push(s)
  return s
}

/** Plays a turn: each step updates the history and publishes a snapshot; the result comes last. */
function play(c: Conversation, s: Session, prompt: string, attempt: string, instant = false) {
  const turn = { id: id('turn'), attempt }
  s.turns.push(turn)
  const run: Run = reply(prompt, s.position, Date.now())
  s.position += new Set(run.steps.map((x) => x.position)).size
  if (instant) {
    for (const r of settled(run.steps)) s.records.set(r.sequence, r)
    snapshot(s, turn.id)
    finish(c, s, turn, run.result)
    return turn
  }
  let wait = 0
  for (const step of run.steps) {
    wait += step.delay
    s.timers.push(
      setTimeout(() => {
        const existing = s.records.get(step.position)
        if (step.preview && existing && !existing.preview) return
        s.records.set(step.position, { sequence: step.position, data: step.record, ...(step.preview ? { preview: true } : {}) })
        snapshot(s, turn.id)
      }, wait),
    )
  }
  s.timers.push(setTimeout(() => finish(c, s, turn, run.result), wait + 50))
  return turn
}

function finish(c: Conversation, s: Session, turn: Session['turns'][number], result: ChatResult) {
  turn.result = result
  publish(s, { kind: 'turn_ended', turnId: turn.id, result })
  c.updatedAt = nowIso()
  // Between turns the session keeps the workspace; left alone, it is ended to free it.
  s.timers.push(setTimeout(() => end(s, 'idle'), idleAfter))
}

function end(s: Session, reason: ChatSessionEndReason, cancel?: ChatResult) {
  if (s.endedAt) return
  for (const t of s.timers) clearTimeout(t)
  s.timers = []
  const open = s.turns.find((t) => !t.result)
  if (open && cancel) {
    s.records = new Map([...s.records].filter(([, r]) => !r.preview))
    const p = s.position++
    s.records.set(p, { sequence: p, data: { at: nowIso(), seq: p, type: 'turnEnded', stop_reason: 'cancelled' } })
    snapshot(s, open.id)
    open.result = cancel
    publish(s, { kind: 'turn_ended', turnId: open.id, result: cancel })
  }
  s.endedAt = nowIso()
  s.endReason = reason
  publish(s, { kind: 'session_ended', turnId: '' })
}

const cancelled: ChatResult = { status: 'cancelled', answer: '', answerComplete: false, stopReason: 'cancelled', failureCode: '' }

const view = (c: Conversation): ChatConversation => ({
  id: c.id,
  projectId: c.projectId,
  workspaceId: c.workspaceId,
  executorId: c.executorId,
  title: c.title,
  createdAt: c.createdAt,
  updatedAt: c.updatedAt,
  sessions: c.sessions.map((s) => ({ id: s.id, ordinal: s.ordinal, startedAt: s.startedAt, endedAt: s.endedAt, endReason: s.endReason })),
})

type Reply = { status: number; data?: unknown }
const fault = (status: number, code: string, params: Record<string, unknown> = {}): Reply => ({ status, data: { code, params, requestId: id('req') } })

/** Admits a prompt where the conversation works, unless another conversation holds the workspace. */
function admit(c: Conversation, prompt: string, attempt: string): Reply | Session {
  const current = live(c)
  if (current) {
    if (running(current)) return fault(409, 'workspace_busy', { conversationId: c.id, title: c.title })
    return current
  }
  const holder = db.conversations.find((x) => x !== c && x.projectId === c.projectId && x.workspaceId === c.workspaceId && live(x))
  if (holder) return fault(409, 'workspace_busy', { conversationId: holder.id, title: holder.title })
  if (!prompt.trim()) return fault(400, 'invalid_prompt')
  void attempt
  return newSession(c)
}

function route(config: InternalAxiosRequestConfig): Reply | undefined {
  const url = config.url ?? ''
  const method = config.method?.toUpperCase()
  const body = typeof config.data === 'string' ? JSON.parse(config.data) : config.data
  const key = String(config.headers?.get?.('Idempotency-Key') ?? id('key'))
  if (/^\/api\/v1\/tenants\/[^/]+\/projects$/.test(url) && method === 'GET') {
    return { status: 200, data: { items: projects.map((p) => ({ id: p.id, name: p.name, createdAt: nowIso(), credentialRefId: null, defaultBranch: 'main', deletedAt: null, lifecycle: 'active', ownerUserId: '', repositoryUrl: '', spaceId: '', tenantId: '', version: 1 })), nextCursor: '' } }
  }
  const ws = url.match(/^\/api\/v1\/tenants\/[^/]+\/projects\/([^/]+)\/workspaces$/)
  if (ws && method === 'GET') {
    const p = projects.find((x) => x.id === ws[1])
    return { status: 200, data: { items: (p?.worktrees ?? []).map((w) => ({ id: w.id, kind: 'isolated', branchName: w.branch, projectId: p!.id })), nextCursor: '' } }
  }
  const chat = url.match(/^\/api\/v2\/tenants\/[^/]+\/chat(\/.*)$/)
  if (!chat) return undefined
  const path = chat[1]
  if (path === '/executors') return { status: 200, data: { items: executors } }
  if (path === '/project-order') {
    if (method === 'PUT') db.order = body.projectIds
    return { status: 200, data: { projectIds: db.order } }
  }
  if (path === '/conversations' && method === 'GET') return { status: 200, data: { items: db.conversations.map(view), nextCursor: '' } }
  if (path === '/conversations' && method === 'POST') {
    const again = db.conversations.find((c) => c.key === key)
    if (again) return { status: 201, data: { conversation: view(again), turn: { sessionId: again.sessions[0].id, turnId: again.sessions[0].turns[0].id } } }
    const c: Conversation = {
      id: id('conv'), key, projectId: body.projectId, workspaceId: body.workspaceId ?? '', executorId: body.executorId,
      title: String(body.prompt).trim().replace(/\s+/g, ' ').slice(0, 24), createdAt: nowIso(), updatedAt: nowIso(), sessions: [],
    }
    const admitted = admit(c, body.prompt, key)
    if (!('turns' in admitted)) return admitted
    db.conversations.push(c)
    const turn = play(c, admitted, body.prompt, key)
    return { status: 201, data: { conversation: view(c), turn: { sessionId: admitted.id, turnId: turn.id } } }
  }
  const [, cid, action] = path.match(/^\/conversations\/([^/]+)\/?(.*)$/) ?? []
  const c = db.conversations.find((x) => x.id === cid)
  if (!c) return fault(404, 'not_found')
  switch (action) {
    case 'messages': {
      const admitted = admit(c, body.prompt, key)
      if (!('turns' in admitted)) return admitted
      const turn = play(c, admitted, body.prompt, key)
      return { status: 201, data: { conversation: view(c), turn: { sessionId: admitted.id, turnId: turn.id } } }
    }
    case 'stop': {
      const s = live(c)
      if (!s) return fault(409, 'no_active_turn')
      const last = s.turns.at(-1)
      // As on the fixed Node, cancelling ends the whole session; a finished turn keeps its result.
      if (last?.result) return { status: 200, data: { result: last.result } }
      end(s, 'stopped', cancelled)
      return { status: 200, data: { result: cancelled } }
    }
    case 'end': {
      const s = live(c)
      if (!s) return fault(409, 'no_active_turn')
      end(s, 'released', cancelled)
      return { status: 200, data: view(c) }
    }
    case '':
      if (method === 'PATCH') {
        c.title = String(body.title).trim()
        return { status: 200, data: view(c) }
      }
      if (method === 'DELETE') {
        const s = live(c)
        if (s) end(s, 'deleted', cancelled)
        db.conversations = db.conversations.filter((x) => x !== c)
        return { status: 204 }
      }
  }
  return fault(404, 'not_found')
}

/** An EventSource over the demo backend's session events, resuming after `?after=`. */
class DemoEventSource {
  onmessage: ((m: MessageEvent<string>) => void) | null = null
  private listeners: Record<string, (() => void)[]> = {}
  private stop?: () => void
  constructor(url: string) {
    const [, sid] = url.match(/\/sessions\/([^/?]+)\/events/) ?? []
    const after = Number(new URL(url, 'http://demo').searchParams.get('after') ?? 0)
    const s = db.conversations.flatMap((c) => c.sessions).find((x) => x.id === sid)
    // Deliver asynchronously, as a network stream would, once listeners are attached.
    setTimeout(() => {
      if (!s) return this.emit('failure')
      const send = (e: ChatEvent) => {
        this.onmessage?.({ data: JSON.stringify(e), lastEventId: String(e.sequence) } as MessageEvent<string>)
        if (e.kind === 'session_ended') this.emit('end')
      }
      for (const e of s.events.filter((x) => x.sequence > after)) send(e)
      if (s.endedAt) return
      const set = db.listeners.get(s.id) ?? new Set()
      set.add(send)
      db.listeners.set(s.id, set)
      this.stop = () => set.delete(send)
    }, 0)
  }
  addEventListener(name: string, fn: () => void) {
    ;(this.listeners[name] ??= []).push(fn)
  }
  close() {
    this.stop?.()
  }
  private emit(name: string) {
    this.stop?.()
    for (const fn of this.listeners[name] ?? []) fn()
  }
}

/** Seeds conversations that show the states a chat goes through, already settled. */
export function resetDemoBackend() {
  for (const c of db.conversations) for (const s of c.sessions) for (const t of s.timers) clearTimeout(t)
  db.conversations = []
  db.order = []
  db.listeners.clear()
  const seed = (projectId: string, workspaceId: string, title: string, prompts: { text: string; then?: ChatSessionEndReason }[]) => {
    const c: Conversation = { id: id('conv'), key: id('seed'), projectId, workspaceId, executorId: executors[0].id, title, createdAt: nowIso(), updatedAt: nowIso(), sessions: [] }
    db.conversations.push(c)
    let s = newSession(c)
    for (const p of prompts) {
      if (s.endedAt) s = newSession(c)
      if (p.then === 'stopped') {
        // Stopped mid-turn: the tools settled, the answer never came.
        const turn = { id: id('turn'), attempt: id('attempt') }
        s.turns.push(turn)
        const run = reply(p.text, s.position)
        for (const r of settled(run.steps).slice(0, 3)) s.records.set(r.sequence, r)
        s.position += 3
        snapshot(s, turn.id)
        end(s, 'stopped', cancelled)
        continue
      }
      play(c, s, p.text, id('attempt'), true)
      if (p.then) end(s, p.then)
    }
  }
  // Holds ora-space/cloud's main workspace, so a new chat there shows the busy notice.
  seed('demo-cloud', '', '梳理 v2 接口缺口', [{ text: '列出对话页要用到的接口', then: 'idle' }, { text: '再补上事件流的续传' }])
  seed('demo-cloud', 'demo-cloud-chat-1', '对话页样式', [{ text: '对照 desktop 检查对话页样式', then: 'stopped' }])
  seed('demo-desktop', '', '跑一遍检查', [{ text: '跑一遍检查，看看哪里失败', then: 'released' }])
}

/** Routes the chat and project requests to the demo backend and its event streams. */
export function installDemoBackend() {
  if ((http.defaults as { demo?: boolean }).demo) return
  resetDemoBackend()
  const network = getAdapter(http.defaults.adapter)
  const adapter: AxiosAdapter = async (config) => {
    const answer = route(config)
    if (!answer) return network(config)
    await new Promise((r) => setTimeout(r, 120))
    const response: AxiosResponse = { status: answer.status, statusText: String(answer.status), data: answer.data, headers: {}, config }
    if (answer.status >= 400) throw new AxiosError(`HTTP ${answer.status}`, String(answer.status), config, undefined, response)
    return response
  }
  http.defaults.adapter = adapter
  ;(http.defaults as { demo?: boolean }).demo = true
  remote.eventSource = (url) => new DemoEventSource(url) as unknown as EventSource
}
