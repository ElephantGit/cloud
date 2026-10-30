import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyConversation } from './model/types'
import { emptyDraft, isEnded, newChatKey, replayClock, resetChatStore, useChatStore } from './store'
import { type ChatBackend, chatBackend, FakeEventSource, line, tenantId } from './test-backend'

const state = () => useChatStore.getState()
const conversation = (id: string) => state().conversations[id] ?? emptyConversation

describe('chat store: samples', () => {
  beforeEach(() => {
    replayClock.scale = 0
    resetChatStore()
  })
  afterEach(() => {
    replayClock.scale = 1
    vi.useRealTimers()
  })

  it('plays a sample through to the end the first time it is opened', () => {
    state().select('sample-tools')
    const c = conversation('sample-tools')
    expect(c.turns).toHaveLength(1)
    expect(c.turns[0].status).toBe('completed')
    expect(c.turns[0].items.filter((i) => i.kind === 'tool').map((t) => t.status)).toEqual([
      'completed',
      'completed',
      'completed',
      'completed',
      'completed',
      'failed',
    ])
    expect(state().playing['sample-tools']).toBeUndefined()
  })

  it('every sample plays to a finished conversation', () => {
    for (const s of state().sessions.filter((x) => x.scenarioId && x.scenarioId !== 'permission')) {
      state().select(s.id)
      const c = conversation(s.id)
      expect(c.turns.length, s.id).toBeGreaterThan(0)
      expect(c.turns.every((t) => t.status !== 'streaming'), s.id).toBe(true)
    }
  })

  it('waits on a permission and plays the branch for the answer', () => {
    state().select('sample-permission')
    expect(conversation('sample-permission').permissions).toMatchObject([{ id: 'perm-1' }])
    expect(conversation('sample-permission').turns[0].status).toBe('streaming')
    expect(state().playing['sample-permission']).toBe(true)

    state().answer('sample-permission', 'perm-1', 'reject')
    const c = conversation('sample-permission')
    expect(c.permissions[0].outcome).toEqual({ optionId: 'reject' })
    expect(c.turns[0]).toMatchObject({ status: 'completed' })
    expect(c.turns[0].items.find((i) => i.kind === 'tool')).toMatchObject({ status: 'failed' })
  })

  it('replays a sample from the start', () => {
    state().select('sample-markdown')
    state().replay('sample-markdown')
    expect(conversation('sample-markdown').turns).toHaveLength(1)
  })

  it('takes the model a sample switches to', () => {
    state().select('sample-context')
    expect(state().sessions.find((s) => s.id === 'sample-context')?.model).toBe('Claude Opus 5.5')
  })

  it('answers a prompt typed into a sample with a stand-in that says so', () => {
    state().select('sample-markdown')
    state().send('sample-markdown', { ...emptyDraft, text: '再来一段' })
    const turn = conversation('sample-markdown').turns.at(-1)
    expect(turn).toMatchObject({ prompt: { text: '再来一段' }, status: 'completed' })
    expect(turn?.items.at(-1)).toMatchObject({ kind: 'message', text: expect.stringContaining('演示回复') })
  })

  it('queues a prompt typed while a sample replies and sends it afterwards', () => {
    vi.useFakeTimers()
    state().select('sample-markdown')
    replayClock.scale = 1
    state().send('sample-markdown', { ...emptyDraft, text: '第一条' })
    state().send('sample-markdown', { ...emptyDraft, text: '第二条' })
    expect(state().queued['sample-markdown']).toEqual(['第二条'])
    vi.runAllTimers()
    expect(state().queued['sample-markdown']).toEqual([])
    expect(conversation('sample-markdown').turns.slice(-2).map((t) => [t.prompt.text, t.status])).toEqual([
      ['第一条', 'completed'],
      ['第二条', 'completed'],
    ])
  })

  it('stops a sample mid-reply', () => {
    vi.useFakeTimers()
    state().select('sample-markdown')
    replayClock.scale = 1
    state().send('sample-markdown', { ...emptyDraft, text: '写长一点' })
    vi.advanceTimersByTime(300)
    state().stop('sample-markdown')
    vi.runAllTimers()
    expect(conversation('sample-markdown').turns.at(-1)).toMatchObject({ status: 'cancelled', stopReason: 'cancelled' })
    expect(state().playing['sample-markdown']).toBeUndefined()
  })

  it('records a sample model switch once it has turns, naming the agent when it changes', () => {
    state().setAgentModel('sample-markdown', 'claude-code', 'Claude Haiku 5.5')
    expect(conversation('sample-markdown').markers).toEqual([])
    state().select('sample-markdown')
    state().setAgentModel('sample-markdown', 'claude-code', 'Claude Opus 5.5')
    state().setAgentModel('sample-markdown', 'codex', 'GPT-5.5')
    expect(conversation('sample-markdown').markers).toMatchObject([
      { kind: 'model', model: 'Claude Opus 5.5', afterTurn: 1 },
      { kind: 'model', model: 'Codex · GPT-5.5', afterTurn: 1 },
    ])
  })

  it('remembers how the history column is laid out', async () => {
    state().setSidebar({ width: 360 })
    state().setSidebar({ open: false })
    const saved = localStorage.getItem('ora-v2-chat') ?? '{}'
    expect(JSON.parse(saved).state.sidebar).toEqual({ width: 360, open: false })
    resetChatStore()
    localStorage.setItem('ora-v2-chat', saved)
    await useChatStore.persist.rehydrate()
    expect(state().sidebar).toEqual({ width: 360, open: false })
  })
})

describe('chat store: backend conversations', () => {
  let backend: ChatBackend
  beforeEach(async () => {
    replayClock.scale = 0
    resetChatStore()
    backend = chatBackend()
    await state().connect(tenantId)
  })

  it('loads projects, executors and conversations', () => {
    expect(state().backend).toBe('ready')
    expect(state().projects.map((p) => p.name)).toEqual(['示例', 'ora-space/cloud', 'ora-space/desktop'])
    expect(state().sessions.find((s) => s.id === 'c-style')).toMatchObject({ projectId: 'p-cloud', workspace: { kind: 'main' }, agent: 'OpenCode', model: 'Fixture' })
    expect(state().newChat).toEqual({ projectId: 'p-cloud', workspace: { kind: 'main' }, agent: 'OpenCode', model: 'Fixture' })
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
    state().moveProject('p-desktop', 'samples')
    expect(state().projects.map((p) => p.id)).toEqual(['p-desktop', 'samples', 'p-cloud'])
    expect(backend.bodies.at(-1)).toEqual({ projectIds: ['p-desktop', 'samples', 'p-cloud'] })
  })

  it('keeps a conversation on its executor', () => {
    state().setAgentModel('c-main', 'Claude Code', 'Claude Sonnet 5.5')
    expect(state().sessions.find((s) => s.id === 'c-main')).toMatchObject({ agent: 'OpenCode', model: 'Fixture' })
  })
})

describe('chat store: backend that cannot run Agents', () => {
  it('reports unavailable and keeps the samples', async () => {
    resetChatStore()
    chatBackend({ unavailable: true })
    await state().connect(tenantId)
    expect(state().backend).toBe('unavailable')
    expect(state().sessions.every((s) => s.scenarioId)).toBe(true)
    await state().sendNewChat({ ...emptyDraft, text: 'hi' })
    expect(state().notices[newChatKey]).toMatchObject({ kind: 'unavailable' })
  })
})
