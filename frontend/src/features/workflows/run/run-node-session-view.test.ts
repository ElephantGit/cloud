import { describe, expect, it } from 'vitest'
import type {
  WorkflowNodeSession,
  WorkflowNodeSessionExecutionState,
  WorkflowNodeSessionRecord,
} from '@/api/generated.schemas'
import { nodeSessionRows, nodeSessionView } from '@/features/workflows/run/run-node-session-view'

/** One stored receipt; `extra` carries the canonEvent markers Cloud writes beside the record. */
function receipt(
  sequence: number,
  record: Record<string, unknown>,
  extra: Partial<WorkflowNodeSessionRecord> = {},
): WorkflowNodeSessionRecord {
  return { sequence, record, ...extra }
}

/** A stored message chunk. */
function message(sequence: number, text: string): WorkflowNodeSessionRecord {
  return receipt(sequence, {
    type: 'update',
    update: { sessionUpdate: 'agent_message_chunk', content: [{ type: 'text', text }] },
  })
}

/** One page of the walk, with the fields a test does not care about filled in. */
function page({
  attempt = 1,
  records = [],
  state = 'available',
  incomplete = false,
  hasMore = false,
  nextBefore = null,
  nodeAttempt = attempt,
  sessionEndReason = null,
}: {
  attempt?: number
  records?: WorkflowNodeSessionRecord[]
  state?: WorkflowNodeSessionExecutionState
  incomplete?: boolean
  hasMore?: boolean
  nextBefore?: number | null
  nodeAttempt?: number
  sessionEndReason?: string | null
} = {}): WorkflowNodeSession {
  return {
    execution: {
      attempt,
      executionId: 'exec-1',
      result: null,
      records,
      state,
      incomplete,
      hasMore,
      nextBefore,
    },
    node: {
      status: 'succeeded',
      attempt: nodeAttempt,
      error: null,
      output: null,
      sessionEndReason,
    },
  }
}

describe('nodeSessionView', () => {
  it('rebuilds the conversation oldest first across newest-first pages', () => {
    const view = nodeSessionView([
      page({ records: [message(8, '八'), message(9, '九')] }),
      page({ records: [message(4, '四'), message(5, '五')], hasMore: true, nextBefore: 4 }),
    ])

    expect(view?.rows.map((row) => row.key)).toEqual([4, 5, 8, 9])
    expect(view?.rows.map((row) => row.text)).toEqual(['四', '五', '八', '九'])
  })

  it('takes the paging facts from the oldest loaded page, which is where the walk continues', () => {
    const view = nodeSessionView([
      page({ records: [message(9, '九')], hasMore: true, nextBefore: 9 }),
      page({ records: [message(4, '四')], hasMore: false }),
    ])

    expect(view?.hasMore).toBe(false)
  })

  it('keeps a gap any page observed', () => {
    const view = nodeSessionView([
      page({ records: [message(9, '九')] }),
      page({ records: [message(4, '四')], incomplete: true }),
    ])

    expect(view?.incomplete).toBe(true)
  })

  it('shows the end reason only when the node state describes the attempt being read', () => {
    const described = nodeSessionView([
      page({ attempt: 2, records: [message(9, '九')], sessionEndReason: 'turn_deadline_exceeded' }),
    ])
    const foreign = nodeSessionView([
      page({
        attempt: 1,
        records: [message(9, '九')],
        nodeAttempt: 2,
        sessionEndReason: 'turn_deadline_exceeded',
      }),
    ])

    expect(described?.endReason).toBe('turn_deadline_exceeded')
    expect(foreign?.endReason).toBeNull()
  })

  it('has no view before the first page loads', () => {
    expect(nodeSessionView([])).toBeUndefined()
  })
})

describe('nodeSessionRows', () => {
  it('reads message, thought, tool and turn-end records as their own kinds', () => {
    const rows = nodeSessionRows([
      message(1, '先读代码'),
      receipt(2, {
        type: 'update',
        update: {
          sessionUpdate: 'agent_thought_chunk',
          content: { type: 'text', text: '考虑一下' },
        },
      }),
      receipt(3, {
        type: 'update',
        update: { sessionUpdate: 'tool_call', title: 'read_file', status: 'completed' },
      }),
      receipt(4, { type: 'turnEnded', stopReason: 'end_turn' }),
    ])

    expect(rows.map(({ kind, text }) => [kind, text])).toEqual([
      ['message', '先读代码'],
      ['thought', '考虑一下'],
      ['tool', 'read_file · completed'],
      ['turnEnded', 'end_turn'],
    ])
  })

  it("reads the prompt the Node echoed back as the attempt's own user input", () => {
    const rows = nodeSessionRows([
      receipt(1, {
        type: 'update',
        update: {
          sessionUpdate: 'user_message_chunk',
          content: [{ type: 'text', text: '把登录页的错误提示统一' }],
        },
      }),
      message(2, '好'),
    ])

    expect(rows.map(({ kind, text }) => [kind, text])).toEqual([
      ['user', '把登录页的错误提示统一'],
      ['message', '好'],
    ])
  })

  it("reads the Node's own gap marker as a position in the conversation", () => {
    const rows = nodeSessionRows([
      message(1, '先读代码'),
      receipt(2, { type: 'gap' }),
      message(3, '读完了'),
    ])

    expect(rows.map(({ kind, text }) => [kind, text])).toEqual([
      ['message', '先读代码'],
      ['gap', ''],
      ['message', '读完了'],
    ])
  })

  it('falls back to the raw event for kinds this view does not read', () => {
    const rows = nodeSessionRows([
      receipt(1, { type: 'update', update: { sessionUpdate: 'plan', steps: [] } }),
      receipt(2, { type: 'meta', model: 'vendor/model-v1' }),
    ])

    expect(rows.map((row) => row.kind)).toEqual(['raw', 'raw'])
    expect(rows[0]?.text).toContain('"plan"')
    expect(rows[1]?.text).toContain('vendor/model-v1')
  })

  it('keeps the visible text when the content carries no text block', () => {
    const rows = nodeSessionRows([
      receipt(
        1,
        { type: 'update', update: { sessionUpdate: 'agent_message_chunk', content: [] } },
        { visibleText: '补充文本' },
      ),
    ])

    expect(rows[0]).toMatchObject({ kind: 'message', text: '补充文本' })
  })

  it('reports a record Cloud stored only in part', () => {
    const rows = nodeSessionRows([
      receipt(1, { type: 'turnEnded' }, { truncated: true }),
      receipt(2, { type: 'turnEnded' }, { visibleTextTruncated: true }),
      receipt(3, { type: 'turnEnded' }),
    ])

    expect(rows.map((row) => row.truncated)).toEqual([true, true, false])
  })
})
