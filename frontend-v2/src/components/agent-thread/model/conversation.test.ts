import type * as acp from '@agentclientprotocol/sdk'
import { describe, expect, it } from 'vitest'
import { applyEvent, isResponding } from './conversation'
import { type ChatEvent, type Conversation, emptyConversation } from './types'

const update = (u: acp.SessionUpdate): ChatEvent => ({ type: 'update', update: u })
const text = (t: string, messageId?: string): ChatEvent =>
  update({ sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: t }, messageId })
const thought = (t: string): ChatEvent => update({ sessionUpdate: 'agent_thought_chunk', content: { type: 'text', text: t } })

/** Applies events one millisecond apart, starting at 1000. */
function play(...events: ChatEvent[]): Conversation {
  return events.reduce((c, e, i) => applyEvent(c, e, 1000 + i), emptyConversation)
}

const prompt: ChatEvent = { type: 'prompt', turnId: 't1', text: '你好' }

describe('applyEvent', () => {
  it('opens a streaming turn on a prompt and closes it on turnEnd', () => {
    const open = play(prompt)
    expect(isResponding(open)).toBe(true)
    expect(open.turns[0]).toMatchObject({ id: 't1', prompt: { text: '你好' }, status: 'streaming', startedAt: 1000 })

    const done = applyEvent(open, { type: 'turnEnd', stopReason: 'end_turn' }, 2000)
    expect(isResponding(done)).toBe(false)
    expect(done.turns[0]).toMatchObject({ status: 'completed', stopReason: 'end_turn', endedAt: 2000 })
  })

  it('merges consecutive text chunks into one message, and starts a new one after a tool', () => {
    const c = play(
      prompt,
      text('先看'),
      text('一下。'),
      update({ sessionUpdate: 'tool_call', toolCallId: 'r1', title: '读取 a.ts', kind: 'read' }),
      text('看完了。'),
    )
    const items = c.turns[0].items
    expect(items.map((i) => i.kind)).toEqual(['message', 'tool', 'message'])
    expect(items[0]).toMatchObject({ text: '先看一下。' })
    expect(items[2]).toMatchObject({ text: '看完了。' })
  })

  it('keeps messages with different message ids apart', () => {
    const c = play(prompt, text('一', 'm1'), text('二', 'm2'), text('三', 'm2'))
    expect(c.turns[0].items).toMatchObject([
      { kind: 'message', text: '一' },
      { kind: 'message', text: '二三' },
    ])
  })

  it('drops a leading blank delta instead of opening an empty message', () => {
    const c = play(prompt, text('\n'), update({ sessionUpdate: 'tool_call', toolCallId: 'x', title: 'x' }))
    expect(c.turns[0].items.map((i) => i.kind)).toEqual(['tool'])
  })

  it('times a thought until the agent moves on', () => {
    const c = play(prompt, thought('想'), thought('一想'), text('答案'))
    expect(c.turns[0].items[0]).toMatchObject({ kind: 'thought', text: '想一想', startedAt: 1001, endedAt: 1003 })
  })

  it('follows a tool call through its updates', () => {
    const c = play(
      prompt,
      update({ sessionUpdate: 'tool_call', toolCallId: 'e1', title: '运行 ls', kind: 'execute', status: 'pending' }),
      update({ sessionUpdate: 'tool_call_update', toolCallId: 'e1', status: 'in_progress' }),
      update({ sessionUpdate: 'tool_call_update', toolCallId: 'e1', status: 'completed', rawOutput: { stdout: 'a\nb' } }),
    )
    expect(c.turns[0].items[0]).toMatchObject({
      kind: 'tool',
      title: '运行 ls',
      toolKind: 'execute',
      status: 'completed',
      rawOutput: { stdout: 'a\nb' },
      startedAt: 1001,
      endedAt: 1003,
    })
  })

  it('starts a tool from an update it never saw announced', () => {
    const c = play(prompt, update({ sessionUpdate: 'tool_call_update', toolCallId: 'u', status: 'in_progress', title: null }))
    expect(c.turns[0].items[0]).toMatchObject({ kind: 'tool', id: 'u', title: '工具调用', status: 'in_progress' })
  })

  it('replaces the plan in place', () => {
    const entry = (content: string, status: acp.PlanEntryStatus): acp.PlanEntry => ({ content, status, priority: 'medium' })
    const c = play(
      prompt,
      update({ sessionUpdate: 'plan', entries: [entry('a', 'in_progress'), entry('b', 'pending')] }),
      text('做 a'),
      update({ sessionUpdate: 'plan', entries: [entry('a', 'completed'), entry('b', 'in_progress')] }),
    )
    expect(c.turns[0].items.map((i) => i.kind)).toEqual(['plan', 'message'])
    expect(c.turns[0].items[0]).toMatchObject({ entries: [{ status: 'completed' }, { status: 'in_progress' }] })
  })

  it('records the context usage outside the turn', () => {
    const c = play(prompt, update({ sessionUpdate: 'usage_update', used: 1200, size: 200000 }))
    expect(c.context).toEqual({ used: 1200, size: 200000, cost: undefined })
  })

  it('marks unreported tools cancelled when the turn is cancelled', () => {
    const c = play(
      prompt,
      update({ sessionUpdate: 'tool_call', toolCallId: 'e1', title: '运行', status: 'in_progress' }),
      { type: 'turnEnd', stopReason: 'cancelled' },
    )
    expect(c.turns[0]).toMatchObject({ status: 'cancelled', items: [{ status: 'cancelled' }] })
  })

  it('fails the turn with the error and closes open permissions', () => {
    const c = play(
      prompt,
      { type: 'permission', request: { id: 'p1', toolCall: { toolCallId: 'e1' }, options: [] } },
      { type: 'turnError', message: '模型不可用' },
    )
    expect(c.turns[0]).toMatchObject({ status: 'failed', error: '模型不可用' })
    expect(c.permissions[0].outcome).toEqual({ cancelled: true })
  })

  it('records the option a permission was answered with', () => {
    const c = play(
      prompt,
      { type: 'permission', request: { id: 'p1', toolCall: { toolCallId: 'e1' }, options: [] } },
      { type: 'permissionResolved', id: 'p1', optionId: 'allow' },
    )
    expect(c.permissions[0].outcome).toEqual({ optionId: 'allow' })
  })

  it('ignores updates outside a live turn', () => {
    const done = play(prompt, { type: 'turnEnd', stopReason: 'end_turn' })
    expect(applyEvent(done, text('迟到的'), 5000)).toBe(done)
  })

  it('places markers after the turns so far', () => {
    const c = play(prompt, { type: 'turnEnd', stopReason: 'end_turn' }, { type: 'checkpoint', label: '改样式前' }, { type: 'model', model: 'Claude Opus' })
    expect(c.markers).toEqual([
      { kind: 'checkpoint', id: 'marker-0', afterTurn: 1, label: '改样式前' },
      { kind: 'model', id: 'marker-1', afterTurn: 1, model: 'Claude Opus' },
    ])
  })
})
