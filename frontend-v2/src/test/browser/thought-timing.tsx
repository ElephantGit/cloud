import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Thread } from '@/components/agent-thread/thread'
import { applySessionEvent, conversationOf, emptySessionView } from '@/components/agent-thread/model/history'
import type { SessionEvent } from '@/components/agent-thread/model/wire'
import '../../index.css'

// A retained server timeline: the Node flush gives all settled output the same append time.
const record = (sequence: number, sessionUpdate: string, text: string, preview = false) => ({
  sequence, preview, data: { type: 'update', at: '2026-11-03T00:00:10Z', update: { sessionUpdate, content: { type: 'text', text } } },
})
const prompt = record(1, 'user_message_chunk', 'hi')
const user = { ...prompt, data: { ...prompt.data, at: '2026-11-03T00:00:00Z' } }
const thought = record(2, 'agent_thought_chunk', '用户说了 hi，我会简短地问候并询问可以提供什么帮助。', true)
const answer = record(3, 'agent_message_chunk', '你好！有什么我可以帮你的吗？')
const events: SessionEvent[] = [
  { sequence: 1, occurredAt: '2026-11-03T00:00:02Z', kind: 'history_preview', turnId: 't', history: { records: [user, thought], incomplete: false } },
  { sequence: 2, occurredAt: '2026-11-03T00:00:07Z', kind: 'history_preview', turnId: 't', history: { records: [thought, { ...answer, preview: true }], incomplete: false } },
  { sequence: 3, occurredAt: '2026-11-03T00:00:10Z', kind: 'history_snapshot', turnId: 't', history: {
    records: [user, { ...thought, preview: false }, answer, { sequence: 4, data: { type: 'turnEnded', at: '2026-11-03T00:00:10Z', stop_reason: 'end_turn' } }], incomplete: false,
  } },
]

/** Browser fixture for replaying retained reasoning events through the production thread. */
export function Fixture() {
  const query = new URLSearchParams(location.search)
  const legacy = query.has('legacy')
  const [stage, setStage] = useState(query.has('complete') || legacy ? 3 : 1)
  const replay = legacy ? events.slice(-1) : events.slice(0, stage)
  const conversation = conversationOf([replay.reduce(applySessionEvent, emptySessionView('fixture'))])
  return <TooltipProvider>
    <main className="mx-auto flex h-screen max-w-4xl flex-col">
      <header className="flex items-center gap-4 border-b p-5">
        <h1 className="font-medium">思考耗时回归 · 固定事件时间线</h1>
        {!legacy && <>
          <button onClick={() => setStage(2)}>显示回答</button>
          <button onClick={() => { history.replaceState(null, '', '?complete=1'); setStage(3) }}>完成本轮</button>
        </>}
      </header>
      <Thread conversation={conversation} onAnswer={() => {}} />
    </main>
  </TooltipProvider>
}

createRoot(document.getElementById('root')!).render(<Fixture />)
