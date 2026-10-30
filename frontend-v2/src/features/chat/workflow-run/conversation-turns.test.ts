import { describe, expect, it } from 'vitest'
import type { WorkflowNodeConversationItem } from '@/lib/workflows/runtime'
import { nodeConversationTurns } from './conversation-turns'

const base = { runId: 'run-1', nodeId: 'review', sessionId: 's-1', status: 'complete' as const }
const message = (id: string, role: 'user' | 'assistant', markdown: string, at: string): WorkflowNodeConversationItem => ({
  ...base,
  kind: 'message',
  id,
  role,
  markdown,
  createdAt: at,
  updatedAt: at,
})
const activity = (id: string, activityKind: 'thought' | 'tool', summary: string, at: string, detail?: string): WorkflowNodeConversationItem => ({
  ...base,
  kind: 'activity',
  id,
  activityKind,
  summary,
  detail,
  createdAt: at,
  updatedAt: at,
})

const conversation = [
  message('m1', 'user', '审查改动', '2026-10-30T06:00:00.000Z'),
  activity('a1', 'thought', '分析节点上下文', '2026-10-30T06:00:00.500Z', '先比较指令和上下文'),
  activity('a2', 'tool', '读取工作流上下文', '2026-10-30T06:00:01.000Z', '读取上游输出'),
  message('m2', 'assistant', 'PASS', '2026-10-30T06:00:02.000Z'),
  message('m3', 'user', '再看一遍测试', '2026-10-30T06:00:03.000Z'),
  message('m4', 'assistant', '测试也通过', '2026-10-30T06:00:04.000Z'),
]

describe('nodeConversationTurns', () => {
  it('opens a turn with each message sent to the Agent and keeps what followed in it', () => {
    const { turns, markers, permissions } = nodeConversationTurns(conversation, 'succeeded')
    expect(markers).toEqual([])
    expect(permissions).toEqual([])
    expect(turns.map((t) => [t.prompt.text, t.status, t.items.map((i) => i.kind)])).toEqual([
      ['审查改动', 'completed', ['thought', 'tool', 'message']],
      ['再看一遍测试', 'completed', ['message']],
    ])
    expect(turns[0]?.items).toEqual([
      { kind: 'thought', id: 'a1', text: '先比较指令和上下文', startedAt: Date.parse('2026-10-30T06:00:00.500Z'), endedAt: Date.parse('2026-10-30T06:00:00.500Z') },
      {
        kind: 'tool',
        id: 'a2',
        title: '读取工作流上下文',
        toolKind: 'other',
        status: 'completed',
        content: [{ type: 'content', content: { type: 'text', text: '读取上游输出' } }],
        locations: [],
        startedAt: Date.parse('2026-10-30T06:00:01.000Z'),
        endedAt: Date.parse('2026-10-30T06:00:01.000Z'),
      },
      { kind: 'message', id: 'm2', text: 'PASS' },
    ])
    const first = turns[0]!
    expect((first.endedAt ?? 0) - first.startedAt).toBe(2000)
  })

  it('ends the last turn the way the node ended', () => {
    const live = nodeConversationTurns(conversation, 'running').turns.at(-1)
    expect(live?.status).toBe('streaming')
    expect(live?.endedAt).toBeUndefined()
    expect(nodeConversationTurns(conversation, 'cancelled').turns.at(-1)?.status).toBe('cancelled')
    expect(nodeConversationTurns(conversation, 'failed', '超时').turns.at(-1)).toMatchObject({ status: 'failed', error: '超时' })
    // Only the last turn is live; earlier ones are finished.
    expect(nodeConversationTurns(conversation, 'running').turns[0]?.status).toBe('completed')
  })

  it('keeps output that arrived before any message in a turn of its own', () => {
    const { turns } = nodeConversationTurns([message('m1', 'assistant', '准备好了', '2026-10-30T06:00:00.000Z')], 'succeeded')
    expect(turns).toHaveLength(1)
    expect(turns[0]).toMatchObject({ prompt: { text: '' }, items: [{ kind: 'message', text: '准备好了' }] })
  })
})
