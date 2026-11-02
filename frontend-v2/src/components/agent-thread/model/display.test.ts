import type * as acp from '@agentclientprotocol/sdk'
import { describe, expect, it } from 'vitest'
import { applyEvent } from './conversation'
import { buildRows, countLines, turnFileChanges, turnSources } from './display'
import { type ChatEvent, type Turn, emptyConversation } from './types'

const update = (u: acp.SessionUpdate): ChatEvent => ({ type: 'update', update: u })
const child = (id: string, parentID: string, title?: string) => ({ 'opencode/child-session': { id, parentID, depth: 1, title } })

function turn(...events: ChatEvent[]): Turn {
  const all: ChatEvent[] = [{ type: 'prompt', turnId: 't', text: 'go' }, ...events, { type: 'turnEnd', stopReason: 'end_turn' }]
  return all.reduce((c, e, i) => applyEvent(c, e, i), emptyConversation).turns[0]
}
const tool = (id: string, kind: acp.ToolKind, extra: Partial<acp.ToolCall> = {}): ChatEvent =>
  update({ sessionUpdate: 'tool_call', toolCallId: id, title: id, kind, status: 'completed', ...extra })
const say = (text: string, meta?: Record<string, unknown>): ChatEvent =>
  update({ sessionUpdate: 'agent_message_chunk', content: { type: 'text', text }, _meta: meta })

describe('buildRows', () => {
  it('puts the work between two texts into one activity block', () => {
    const rows = buildRows(turn(say('先查'), tool('e', 'execute'), tool('x', 'edit'), say('好了')))
    expect(rows.map((r) => r.kind)).toEqual(['message', 'activity', 'message'])
    expect(rows[1]).toMatchObject({ entries: [{ id: 'e' }, { id: 'x' }] })
  })

  it('clusters consecutive reads and searches', () => {
    const rows = buildRows(turn(tool('r1', 'read'), tool('s1', 'search'), tool('e', 'execute'), tool('r2', 'read')))
    expect(rows[0]).toMatchObject({
      kind: 'activity',
      entries: [{ kind: 'explore', tools: [{ id: 'r1' }, { id: 's1' }] }, { id: 'e' }, { id: 'r2' }],
    })
  })

  it('draws a linked child inside the tool call that started it', () => {
    const rows = buildRows(
      turn(
        tool('task', 'other', { rawOutput: { metadata: { sessionID: 'c1' } } }),
        say('子任务的结论', child('c1', 'root', '调研子代理')),
        say('汇总'),
      ),
    )
    expect(rows.map((r) => r.kind)).toEqual(['activity', 'message'])
    expect(rows[0]).toMatchObject({
      entries: [{ id: 'task', subagent: { kind: 'subagent', id: 'c1', title: '调研子代理', rows: [{ kind: 'message', text: '子任务的结论' }] } }],
    })
  })

  it('places an unlinked child where it first spoke and nests grandchildren', () => {
    const rows = buildRows(
      turn(say('开始'), say('子', child('c1', 'root')), say('孙', child('c2', 'c1')), say('结束')),
    )
    expect(rows).toMatchObject([
      { kind: 'message', text: '开始' },
      {
        kind: 'activity',
        entries: [
          {
            kind: 'subagent',
            id: 'c1',
            rows: [{ kind: 'message', text: '子' }, { kind: 'activity', entries: [{ kind: 'subagent', id: 'c2', rows: [{ text: '孙' }] }] }],
          },
        ],
      },
      { kind: 'message', text: '结束' },
    ])
  })

  it('leaves resource links to the sources list', () => {
    const link: ChatEvent = update({
      sessionUpdate: 'agent_message_chunk',
      content: { type: 'resource_link', name: 'ACP', uri: 'https://agentclientprotocol.com' },
    })
    const t = turn(say('见'), link)
    expect(buildRows(t).map((r) => r.kind)).toEqual(['message'])
    expect(turnSources(t)).toMatchObject([{ name: 'ACP' }])
  })
})

describe('file changes', () => {
  it('counts lines per file across diffs', () => {
    const diff = (path: string, oldText: string | null, newText: string): ChatEvent =>
      tool(`${path}-${newText.length}`, 'edit', { content: [{ type: 'diff', path, oldText, newText }] })
    const t = turn(diff('a.ts', 'x\ny\n', 'x\nz\nw\n'), diff('b.ts', null, '1\n2\n'), diff('a.ts', 'w\n', ''))
    expect(turnFileChanges(t)).toEqual([
      { path: 'a.ts', added: 2, removed: 2, created: false },
      { path: 'b.ts', added: 2, removed: 0, created: true },
    ])
  })

  it('treats lines as a multiset', () => {
    expect(countLines('a\na\nb', 'a\nb\nb')).toEqual({ added: 1, removed: 1 })
  })
})
