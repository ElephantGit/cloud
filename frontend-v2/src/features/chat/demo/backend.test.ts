import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyConversation } from '../model/types'
import { emptyDraft, isEnded, replayClock, resetChatStore, useChatStore } from '../store'
import { installDemoBackend, resetDemoBackend } from './backend'

const state = () => useChatStore.getState()
const conversation = (id: string) => state().conversations[id] ?? emptyConversation
const byTitle = (title: string) => state().sessions.find((s) => s.title === title)!.id
const wait = (ms: number) => vi.advanceTimersByTimeAsync(ms)

describe('demo backend', () => {
  beforeEach(async () => {
    vi.useFakeTimers()
    replayClock.scale = 0
    resetChatStore()
    installDemoBackend()
    resetDemoBackend()
    const connected = state().connect('demo-tenant')
    await wait(500)
    await connected
  })
  afterEach(() => vi.useRealTimers())

  it('lists made-up projects and seeded conversations', () => {
    expect(state().backend).toBe('ready')
    expect(state().projects.map((p) => p.name)).toEqual(['示例', 'ora-space/cloud', 'ora-space/desktop'])
    expect(state().sessions.filter((s) => s.remote).map((s) => s.title)).toEqual(['梳理 v2 接口缺口', '对话页样式', '跑一遍检查'])
  })

  it('replays seeded conversations: one still live, and two ended with what ended them', async () => {
    const gaps = byTitle('梳理 v2 接口缺口')
    state().select(gaps)
    await wait(10)
    expect(conversation(gaps).turns.map((t) => t.status)).toEqual(['completed', 'completed'])
    expect(conversation(gaps).markers).toEqual([])
    expect(isEnded(state().views[gaps])).toBe(false)
    const stopped = byTitle('对话页样式')
    state().select(stopped)
    await wait(10)
    expect(conversation(stopped).turns[0].status).toBe('cancelled')
    expect(conversation(stopped).markers).toMatchObject([{ reason: 'stopped' }])
    const failed = byTitle('跑一遍检查')
    state().select(failed)
    await wait(10)
    expect(conversation(failed).turns[0]).toMatchObject({ status: 'failed', error: expect.stringContaining('没能完整读取') })
    expect(conversation(failed).markers).toMatchObject([{ reason: 'interrupted' }])
  })

  it('streams a scripted reply whose previews settle into one answer', async () => {
    state().setNewChat({ projectId: 'demo-cloud' })
    const sent = state().sendNewChat({ ...emptyDraft, text: '介绍一下对话页' })
    await wait(200)
    await sent
    const id = state().selectedId!
    await wait(1500)
    expect(conversation(id).turns[0].status).toBe('streaming')
    await wait(30_000)
    const [turn] = conversation(id).turns
    expect(turn.status).toBe('completed')
    expect(turn.items.filter((i) => i.kind === 'message')).toMatchObject([{ text: expect.stringContaining('演示后端的回复') }])
    expect(turn.items.filter((i) => i.kind === 'tool').map((t) => t.status)).toEqual(['completed', 'completed'])
  })

  it('runs two conversations in one project side by side', async () => {
    state().setNewChat({ projectId: 'demo-cloud' })
    const sent = state().sendNewChat({ ...emptyDraft, text: '再开一个' })
    await wait(200)
    await sent
    const id = state().selectedId!
    expect(state().notices).toEqual({})
    expect(state().sessions.find((s) => s.id === id)).toMatchObject({ projectId: 'demo-cloud', remote: expect.anything() })
    state().send(byTitle('梳理 v2 接口缺口'), { ...emptyDraft, text: '接着说' })
    await wait(30_000)
    expect(conversation(id).turns[0].status).toBe('completed')
    expect(conversation(byTitle('梳理 v2 接口缺口')).turns.map((t) => t.status)).toEqual(['completed', 'completed', 'completed'])
  })

  it('stops a long reply, which ends the conversation for good', async () => {
    state().setNewChat({ projectId: 'demo-desktop' })
    const sent = state().sendNewChat({ ...emptyDraft, text: '写一段长的' })
    await wait(200)
    await sent
    const id = state().selectedId!
    await wait(5000)
    state().stop(id)
    await wait(300)
    expect(conversation(id).turns[0].status).toBe('cancelled')
    expect(conversation(id).markers).toMatchObject([{ reason: 'stopped' }])
    expect(isEnded(state().views[id])).toBe(true)
    state().send(id, { ...emptyDraft, text: '继续' })
    await wait(30_000)
    expect(conversation(id).turns.map((t) => t.status)).toEqual(['cancelled'])
    expect(state().notices[id]).toMatchObject({ kind: 'ended' })
  })
})
