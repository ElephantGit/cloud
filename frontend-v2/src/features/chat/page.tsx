import { FlaskConicalIcon, FolderIcon, GitBranchIcon, RotateCcwIcon } from 'lucide-react'
import { useCallback, useLayoutEffect, useRef } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Composer } from './components/composer'
import { ContextBar } from './components/context-bar'
import { FadeText } from './components/fade-text'
import { subtleScrollbar } from './components/scrollbar'
import { ShowSidebarButton, Sidebar } from './components/sidebar'
import { ContextUsage, LandingHeading, Thread } from './components/thread'
import { isResponding } from './model/conversation'
import { emptyConversation } from './model/types'
import { scenarios } from './samples/scenarios'
import { agents, emptyDraft, newChatKey, plugins, type Session, useChatStore } from './store'

/** Shared so a session with nothing queued keeps a stable selector result. */
const noQueue: string[] = []

/** How the input slides from the middle of the page to the bottom when the first prompt goes out. */
const slide = { duration: 420, easing: 'cubic-bezier(0.32, 0.72, 0, 1)' }

/** Chat: history on the left; on the right the selected session, or a new chat when none is. */
export default function Page() {
  const session = useChatStore((s) => s.sessions.find((x) => x.id === s.selectedId))
  return (
    <div className="flex min-h-0 flex-1">
      <Sidebar />
      <section className="flex min-w-0 flex-1 flex-col" aria-label="会话">
        <ChatPane session={session} />
      </section>
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
  const sessions = useChatStore((s) => s.sessions)
  const { send, sendNewChat, stop, answer, setAgentModel, setNewChat, createProject, setDraft, unqueue, replay } = useChatStore.getState()
  const onAnswer = useCallback((permissionId: string, optionId: string) => session && answer(session.id, permissionId, optionId), [answer, session])
  const scenario = scenarios.find((s) => s.id === session?.scenarioId)
  const landing = conversation.turns.length === 0

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

  const branches = [
    ...new Set(
      sessions.flatMap((s) => (s.projectId === newChat.projectId && s.workspace.kind === 'isolated' ? [s.workspace.branch] : [])),
    ),
  ]

  return (
    <>
      <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
        <ShowSidebarButton />
        <h2 className="flex min-w-0 text-sm font-medium">
          <FadeText text={session?.title ?? '新对话'} />
        </h2>
        {session && <WorkspaceBadge session={session} />}
        {scenario && (
          <Badge variant="outline" className="gap-1 text-muted-foreground">
            <FlaskConicalIcon />
            样例：{scenario.covers}
          </Badge>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1">
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
            <Composer
              draft={draft}
              onDraft={(d) => setDraft(key, d)}
              onSend={(d) => {
                if (landing && slot.current) slideFrom.current = { top: slot.current.getBoundingClientRect().top, at: performance.now() }
                if (session) {
                  send(session.id, d)
                  setDraft(key, emptyDraft)
                } else sendNewChat(d)
              }}
              onStop={() => session && stop(session.id)}
              responding={isResponding(conversation)}
              queued={queued}
              onUnqueue={(i) => session && unqueue(session.id, i)}
              agents={agents}
              agent={session?.agent ?? newChat.agent}
              model={session?.model ?? newChat.model}
              onChoose={(agent, model) => (session ? setAgentModel(session.id, agent, model) : setNewChat({ agent, model }))}
              plugins={plugins}
              context={
                !session && (
                  <ContextBar
                    value={newChat}
                    projects={projects.filter((p) => !p.samples)}
                    branches={branches}
                    onChange={setNewChat}
                    onCreateProject={(path) => setNewChat({ projectId: createProject(path), workspace: { kind: 'main' } })}
                  />
                )
              }
            />
          </div>
        </div>
      </main>
    </>
  )
}

function WorkspaceBadge({ session }: { session: Session }) {
  if (session.workspace.kind === 'isolated') {
    return (
      <Badge variant="secondary" className="gap-1 font-mono" title="Agent 在独立的 worktree 里工作">
        <GitBranchIcon />
        {session.workspace.branch}
      </Badge>
    )
  }
  return (
    <Badge variant="secondary" className="gap-1" title="Agent 直接在项目主目录里工作">
      <FolderIcon />
      主目录
    </Badge>
  )
}
