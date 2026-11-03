import { childSessionOf } from './conversation'
import type { Meta } from './types'
import type { SessionEvent, SessionHistoryRecord } from './wire'

/** Sampled reasoning intervals, keyed by the Node history position within one session. */
export type ThoughtTimings = Record<number, { startedAt: number; endedAt?: number; source?: string }>

/**
 * Observe reasoning through retained server event times, never history append times or the
 * browser clock. Replaying those events restores the same estimate after a cold reload.
 * A settled thought alone cannot prove when it began, so only previews open an interval.
 */
export function observeThoughts(timings: ThoughtTimings, event: SessionEvent, settled: SessionHistoryRecord[]): ThoughtTimings {
  const at = Date.parse(event.occurredAt)
  if (!Number.isFinite(at)) return timings
  const next = { ...timings }
  const records = event.history?.records ?? []
  const settledPositions = new Set(settled.filter((record) => !record.preview).map((record) => record.sequence))
  for (const record of records) {
    const update = updateOf(record)
    if (record.preview && !settledPositions.has(record.sequence) && update?.sessionUpdate === 'agent_thought_chunk' && !isBlank(update) && !next[record.sequence]) {
      next[record.sequence] = { startedAt: at, source: childSessionOf(update._meta)?.id }
    }
  }
  for (const [position, timing] of Object.entries(next)) {
    if (timing.endedAt !== undefined) continue
    const movedOn = records.some((record) => {
      if (record.sequence <= Number(position)) return false
      if (record.data.type === 'turnEnded') return true
      const update = updateOf(record)
      if (update?.sessionUpdate === 'user_message_chunk') return true
      return update && !isBlank(update) && ['agent_thought_chunk', 'agent_message_chunk', 'tool_call'].includes(update.sessionUpdate)
        && childSessionOf(update._meta)?.id === timing.source
    })
    if (movedOn || event.kind === 'turn_ended' || event.kind === 'session_ended') {
      next[Number(position)] = { ...timing, endedAt: Math.max(timing.startedAt, at) }
    }
  }
  return next
}

interface TimingUpdate {
  sessionUpdate: string
  _meta?: Meta
  content?: { type: string; text?: string }
}

function isBlank(update: TimingUpdate): boolean {
  return update.content?.type === 'text' && update.content.text?.trim() === ''
}

function updateOf(record: SessionHistoryRecord): TimingUpdate | undefined {
  return record.data.type === 'update' ? record.data.update as TimingUpdate : undefined
}
