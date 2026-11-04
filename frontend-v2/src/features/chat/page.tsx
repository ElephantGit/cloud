import { SquarePenIcon } from 'lucide-react'
import { lazy, Suspense, useEffect, useLayoutEffect, useMemo, useRef } from 'react'
import { useSearchParams } from 'react-router'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { useUser } from '@/app/auth/user'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Composer } from './components/composer'
import { ContextBar } from './components/context-bar'
import { FadeText } from './components/fade-text'
import { subtleScrollbar } from '@/lib/scrollbar'
import { ShowSidebarButton, Sidebar } from './components/sidebar'
import { Notice } from './components/notice'
import { SandboxLabel } from './components/sandbox-label'
import { ContextUsage, LandingHeading, Thread } from '@/components/agent-thread/thread'
import { isResponding } from '@/components/agent-thread/model/conversation'
import { emptyConversation, type PluginRef } from '@/components/agent-thread/model/types'
import { WorkspacePanel, WorkspaceToggle } from '@/components/workspace-panel/workspace-panel'
import { WorkspaceNavigationProvider } from '@/components/workspace-panel/navigation'
import { WorkspaceSplit } from '@/components/workspace-panel/workspace-split'
import { conversationRevision } from '@/components/workspace-panel/revision'
import { useWorkspacePanel } from '@/components/workspace-panel/use-panel'
import { type Agent, agentsOf, emptyDraft, isEnded, newChatKey, type Session, useChatStore } from './store'

/** The run view brings the graph canvas, so it loads only once a run is opened. */
const RunPane = lazy(() => import('./workflow-run/run-pane'))

/** Shared so a session with nothing queued keeps a stable selector result. */
const noQueue: string[] = []
/** The backend takes text only: no plugins to offer, and no permission requests to answer. */
const noPlugins: PluginRef[] = []
const noAnswer = () => {}

/** How the input slides from the middle of the page to the bottom when the first prompt goes out. */
const slide = { duration: 420, easing: 'cubic-bezier(0.32, 0.72, 0, 1)' }

/**
 * Chat: history on the left; on the right the selected session, a picked workflow run, or a new
 * chat when neither is. Source links select a conversation or workflow run after loading it.
 */
export default function Page() {
  const user = useUser()
  const session = useChatStore((s) => s.sessions.find((x) => x.id === s.selectedId))
  const runId = useChatStore((s) => s.selectedRunId)
  const tenants = useGetApiV1MeTenants()
  const tenantId = tenants.data?.items[0]?.id
  const connected = useChatStore((s) => !!tenantId && s.tenantId === tenantId && s.userId === user.id)
  const [params, setParams] = useSearchParams()
  const linkedRun = params.get('run')
  const linkedConversation = params.get('conversation')
  const linkedChat = useChatStore((s) => s.sessions.find((x) => x.id === linkedConversation))
  const loading = useChatStore((s) => s.backend === 'loading' || s.backend === 'idle')
  useEffect(() => {
    if (tenantId) void useChatStore.getState().connect(tenantId, user.id)
  }, [tenantId, user.id])
  useEffect(() => {
    if (!connected || !linkedRun || linkedConversation) return
    useChatStore.getState().selectRun(linkedRun)
    setParams(
      (p) => {
        p.delete('run')
        return p
      },
      { replace: true },
    )
  }, [connected, linkedRun, linkedConversation, setParams])
  useEffect(() => {
    if (!connected || !linkedConversation || !linkedChat) return
    useChatStore.getState().select(linkedConversation)
    setParams((p) => {
      p.delete('conversation')
      p.delete('run')
      return p
    }, { replace: true })
  }, [connected, linkedConversation, linkedChat, setParams])
  if (tenants.isError) {
    return (
      <div role="alert" className="flex min-h-0 flex-1 items-center justify-center gap-3 p-6 text-sm text-muted-foreground">
        <span>无法加载工作区。</span>
        <Button size="sm" variant="outline" onClick={() => void tenants.refetch()}>重试</Button>
      </div>
    )
  }
  if (!tenantId) {
    return (
      <div role="status" className="flex min-h-0 flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
        {tenants.isPending ? '正在加载工作区…' : '你还没有可用的工作区。'}
      </div>
    )
  }
  if (!connected || (linkedConversation && loading)) {
    return (
      <div role="status" className="flex min-h-0 flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
        正在加载对话…
      </div>
    )
  }
  if (linkedConversation && !linkedChat) {
    return (
      <div className="space-y-3 p-6">
        <p role="alert">此对话已删除或无权访问。</p>
        <Button variant="outline" onClick={() => setParams({})}>返回对话</Button>
      </div>
    )
  }
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
        <ChatWithWorkspace session={session} tenantId={tenantId} />
      )}
    </div>
  )
}

/**
 * The chat and, for a conversation the backend runs, the side panel with its checkout. The panel
 * remembers whether it was open, and a turn's changed file opens it on that file's diff.
 */
function ChatWithWorkspace({ session, tenantId }: { session?: Session; tenantId: string }) {
  const panel = useWorkspacePanel()
  const conversationId = session?.remote ? session.id : undefined
  const source = useMemo(() => (conversationId ? { kind: 'chat' as const, tenantId, conversationId } : undefined), [conversationId, tenantId])
  const revision = useChatStore((s) => (conversationId && s.conversations[conversationId] ? conversationRevision(s.conversations[conversationId]) : ''))
  const shown = source !== undefined && panel.open
  return (
    <WorkspaceNavigationProvider value={source ? panel.navigation : null}>
      <WorkspaceSplit
        id="chat"
        panel={shown && <WorkspacePanel key={source.conversationId} source={source} revision={revision} request={panel.request} onClose={() => panel.setOpen(false)} />}
      >
        <section className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label="会话">
          <ChatPane session={session} workspace={source ? { open: panel.open, toggle: () => panel.setOpen(!panel.open) } : undefined} />
        </section>
      </WorkspaceSplit>
    </WorkspaceNavigationProvider>
  )
}

/**
 * One pane for a session and for a new chat, so the input stays mounted when the new chat's
 * first prompt creates its session. Until a chat has a prompt its heading and input sit together
 * in the middle of the page, as in desktop; the first prompt slides the input to the bottom.
 */
function ChatPane({ session, workspace }: { session?: Session; workspace?: { open: boolean; toggle: () => void } }) {
  const key = session?.id ?? newChatKey
  const conversation = useChatStore((s) => (session && s.conversations[session.id]) || emptyConversation)
  const draft = useChatStore((s) => s.drafts[key] ?? emptyDraft)
  const queued = useChatStore((s) => (session && s.queued[session.id]) || noQueue)
  const newChat = useChatStore((s) => s.newChat)
  const newChatOpens = useChatStore((s) => s.newChatOpens)
  const projects = useChatStore((s) => s.projects)
  const executors = useChatStore((s) => s.executors)
  const backend = useChatStore((s) => s.backend)
  const notice = useChatStore((s) => s.notices[key])
  // A backend conversation whose session ended (stopped, or the server restarted) is read-only.
  const ended = useChatStore((s) => !!session?.remote && isEnded(s.views[session.id]))
  const executorUnavailable = !!session?.executorUnavailable
  const { send, sendNewChat, startNewChat, stop, setNewChat, setDraft, unqueue, dismiss, createProject } = useChatStore.getState()
  const landing = conversation.turns.length === 0
  // A conversation runs on the backend, text only, and keeps the agent and model it started with.
  const agents: Agent[] = session ? [{ id: session.agent, name: session.agent, models: [session.model] }] : agentsOf(executors)

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
        {session && <SandboxLabel />}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <ContextUsage conversation={conversation} />
          {workspace && <WorkspaceToggle {...workspace} />}
        </div>
      </header>
      <main className={cn('flex min-h-0 flex-1 flex-col', landing && ['overflow-y-auto', subtleScrollbar])}>
        {landing ? (
          // mt-auto here and mb-auto on the input split the free space, centring the pair.
          <div className="mt-auto w-full px-3 pt-10 sm:px-6">
            <div className="mx-auto w-full max-w-[760px]">
              <LandingHeading />
            </div>
          </div>
        ) : (
          <div className="min-h-0 flex-1">
            <Thread key={key} conversation={conversation} onAnswer={noAnswer} />
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
              <p role="status" className="mb-2 rounded-xl border border-dashed px-3 py-2 text-xs text-muted-foreground">
                {backend === 'loading' ? '正在连接后端…' : '后端还不能运行 Agent，暂时无法开始新对话。'}
              </p>
            )}
            {notice && !ended && !executorUnavailable && <Notice notice={notice} onDismiss={() => dismiss(key)} />}
            {session && (executorUnavailable || ended) ? (
              <div role={executorUnavailable ? 'alert' : 'status'} className="flex items-center gap-3 rounded-2xl border bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
                <p className="min-w-0 flex-1">
                  {executorUnavailable
                    ? '原来的 Agent 或模型已不可用，这个对话不能继续。请新开对话并选择当前可用的模型。'
                    : '这个对话已结束，Agent 不能继续。要接着做，请新开一个对话。'}
                </p>
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
              onChoose={(agent, model) => !session && setNewChat({ agent, model })}
              plugins={noPlugins}
              textOnly
              focusKey={session ? undefined : newChatOpens}
              context={!session && <ContextBar value={newChat} projects={projects} onChange={setNewChat} onCreateProject={createProject} />}
            />
            )}
          </div>
        </div>
      </main>
    </>
  )
}
