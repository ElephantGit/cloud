import { useEffect, useMemo, useState } from 'react'
import { applySessionEvent, conversationOf, emptySessionView, type SessionView } from '@/components/agent-thread/model/history'
import type { Conversation } from '@/components/agent-thread/model/types'
import type { SessionEvent } from '@/components/agent-thread/model/wire'

/**
 * Follows one task run's session over v2-server's run events stream and folds it as the chat page
 * folds a conversation, so every member sees the Agent's thinking, tool calls and answers live.
 * The stream is the starter's session read on the reader's behalf; `conversation` is null until
 * the first event arrives, and `refused` says the server turned the stream away for good.
 */
export function useRunSession(tenantId: string, taskId: string, runId: string | undefined): { conversation: Conversation | null; refused: boolean } {
  const [view, setView] = useState<SessionView | null>(null)
  const [refused, setRefused] = useState(false)
  useEffect(() => {
    if (tenantId === '' || runId === undefined) return
    let current = emptySessionView(runId)
    const source = new EventSource(`/api/v2/tenants/${tenantId}/tasks/${taskId}/runs/${runId}/events`)
    source.onmessage = (m: MessageEvent<string>) => {
      current = applySessionEvent(current, JSON.parse(m.data) as SessionEvent)
      setView(current)
    }
    // The session has sent everything it ever will, or cannot be read on: stop reconnecting.
    source.addEventListener('end', () => source.close())
    source.addEventListener('failure', () => source.close())
    source.onerror = () => {
      if (source.readyState === EventSource.CLOSED && current.cursor === 0) setRefused(true)
    }
    return () => {
      source.close()
      setView(null)
      setRefused(false)
    }
  }, [tenantId, taskId, runId])
  const conversation = useMemo(() => {
    if (view === null) return null
    // The run card says how and why the session ended; chat's "cannot continue" marker would not.
    const folded = conversationOf([view])
    return { ...folded, markers: folded.markers.filter(marker => marker.kind !== 'session') }
  }, [view])
  return { conversation, refused }
}
