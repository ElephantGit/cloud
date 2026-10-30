import { FlaskConicalIcon, RotateCcwIcon, SquarePenIcon } from 'lucide-react'
import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Composer } from './components/composer'
import { ContextBar } from './components/context-bar'
import { FadeText } from './components/fade-text'
import { subtleScrollbar } from '@/lib/scrollbar'
import { ShowSidebarButton, Sidebar } from './components/sidebar'
import { Notice } from './components/notice'
import { WorktreeLabel } from './components/worktree-label'
import { ContextUsage, LandingHeading, Thread } from './components/thread'
import { isResponding } from './model/conversation'
import { emptyConversation } from './model/types'
import { scenarios } from './samples/scenarios'
import { type Agent, agentsOf, emptyDraft, isEnded, newChatKey, sampleAgents, samplePlugins, type Session, useChatStore } from './store'

/** The run view brings the graph canvas, so it loads only once a run is opened. */
const RunPane = lazy(() => import('./workflow-run/run-pane'))

/** Shared so a session with nothing queued keeps a stable selector result. */
const noQueue: string[] = []

/** How the input slides from the middle of the page to the bottom when the first prompt goes out. */
const slide = { duration: 420, easing: 'cubic-bezier(0.32, 0.72, 0, 1)' }

/**
 * Chat: history on the left; on the right the selected session, a picked workflow run, or a new
 * chat when neither is.
 */
export default function Page() {
  const session = useChatStore((s) => s.sessions.find((x) => x.id === s.selectedId))
  const runId = useChatStore((s) => s.selectedRunId)
  const tenantId = useGetApiV1MeTenants().data?.items[0]?.id
  useEffect(() => {
    if (tenantId) void useChatStore.getState().connect(tenantId)
  }, [tenantId])
  return (
    <div className="flex min-h-0 flex-1">
      <Sidebar />
      {runId ? (
        <section className="flex min-w-0 flex-1 flex-col" aria-label="工作流运行">
          <Suspense>
            <RunPane runId={runId} />
          </Suspense>
        </section>
      ) : (
        <section className="flex min-w-0 flex-1 flex-col" aria-label="会话">
          <ChatPane session={session} />
        </section>
      )}
    </div>
  )
}

/**
 * One pane for a session and for a new chat, so the input stays mounted when the new chat's
 * first prompt creates its session. Until a chat has a prompt its heading and input sit together
 * in the middle of the page, as in desktop; the first prompt slides the input to the bottom.
 */
function ChatPane({ session }: { session?: Session }) {
  const key = session?.id ?? newChatKey
  const conversation = useChatStore((s) => (session && s.conversations[session.id]) || emptyConversation)
  const draft = useChatStore((s) => s.drafts[key] ?? emptyDraft)
  const queued = useChatStore((s) => (session && s.queued[session.id]) || noQueue)
  const playing = useChatStore((s) => !!(session && s.playing[session.id]))
  const newChat = useChatStore((s) => s.newChat)
  const projects = useChatStore((s) => s.projects)
  const executors = useChatStore((s) => s.executors)
  const backend = useChatStore((s) => s.backend)
  const notice = useChatStore((s) => s.notices[key])
  // A backend conversation whose session ended (stopped, or the server restarted) is read-only.
  const ended = useChatStore((s) => !!session?.remote && isEnded(s.views[session.id]))
  const { send, sendNewChat, startNewChat, stop, answer, setAgentModel, setNewChat, setDraft, unqueue, replay, dismiss } = useChatStore.getState()
  const onAnswer = useCallback((permissionId: string, optionId: string) => session && answer(session.id, permissionId, optionId), [answer, session])
  const scenario = scenarios.find((s) => s.id === session?.scenarioId)
  const landing = conversation.turns.length === 0
  // Samples play in the browser; everything else runs on the backend, text only, and keeps the
  // agent and model it started with.
  const local = !!session?.scenarioId
  const agents: Agent[] = local ? sampleAgents : session ? [{ id: session.agent, name: session.agent, models: [session.model] }] : agentsOf(executors)

  // FLIP: where the input was when the first prompt left; once the layout has switched, it is
  // offset back there and animated home.
  const slot = useRef<HTMLDivElement>(null)
  const slideFrom = useRef<{ top: number; at: number }>(undefined)
  useLayoutEffect(() => {
    const from = slideFrom.current
    if (!from || landing || !slot.current) return
    slideFrom.current = undefined
    if (performance.now() - from.at > 1000 || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const dy = from.top - slot.current.getBoundingClientRect().top
    if (dy !== 0) slot.current.animate?.([{ transform: `translateY(${dy}px)` }, { transform: 'translateY(0)' }], slide)
  })

  return (
    <>
      <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
        <ShowSidebarButton />
        <h2 className="flex min-w-0 text-sm font-medium">
          <FadeText text={session?.title ?? '新对话'} />
        </h2>
        {session && <WorktreeLabel />}
        {scenario && (
          <Badge variant="outline" className="gap-1 text-muted-foreground">
            <FlaskConicalIcon />
            样例：{scenario.covers}
          </Badge>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {import.meta.env.VITE_CHAT_DEMO === '1' && !local && (
            <Badge variant="outline" className="text-muted-foreground" title="对话和回复由浏览器里的演示后端生成，不经过 v2-server，也不运行 Agent">
              演示模式
            </Badge>
          )}
          <ContextUsage conversation={conversation} />
          {session && scenario && (
            <Button variant="ghost" size="sm" onClick={() => replay(session.id)}>
              <RotateCcwIcon />
              重新播放
            </Button>
          )}
        </div>
      </header>
      <main className={cn('flex min-h-0 flex-1 flex-col', landing && ['overflow-y-auto', subtleScrollbar])} data-playing={playing || undefined}>
        {landing ? (
          // mt-auto here and mb-auto on the input split the free space, centring the pair.
          <div className="mt-auto w-full px-3 pt-10 sm:px-6">
            <div className="mx-auto w-full max-w-[760px]">
              <LandingHeading />
            </div>
          </div>
        ) : (
          <div className="min-h-0 flex-1">
            <Thread key={key} conversation={conversation} onAnswer={onAnswer} />
          </div>
        )}
        <div
          ref={slot}
          className={
            landing
              ? 'relative mb-auto w-full px-3 pb-10 sm:px-6'
              : 'relative shrink-0 bg-gradient-to-t from-background via-background to-transparent px-3 pt-2 pb-4 sm:px-5'
          }
        >
          <div className="mx-auto w-full max-w-[760px]">
            {!session && (backend === 'unavailable' || backend === 'loading') && (
              <p role="status" className="mb-2 rounded-lg border border-dashed px-3 py-2 text-xs text-muted-foreground">
                {backend === 'loading' ? '正在连接后端…' : '后端还不能运行 Agent，暂时无法开始新对话。左栏的示例不依赖后端，可以先看看。'}
              </p>
            )}
            {notice && !ended && <Notice notice={notice} onDismiss={() => dismiss(key)} />}
            {session && ended ? (
              <div role="status" className="flex items-center gap-3 rounded-2xl border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
                <p className="min-w-0 flex-1">这个对话已结束，Agent 不能继续。要接着做，请新开一个对话。</p>
                <Button size="sm" variant="outline" className="shrink-0" onClick={() => startNewChat({ projectId: session.projectId })}>
                  <SquarePenIcon />
                  新对话
                </Button>
              </div>
            ) : (
            <Composer
              draft={draft}
              onDraft={(d) => setDraft(key, d)}
              onSend={(d) => {
                if (landing && slot.current) slideFrom.current = { top: slot.current.getBoundingClientRect().top, at: performance.now() }
                if (session) {
                  send(session.id, d)
                  setDraft(key, emptyDraft)
                } else void sendNewChat(d)
              }}
              onStop={() => session && stop(session.id)}
              responding={isResponding(conversation)}
              queued={queued}
              onUnqueue={(i) => session && unqueue(session.id, i)}
              agents={agents}
              agent={session?.agent ?? newChat.agent}
              model={session?.model ?? newChat.model}
              onChoose={(agent, model) => (session ? setAgentModel(session.id, agent, model) : setNewChat({ agent, model }))}
              plugins={samplePlugins}
              textOnly={!local}
              context={!session && <ContextBar value={newChat} projects={projects.filter((p) => !p.samples)} branches={[]} onChange={setNewChat} />}
            />
            )}
          </div>
        </div>
      </main>
    </>
  )
}
