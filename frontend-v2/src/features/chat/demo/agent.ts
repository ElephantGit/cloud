import type { ChatHistoryRecord, ChatResult } from '../api/generated'

/**
 * What a scripted turn does, step by step, in the fixed Node's history vocabulary: each step
 * writes one record at a history position, as a provisional preview or settled. Replaying the
 * steps through the demo backend produces the same snapshots a real session would.
 */
export interface Step {
  /** Milliseconds after the previous step. */
  delay: number
  /** History position; a later step at the same position replaces the earlier one. */
  position: number
  record: Record<string, unknown>
  preview?: boolean
}

/** A scripted turn: its steps and the result the backend reports once it ends. */
export interface Run {
  steps: Step[]
  result: ChatResult
}

const update = (u: Record<string, unknown>, extra: Record<string, unknown> = {}) => ({ type: 'update', update: u, ...extra })
const text = (kind: 'agent_message_chunk' | 'agent_thought_chunk', t: string) => update({ sessionUpdate: kind, content: { type: 'text', text: t } })

/** Builds a turn's steps, handing out history positions from `first`. */
class Turn {
  readonly steps: Step[] = []
  private position: number
  private now: number
  constructor(first: number, start: number) {
    this.position = first
    this.now = start
  }

  private at(delay: number) {
    this.now += delay
    return new Date(this.now).toISOString()
  }

  private write(delay: number, position: number, record: Record<string, unknown>, preview = false) {
    this.steps.push({ delay, position, record: { at: this.at(delay), seq: position, ...record }, ...(preview ? { preview } : {}) })
  }

  user(prompt: string) {
    this.write(0, this.position++, update({ sessionUpdate: 'user_message_chunk', content: { type: 'text', text: prompt } }))
    return this
  }

  think(t: string) {
    const p = this.position++
    this.write(300, p, text('agent_thought_chunk', t.slice(0, Math.ceil(t.length / 2))), true)
    this.write(500, p, text('agent_thought_chunk', t))
    return this
  }

  /** A tool shown running, then settled; `fail` settles it as failed. */
  tool(title: string, kind: string, input: unknown, output: string, opts: { fail?: boolean; path?: string; ms?: number } = {}) {
    const p = this.position++
    const id = `call-${p}`
    const started = new Date(this.now + 200).toISOString()
    const call = { sessionUpdate: 'tool_call', toolCallId: id, title, kind, rawInput: input, locations: opts.path ? [{ path: opts.path }] : [] }
    this.write(200, p, update({ ...call, status: 'in_progress' }), true)
    const ms = opts.ms ?? 900
    this.write(ms, p, update(
      { ...call, status: opts.fail ? 'failed' : 'completed', rawOutput: output, content: [{ type: 'content', content: { type: 'text', text: output } }] },
      { tool_timing: { startedAt: started, durationMs: ms } },
    ))
    return this
  }

  /** Agent text streamed as growing previews, then settled whole. */
  say(t: string, chunk = 8, every = 60) {
    const p = this.position++
    for (let n = chunk; n < t.length; n += chunk) this.write(every, p, text('agent_message_chunk', t.slice(0, n)), true)
    this.write(every, p, text('agent_message_chunk', t))
    return this
  }

  end(stop = 'end_turn') {
    this.write(120, this.position++, { type: 'turnEnded', stop_reason: stop })
    return this
  }
}

const done = (answer: string): ChatResult => ({ status: 'completed', answer, answerComplete: true, stopReason: 'end_turn', failureCode: '' })

/**
 * The demo Agent's reply to a prompt, starting at history position `first`. It always says it is
 * a demo. A prompt mentioning 失败 shows a failed tool and an answer that could not be read back in
 * full; one mentioning 长 streams slowly, to try stopping it.
 */
export function reply(prompt: string, first: number, start = Date.now()): Run {
  const t = new Turn(first, start).user(prompt)
  t.think('演示模式：这段回复由前端脚本生成，不是真实的 Agent。先看看项目结构，再回答。')
  t.tool('读取 README.md', 'read', { path: 'README.md' }, '# ora-space/cloud\n\nOra Cloud 与 v2 演示。', { path: 'README.md' })
  if (prompt.includes('失败')) {
    t.tool('运行 npm run check', 'execute', { command: 'npm run check' }, 'Error: PostgreSQL is not reachable at 127.0.0.1:55432', { fail: true, ms: 1400 })
    const partial = '检查没有跑完：PostgreSQL 连不上。下面的结论只覆盖'
    t.say(partial).end()
    return { steps: t.steps, result: { status: 'failed', answer: partial, answerComplete: false, stopReason: 'end_turn', failureCode: 'history_incomplete' } }
  }
  t.tool('搜索 "chat"', 'search', { pattern: 'chat' }, 'frontend-v2/src/features/chat/page.tsx\nv2-server/internal/chat/chat.go')
  if (prompt.includes('长')) {
    const long = Array.from({ length: 12 }, (_, i) => `${i + 1}. 第 ${i + 1} 点：这是一段很长的演示回答，用来试试回复中途停止。`).join('\n')
    t.say(long, 4, 160).end()
    return { steps: t.steps, result: done(long) }
  }
  const answer = `收到：「${prompt}」。\n\n这是演示后端的回复，用来展示对话页的完整流程：\n\n- 发出的消息先显示在线程里，历史快照到达后整份替换；\n- 工具调用先显示为进行中，定稿后折叠进活动块；\n- 回复逐步出现，定稿后替换预览，不会重复。\n\n在消息里写"长"可以试停止，写"失败"可以看失败的显示。`
  t.say(answer).end()
  return { steps: t.steps, result: done(answer) }
}

/** Settles a run's steps at once into records, last write per position: a finished history. */
export function settled(steps: Step[]): ChatHistoryRecord[] {
  const byPosition = new Map<number, ChatHistoryRecord>()
  for (const s of steps) if (!s.preview || !byPosition.has(s.position)) byPosition.set(s.position, { sequence: s.position, data: s.record })
  return [...byPosition.values()].sort((a, b) => a.sequence - b.sequence)
}
