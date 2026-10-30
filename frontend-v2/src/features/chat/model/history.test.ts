import { describe, expect, it } from 'vitest'
import type { ChatEvent, ChatHistoryRecord, ChatResult } from '../api/generated'
import { applySessionEvent, conversationOf, emptySessionView, type SessionView } from './history'

const at = (s: number) => `2026-10-30T09:00:${String(s).padStart(2, '0')}Z`
const ms = (s: number) => Date.parse(at(s))

const user = (seq: number, text: string): ChatHistoryRecord => ({
  sequence: seq,
  data: { at: at(seq), seq, type: 'update', update: { sessionUpdate: 'user_message_chunk', content: { type: 'text', text } } },
})
const say = (seq: number, text: string, preview = false): ChatHistoryRecord => ({
  sequence: seq,
  data: { at: at(seq), seq, type: 'update', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text } } },
  ...(preview ? { preview } : {}),
})
const tool = (seq: number, status: 'completed' | 'in_progress'): ChatHistoryRecord => ({
  sequence: seq,
  data: {
    at: at(seq + 5),
    seq,
    type: 'update',
    update: { sessionUpdate: 'tool_call', toolCallId: `call-${seq}`, title: '读取 README.md', kind: 'read', status },
    tool_timing: { startedAt: at(seq), durationMs: 2000 },
  },
})
const ended = (seq: number, stop = 'end_turn'): ChatHistoryRecord => ({ sequence: seq, data: { at: at(seq), seq, type: 'turnEnded', stop_reason: stop } })
const meta: ChatHistoryRecord = { sequence: 0, data: { at: at(0), seq: 0, type: 'meta', schemaVersion: 2, sessionId: 'x' } }
const done: ChatResult = { status: 'completed', answer: '', answerComplete: true, stopReason: 'end_turn', failureCode: '' }

const view = (patch: Partial<SessionView>): SessionView => ({ ...emptySessionView('s1'), ...patch })

describe('applySessionEvent', () => {
  it('replaces the whole history on each snapshot and keeps turn results in order', () => {
    const event = (e: Partial<ChatEvent>): ChatEvent => ({ sequence: 1, occurredAt: at(0), kind: 'history_snapshot', turnId: 't1', ...e })
    let v = emptySessionView('s1')
    v = applySessionEvent(v, event({ history: { records: [user(1, 'hi'), say(2, '草稿', true)], incomplete: true } }))
    v = applySessionEvent(v, event({ history: { records: [user(1, 'hi'), say(2, '定稿')], incomplete: false } }))
    v = applySessionEvent(v, event({ kind: 'turn_ended', result: done }))
    v = applySessionEvent(v, event({ kind: 'session_ended' }))
    expect(v.records.map((r) => r.data)).toEqual([user(1, 'hi').data, say(2, '定稿').data])
    expect(v).toMatchObject({ incomplete: false, results: [done], ended: true })
  })
})

describe('conversationOf', () => {
  it('rebuilds turns from settled history in position order', () => {
    const c = conversationOf([view({ records: [ended(5), say(4, '第二段'), tool(2, 'completed'), user(1, '读一下'), say(3, '第一段'), meta], results: [done] })])
    expect(c.turns).toHaveLength(1)
    const [turn] = c.turns
    expect(turn).toMatchObject({ id: 's1:1', prompt: { text: '读一下' }, status: 'completed', stopReason: 'end_turn' })
    // Two settled messages stay two items; they are never glued into one.
    expect(turn.items.map((i) => i.kind)).toEqual(['tool', 'message', 'message'])
    expect(turn.items[0]).toMatchObject({ status: 'completed', startedAt: ms(2), endedAt: ms(4) })
  })

  it('closes a turn the history left open with its result, and fails one with no result once the session ended', () => {
    const cancelled: ChatResult = { ...done, status: 'cancelled', stopReason: 'cancelled' }
    const c = conversationOf([view({ records: [user(1, 'a'), tool(2, 'in_progress'), user(3, 'b')], results: [cancelled], ended: true, endReason: 'stopped' })])
    expect(c.turns.map((t) => t.status)).toEqual(['cancelled', 'failed'])
    expect(c.turns[0].items[0]).toMatchObject({ kind: 'tool', status: 'cancelled' })
  })

  it('never shows an answer that could not be read back in full as complete', () => {
    const failed: ChatResult = { status: 'failed', answer: '', answerComplete: false, stopReason: 'end_turn', failureCode: 'history_limit' }
    const [turn] = conversationOf([view({ records: [user(1, 'a'), say(2, '前半段'), ended(3)], results: [failed] })]).turns
    expect(turn.status).toBe('failed')
    expect(turn.error).toContain('history_limit')
  })

  it('marks where each ended session stops, so the next one reads as a fresh Agent', () => {
    const c = conversationOf([
      view({ id: 's1', records: [user(1, 'a'), ended(2)], results: [done], ended: true, endReason: 'idle' }),
      view({ id: 's2', records: [user(1, 'b'), ended(2)], results: [done], ended: true }),
      view({ id: 's3', records: [user(1, 'c')] }),
    ])
    expect(c.turns.map((t) => t.prompt.text)).toEqual(['a', 'b', 'c'])
    expect(c.turns.map((t) => t.status)).toEqual(['completed', 'completed', 'streaming'])
    expect(c.markers).toEqual([
      { kind: 'session', id: 'session-s1', afterTurn: 1, reason: 'idle' },
      { kind: 'session', id: 'session-s2', afterTurn: 2, reason: 'interrupted' },
    ])
  })

  it('joins a prompt sent as several blocks', () => {
    const [turn] = conversationOf([view({ records: [user(1, '第一行'), user(2, '第二行')] })]).turns
    expect(turn.prompt.text).toBe('第一行第二行')
  })
})
