import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyConversation } from '@/components/agent-thread/model/types'
import { agentsOf, emptyDraft, isEnded, newChatKey, resetChatStore, useChatStore } from './store'
import { type RemoteState, remote } from './remote'
import { type ChatBackend, chatBackend, executors, FakeEventSource, line, tenantId } from './test-backend'

const state = () => useChatStore.getState()
const conversation = (id: string) => state().conversations[id] ?? emptyConversation
const userId = 'user-a'

describe('chat store: layout', () => {
  beforeEach(() => resetChatStore())

  it('remembers how the history column is laid out', async () => {
    state().setSidebar({ width: 360 })
    state().setSidebar({ open: false })
    const saved = localStorage.getItem('ora-v2-chat') ?? '{}'
    expect(JSON.parse(saved).state.sidebar).toEqual({ width: 360, open: false })
    expect(JSON.parse(saved).state.projectOrder).toBeUndefined()
    resetChatStore()
    localStorage.setItem('ora-v2-chat', saved)
    await useChatStore.persist.rehydrate()
    expect(state().sidebar).toEqual({ width: 360, open: false })
  })
})

describe('chat store: backend conversations', () => {
  let backend: ChatBackend
  beforeEach(async () => {
    resetChatStore()
    backend = chatBackend()
    await state().connect(tenantId, userId)
  })

  afterEach(() => vi.restoreAllMocks())

  it('loads projects, executors and conversations', () => {
    expect(state().backend).toBe('ready')
    expect(state().projects.map((p) => p.name)).toEqual(['ora-space/cloud', 'ora-space/desktop'])
    expect(state().sessions.find((s) => s.id === 'c-style')).toMatchObject({ projectId: 'p-cloud', agent: 'OpenCode', model: 'Fixture' })
    expect(state().newChat).toEqual({ projectId: 'p-cloud', agent: 'OpenCode', model: 'Fixture' })
  })

  it('follows a conversation once it is opened and shows each snapshot whole', () => {
    state().select('c-main')
    const stream = FakeEventSource.of('c-main')
    expect(stream.url).toBe(`/api/v2/tenants/${tenantId}/chat/conversations/c-main/events`)
    stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '你好'), line.say(2, '草稿')], incomplete: true } })
    stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '你好'), line.say(2, '你好，我在。'), line.ended(3)], incomplete: false } })
    const [turn] = conversation('c-main').turns
    expect(turn.items).toMatchObject([{ kind: 'message', text: '你好，我在。' }])
    expect(turn.status).toBe('completed')
  })

  it('creates a conversation from the new chat, showing the prompt until the history has it', async () => {
    state().setNewChat({ projectId: 'p-cloud', agent: 'Claude Code', model: 'Claude Sonnet 5.5' })
    state().setDraft(newChatKey, { ...emptyDraft, text: '列出接口' })
    const sent = state().sendNewChat({ ...emptyDraft, text: '列出接口' })
    // The input slides down at once: a placeholder holds the prompt while the backend takes it.
    expect(conversation(state().selectedId!).turns[0].prompt.text).toBe('列出接口')
    await sent
    expect(backend.bodies.at(-1)).toEqual({ projectId: 'p-cloud', executorId: 'cc-sonnet', prompt: '列出接口' })
    const id = state().selectedId!
    expect(id).toBe('c-new-1')
    expect(state().drafts[newChatKey]).toEqual(emptyDraft)
    expect(conversation(id).turns.map((t) => [t.prompt.text, t.status])).toEqual([['列出接口', 'streaming']])
    FakeEventSource.of('c-new-1').emit({ kind: 'history_snapshot', history: { records: [line.user(1, '列出接口')], incomplete: false } })
    expect(state().pending[id]).toBeUndefined()
    expect(conversation(id).turns).toHaveLength(1)
  })

  it('keeps the draft when the first prompt is refused, leaving no conversation', async () => {
    backend.refuse = { status: 503, data: { code: 'stop_unconfirmed', params: {}, requestId: 'r' } }
    await state().sendNewChat({ ...emptyDraft, text: '再开一个' })
    expect(state().notices[newChatKey]).toMatchObject({ kind: 'error' })
    expect(state().selectedId).toBeUndefined()
    expect(state().drafts[newChatKey]?.text).toBe('再开一个')
    expect(state().sessions.filter((s) => s.remote)).toHaveLength(2)
  })

  it('queues a prompt the backend refuses while the previous turn still runs', async () => {
    state().select('c-main')
    const stream = FakeEventSource.of('c-main')
    backend.refuse = { status: 409, data: { code: 'turn_running', params: {}, requestId: 'r' } }
    state().send('c-main', { ...emptyDraft, text: '接着' })
    await vi.waitFor(() => expect(state().queued['c-main']).toEqual(['接着']))
    expect(state().notices['c-main']).toBeUndefined()
    // It waits for the running turn's end instead of retrying straight away.
    const sends = () => backend.seen.filter((r) => r.endsWith('/conversations/c-main/messages')).length
    await new Promise((r) => setTimeout(r, 20))
    expect(sends()).toBe(1)
    stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '第一条'), line.ended(2)], incomplete: false } })
    stream.emit({ kind: 'turn_ended', result: { status: 'completed', answer: '', answerComplete: true, stopReason: 'end_turn', failureCode: '' } })
    await vi.waitFor(() => expect(sends()).toBe(2))
    expect(backend.bodies.at(-1)).toEqual({ prompt: '接着' })
  })

  it('queues while the Agent works, sends when the turn ends, and marks an unread finish', async () => {
    state().select('c-main')
    const stream = FakeEventSource.of('c-main')
    stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '第一条')], incomplete: false } })
    state().send('c-main', { ...emptyDraft, text: '第二条' })
    expect(state().queued['c-main']).toEqual(['第二条'])
    state().startNewChat()
    stream.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '第一条'), line.say(2, '好'), line.ended(3)], incomplete: false } })
    expect(state().unread['c-main']).toBe(true)
    await vi.waitFor(() => expect(backend.bodies.at(-1)).toEqual({ prompt: '第二条' }))
    expect(state().queued['c-main']).toEqual([])
  })

  it('stops a turn, which ends the conversation: it is marked and takes no more prompts', async () => {
    state().select('c-main')
    const first = FakeEventSource.of('c-main')
    first.emit({ kind: 'history_snapshot', history: { records: [line.user(1, '写长一点')], incomplete: false } })
    state().send('c-main', { ...emptyDraft, text: '排队的' })
    state().stop('c-main')
    expect(state().queued['c-main']).toEqual([])
    await vi.waitFor(() => expect(backend.seen).toContain(`POST /api/v2/tenants/${tenantId}/chat/conversations/c-main/stop`))
    first.emit({ kind: 'turn_ended', result: { status: 'cancelled', answer: '', answerComplete: false, stopReason: 'cancelled', failureCode: '' } })
    first.emit({ kind: 'session_ended' })
    first.fire('end')
    expect(conversation('c-main').turns[0].status).toBe('cancelled')
    expect(conversation('c-main').markers).toMatchObject([{ kind: 'session', reason: 'stopped', afterTurn: 1 }])
    expect(isEnded(state().views['c-main'])).toBe(true)
    const requests = backend.seen.length
    state().send('c-main', { ...emptyDraft, text: '重来' })
    expect(state().notices['c-main']).toMatchObject({ kind: 'ended' })
    expect(backend.seen).toHaveLength(requests)
  })

  it('shows a conversation the server ended (a restart) as ended once a send is refused', async () => {
    state().select('c-main')
    backend.conversations[0].session = { ...backend.conversations[0].session!, endedAt: '2026-10-30T09:05:00Z', endReason: 'interrupted' }
    backend.refuse = { status: 409, data: { code: 'conversation_ended', params: {}, requestId: 'r' } }
    state().send('c-main', { ...emptyDraft, text: '还在吗' })
    await vi.waitFor(() => expect(isEnded(state().views['c-main'])).toBe(true))
    expect(state().notices['c-main']).toMatchObject({ kind: 'ended' })
    expect(conversation('c-main').markers).toMatchObject([{ kind: 'session', reason: 'interrupted' }])
  })

  it('renames and deletes conversations on the backend', async () => {
    state().renameSession('c-main', '  接口清单 ')
    expect(state().sessions.find((s) => s.id === 'c-main')?.title).toBe('接口清单')
    expect(backend.bodies.at(-1)).toEqual({ title: '接口清单' })
    state().select('c-main')
    await state().deleteSession('c-main')
    expect(backend.seen).toContain(`DELETE /api/v2/tenants/${tenantId}/chat/conversations/c-main`)
    expect(state().sessions.some((s) => s.id === 'c-main')).toBe(false)
    expect(state().selectedId).toBeUndefined()
  })

  it('saves the project order for the person', () => {
    state().moveProject('p-desktop', 'p-cloud')
    expect(state().projects.map((p) => p.id)).toEqual(['p-desktop', 'p-cloud'])
    expect(backend.bodies.at(-1)).toEqual({ projectIds: ['p-desktop', 'p-cloud'] })
  })

  it('keeps one account\'s state when the same account reconnects', async () => {
    const loads = () => backend.seen.filter((r) => r === `GET /api/v2/tenants/${tenantId}/chat/conversations`).length
    expect(loads()).toBe(1)
    await state().connect(tenantId, userId)
    expect(loads()).toBe(1)
  })

  it('drops the previous account\'s conversations and streams when another account connects', async () => {
    state().select('c-main')
    const oldStream = FakeEventSource.of('c-main')
    backend.conversations = [{
      ...backend.conversations[0],
      id: 'c-member',
      title: '成员自己的对话',
      session: { ...backend.conversations[0].session!, id: 'c-member-s1' },
    }]

    await state().connect(tenantId, 'user-b')

    expect(state().userId).toBe('user-b')
    expect(state().selectedId).toBeUndefined()
    expect(state().sessions.map((s) => s.title)).toEqual(['成员自己的对话'])
    expect(oldStream.closed).toBe(true)
  })

  it('ignores an old account\'s load that finishes after the account changed', async () => {
    resetChatStore()
    backend = chatBackend()
    const snapshot = (id: string, title: string): RemoteState => {
      const c = {
        ...backend.conversations[0],
        id,
        title,
        session: { ...backend.conversations[0].session!, id: `${id}-s1` },
      }
      return { executors: [executors[0]], projects: [{ id: 'p-cloud', name: 'ora-space/cloud' }], conversations: [c], projectOrder: ['p-cloud'] }
    }
    let finishOwner!: (value: RemoteState) => void
    const ownerLoad = new Promise<RemoteState>((resolve) => (finishOwner = resolve))
    vi.spyOn(remote, 'load')
      .mockReturnValueOnce(ownerLoad)
      .mockResolvedValueOnce(snapshot('c-member', '成员自己的对话'))

    const first = state().connect(tenantId, 'user-a')
    await state().connect(tenantId, 'user-b')
    finishOwner(snapshot('c-owner', 'Owner 的对话'))
    await first

    expect(state().userId).toBe('user-b')
    expect(state().sessions.map((s) => s.title)).toEqual(['成员自己的对话'])
  })
})

describe('chat store: backend that cannot run Agents', () => {
  it('reports unavailable and refuses a new chat', async () => {
    resetChatStore()
    chatBackend({ unavailable: true })
    await state().connect(tenantId, userId)
    expect(state().backend).toBe('unavailable')
    await state().sendNewChat({ ...emptyDraft, text: 'hi' })
    expect(state().notices[newChatKey]).toMatchObject({ kind: 'unavailable' })
  })
})

describe('agentsOf', () => {
  it('keeps agents apart when they run a model of the same name, OpenCode first as listed', () => {
    const fixture = (id: string, agentName: string) => ({ id, name: `${agentName} · Fixture`, agentId: id, agentName, modelId: 'fixture', modelName: 'Fixture' })
    expect(agentsOf([fixture('opencode:fixture', 'OpenCode'), fixture('claude:fixture', 'Claude Code')])).toEqual([
      { id: 'OpenCode', name: 'OpenCode', models: ['Fixture'] },
      { id: 'Claude Code', name: 'Claude Code', models: ['Fixture'] },
    ])
  })
})
