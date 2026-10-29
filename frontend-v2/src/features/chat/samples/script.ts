import type * as acp from '@agentclientprotocol/sdk'
import type { Attachment, ChatEvent, PluginRef } from '../model/types'

/**
 * One step of a recorded conversation. Replaying a script feeds its events through the same
 * reducer live sessions use, so a sample that renders right shows the mapping is right.
 */
export type Step =
  | { kind: 'event'; delay: number; event: ChatEvent }
  | { kind: 'sleep'; delay: number }
  /** A prompt the person typed while the agent was busy; it waits above the input. */
  | { kind: 'queue'; delay: number; text: string }
  /** Stops until the person answers permission `id`, then plays the branch for their answer. */
  | { kind: 'await'; id: string; branches: Record<string, Step[]> }

interface ToolCall {
  id: string
  title: string
  kind: acp.ToolKind
  input?: unknown
  locations?: acp.ToolCallLocation[]
  meta?: Record<string, unknown>
}

/** How long the agent takes per streamed chunk, and how big the chunks are. */
const chunkDelay = 18
const chunkSize = 6

/**
 * Builds a script the way a person describes a conversation: who said what, what the agent
 * thought and which tools it ran. Delays are milliseconds after the previous step.
 */
export class Script {
  readonly steps: Step[] = []
  private turns = 0
  private meta?: Record<string, unknown>

  /** A prompt from the person; starts a turn. */
  user(text: string, extra: { attachments?: Attachment[]; plugins?: PluginRef[] } = {}, delay = 400) {
    this.turns++
    return this.event({ type: 'prompt', turnId: `turn-${this.turns}`, text, ...extra }, delay)
  }

  /** Agent text, streamed in small chunks like a model would. */
  say(text: string, messageId?: string) {
    return this.stream('agent_message_chunk', text, messageId)
  }

  /** Agent reasoning, streamed. */
  think(text: string) {
    return this.stream('agent_thought_chunk', text)
  }

  /** A non-text block in the agent's message stream, such as a resource link. */
  block(content: Exclude<acp.ContentBlock, { type: 'text' }>, delay = 60) {
    return this.update({ sessionUpdate: 'agent_message_chunk', content }, delay)
  }

  /** Starts a tool call; finish it with `done`, `fail` or `progress`. */
  tool(call: ToolCall, delay = 250) {
    this.update(
      {
        sessionUpdate: 'tool_call',
        toolCallId: call.id,
        title: call.title,
        kind: call.kind,
        status: 'in_progress',
        rawInput: call.input,
        locations: call.locations,
        _meta: call.meta,
      },
      delay,
    )
    return this
  }

  /** Reports a tool call's success after `after` milliseconds. */
  done(id: string, result: { output?: unknown; content?: acp.ToolCallContent[]; title?: string } = {}, after = 700) {
    return this.update(
      {
        sessionUpdate: 'tool_call_update',
        toolCallId: id,
        status: 'completed',
        rawOutput: result.output,
        content: result.content,
        title: result.title,
      },
      after,
    )
  }

  /** Reports a tool call's failure, with the error as its text content. */
  fail(id: string, error: string, after = 700, output?: unknown) {
    return this.update(
      {
        sessionUpdate: 'tool_call_update',
        toolCallId: id,
        status: 'failed',
        content: [{ type: 'content', content: { type: 'text', text: error } }],
        rawOutput: output,
      },
      after,
    )
  }

  /** The agent's whole plan, as ACP sends it each time it changes. */
  plan(entries: [string, acp.PlanEntryStatus][], delay = 200) {
    return this.update(
      { sessionUpdate: 'plan', entries: entries.map(([content, status]) => ({ content, status, priority: 'medium' })) },
      delay,
    )
  }

  /** The context window the agent reports after a step. */
  usage(used: number, size = 200_000, delay = 0) {
    return this.update({ sessionUpdate: 'usage_update', used, size }, delay)
  }

  /** Asks the person for permission and waits; each branch plays after the matching answer. */
  ask(id: string, toolCall: acp.ToolCallUpdate, options: acp.PermissionOption[], branches: Record<string, (s: Script) => void>) {
    this.event({ type: 'permission', request: { id, toolCall, options } }, 300)
    const played: Record<string, Step[]> = {}
    for (const [optionId, build] of Object.entries(branches)) {
      const branch = new Script()
      branch.turns = this.turns
      branch.meta = this.meta
      build(branch)
      played[optionId] = [{ kind: 'event', delay: 0, event: { type: 'permissionResolved', id, optionId } }, ...branch.steps]
    }
    this.steps.push({ kind: 'await', id, branches: played })
    return this
  }

  /** A prompt queued while the agent works; the next `user` call sends it. */
  queue(text: string, delay = 300) {
    this.steps.push({ kind: 'queue', delay, text })
    return this
  }

  /** Ends the turn. */
  end(stopReason: acp.StopReason = 'end_turn', usage?: acp.Usage, delay = 200) {
    return this.event({ type: 'turnEnd', stopReason, usage }, delay)
  }

  /** Fails the turn, as a broken stream or unavailable model would. */
  error(message: string, delay = 400) {
    return this.event({ type: 'turnError', message }, delay)
  }

  /** Records a switch to another model between turns. */
  model(model: string) {
    return this.event({ type: 'model', model }, 200)
  }

  /** Records a restorable point between turns. */
  checkpoint(label: string) {
    return this.event({ type: 'checkpoint', label }, 100)
  }

  /** Waits without doing anything. */
  sleep(ms: number) {
    this.steps.push({ kind: 'sleep', delay: ms })
    return this
  }

  /** Runs `build` with every update it adds attributed to child session `id` of `parentId`. */
  as(id: string, parentId: string, title: string, build: (s: Script) => void) {
    const outer = this.meta
    this.meta = { 'opencode/child-session': { id, parentID: parentId, depth: 1, title } }
    build(this)
    this.meta = outer
    return this
  }

  private stream(kind: 'agent_message_chunk' | 'agent_thought_chunk', text: string, messageId?: string) {
    for (let i = 0; i < text.length; i += chunkSize) {
      this.update(
        { sessionUpdate: kind, content: { type: 'text', text: text.slice(i, i + chunkSize) }, messageId },
        i === 0 ? 250 : chunkDelay,
      )
    }
    return this
  }

  private update(update: acp.SessionUpdate, delay: number) {
    const withMeta = this.meta && 'sessionUpdate' in update ? ({ ...update, _meta: { ...this.meta, ...(update as { _meta?: object })._meta } } as acp.SessionUpdate) : update
    return this.event({ type: 'update', update: withMeta }, delay)
  }

  private event(event: ChatEvent, delay: number) {
    this.steps.push({ kind: 'event', delay, event })
    return this
  }
}

/** Starts a script. */
export const script = () => new Script()
