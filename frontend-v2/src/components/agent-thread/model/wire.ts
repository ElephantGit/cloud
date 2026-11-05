/**
 * The session event stream as v2-server sends it, for a chat conversation, a workflow node and a
 * task run alike: all three are sessions of the shared execution service. The chat contract's
 * generated types have the same shape; these keep the shared view free of any one page's client.
 */

/** A turn's result; failureCode is a stable code, never provider text. */
export interface SessionResult {
  status: 'completed' | 'cancelled' | 'failed' | 'interrupted'
  answer: string
  answerComplete: boolean
  stopReason: string
  failureCode: string
  usage?: { [key: string]: unknown }
}

/** One record of the fixed Node's history, at its position; preview records are provisional. */
export interface SessionHistoryRecord {
  sequence: number
  data: { [key: string]: unknown }
  preview?: boolean
}

/** Why the page ended the session; empty when it ended on its own (a restart) or is live. */
export type SessionEndReason = '' | 'stopped' | 'deleted' | 'interrupted'

/** One timed stage of a session's path to first output; endedAt is absent while it runs. */
export interface SessionPreparationStage {
  id: 'runtime' | 'workspace' | 'plugin' | 'agent' | 'response'
  startedAt: string
  endedAt?: string
  /** Work prepared in advance served this stage. */
  reused?: boolean
}

/** What a session's first turn waited for; each stage starts when the previous one ends. */
export interface SessionPreparation {
  stages: SessionPreparationStage[]
}

/** The data of one event-stream message. */
export interface SessionEvent {
  sequence: number
  occurredAt: string
  kind: 'history_snapshot' | 'history_preview' | 'turn_ended' | 'text_delta' | 'session_ended' | 'preparation'
  turnId: string
  history?: { records: SessionHistoryRecord[]; incomplete: boolean }
  textDelta?: string
  result?: SessionResult
  /** On turn_ended and preparation, the text the turn was asked, when it could be read. */
  prompt?: string
  /** On preparation, the stages so far; it replaces the previous set. */
  preparation?: SessionPreparation
}
