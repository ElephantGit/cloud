import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyConversation } from './model/types'
import { emptyDraft, newChatKey, replayClock, resetChatStore, useChatStore } from './store'

const state = () => useChatStore.getState()
const conversation = (id: string) => state().conversations[id] ?? emptyConversation

describe('chat store', () => {
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

  it('answers a new session with a stand-in reply and names it after the first prompt', () => {
    const id = state().createSession('cloud', { kind: 'isolated', branch: 'feat/x' })
    expect(state().selectedId).toBe(id)
    state().send(id, { ...emptyDraft, text: '列出所有接口' })
    const c = conversation(id)
    expect(c.turns[0]).toMatchObject({ prompt: { text: '列出所有接口' }, status: 'completed' })
    expect(c.turns[0].items.at(-1)).toMatchObject({ kind: 'message', text: expect.stringContaining('演示回复') })
    expect(state().sessions.find((s) => s.id === id)).toMatchObject({ title: '列出所有接口', workspace: { kind: 'isolated', branch: 'feat/x' } })
  })

  it('starts a new chat where the selected session works, but never among the samples', () => {
    state().select('demo-2')
    state().startNewChat()
    expect(state().selectedId).toBeUndefined()
    expect(state().newChat).toMatchObject({ projectId: 'cloud', workspace: { kind: 'isolated', branch: 'feat/766-v2-chat' } })
    state().select('sample-tools')
    state().startNewChat()
    expect(state().newChat).toMatchObject({ projectId: 'cloud', workspace: { kind: 'isolated', branch: 'feat/766-v2-chat' } })
    state().startNewChat({ projectId: 'desktop' })
    expect(state().newChat).toMatchObject({ projectId: 'desktop', workspace: { kind: 'main' } })
  })

  it('creates the session of a new chat from its first prompt', () => {
    state().startNewChat({ projectId: 'desktop', workspace: { kind: 'isolated', branch: 'feat/y' } })
    state().setNewChat({ agent: 'codex', model: 'GPT-5.5 Mini' })
    state().setDraft(newChatKey, { ...emptyDraft, text: '你好' })
    const before = state().sessions.length
    state().sendNewChat({ ...emptyDraft, text: '你好' })
    expect(state().sessions).toHaveLength(before + 1)
    const session = state().sessions.at(-1)
    expect(session).toMatchObject({ projectId: 'desktop', workspace: { kind: 'isolated', branch: 'feat/y' }, agent: 'codex', model: 'GPT-5.5 Mini', title: '你好' })
    expect(state().selectedId).toBe(session?.id)
    expect(state().drafts[newChatKey]).toEqual(emptyDraft)
    expect(conversation(session!.id).turns[0].prompt.text).toBe('你好')
  })

  it('names a new worktree from the first prompt, clear of the branches the project has', () => {
    state().startNewChat({ projectId: 'cloud', workspace: { kind: 'new-worktree' } })
    state().sendNewChat({ ...emptyDraft, text: 'feat 766 v2 chat' })
    expect(state().sessions.at(-1)?.workspace).toEqual({ kind: 'isolated', branch: 'ora/feat-766-v2-chat' })
    state().startNewChat({ projectId: 'cloud', workspace: { kind: 'new-worktree' } })
    state().sendNewChat({ ...emptyDraft, text: 'feat 766 v2 chat again' })
    expect(state().sessions.at(-1)?.workspace).toEqual({ kind: 'isolated', branch: 'ora/feat-766-v2-chat-2' })
  })

  it('queues a prompt sent while the agent is busy and sends it afterwards', () => {
    vi.useFakeTimers()
    replayClock.scale = 1
    state().select('demo-1')
    state().send('demo-1', { ...emptyDraft, text: '第一条' })
    state().send('demo-1', { ...emptyDraft, text: '第二条' })
    expect(state().queued['demo-1']).toEqual(['第二条'])

    vi.runAllTimers()
    expect(state().queued['demo-1']).toEqual([])
    expect(conversation('demo-1').turns.map((t) => [t.prompt.text, t.status])).toEqual([
      ['第一条', 'completed'],
      ['第二条', 'completed'],
    ])
  })

  it('stops a reply mid-turn', () => {
    vi.useFakeTimers()
    replayClock.scale = 1
    state().send('demo-1', { ...emptyDraft, text: '写长一点' })
    vi.advanceTimersByTime(300)
    state().stop('demo-1')
    vi.runAllTimers()
    expect(conversation('demo-1').turns[0]).toMatchObject({ status: 'cancelled', stopReason: 'cancelled' })
    expect(state().playing['demo-1']).toBeUndefined()
  })

  it('marks a session unread when it finishes in the background', () => {
    vi.useFakeTimers()
    replayClock.scale = 1
    state().send('demo-1', { ...emptyDraft, text: '后台跑' })
    state().select('demo-4')
    vi.runAllTimers()
    expect(state().unread['demo-1']).toBe(true)
    state().select('demo-1')
    expect(state().unread['demo-1']).toBeUndefined()
  })

  it('renames and deletes sessions', () => {
    state().renameSession('demo-1', '  新名字 ')
    expect(state().sessions.find((s) => s.id === 'demo-1')?.title).toBe('新名字')
    state().renameSession('demo-1', '   ')
    expect(state().sessions.find((s) => s.id === 'demo-1')?.title).toBe('新名字')
    state().select('demo-1')
    state().deleteSession('demo-1')
    expect(state().sessions.some((s) => s.id === 'demo-1')).toBe(false)
    expect(state().selectedId).toBeUndefined()
  })

  it('moves a project and remembers the order', () => {
    state().moveProject('desktop', 'samples')
    expect(state().projects.map((p) => p.id)).toEqual(['desktop', 'samples', 'cloud'])
    const saved = JSON.parse(localStorage.getItem('ora-v2-chat') ?? '{}')
    expect(saved.state.projectOrder).toEqual(['desktop', 'samples', 'cloud'])
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

  it('records a model switch only once the session has turns, naming the agent when it changes', () => {
    state().setAgentModel('demo-4', 'claude-code', 'Claude Haiku 5.5')
    expect(conversation('demo-4').markers).toEqual([])
    state().send('demo-4', { ...emptyDraft, text: '你好' })
    state().setAgentModel('demo-4', 'claude-code', 'Claude Opus 5.5')
    state().setAgentModel('demo-4', 'codex', 'GPT-5.5')
    expect(conversation('demo-4').markers).toMatchObject([
      { kind: 'model', model: 'Claude Opus 5.5', afterTurn: 1 },
      { kind: 'model', model: 'Codex · GPT-5.5', afterTurn: 1 },
    ])
    expect(state().sessions.find((s) => s.id === 'demo-4')).toMatchObject({ agent: 'codex', model: 'GPT-5.5' })
  })

  it('adds a project named after its folder', () => {
    const id = state().createProject('/workspace/my-app/')
    expect(state().projects.at(-1)).toEqual({ id, name: 'my-app', path: '/workspace/my-app' })
    const windows = state().createProject('C:\\code\\tool')
    expect(state().projects.find((p) => p.id === windows)?.name).toBe('tool')
  })
})
