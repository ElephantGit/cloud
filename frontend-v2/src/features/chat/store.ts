import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { SidebarLayout } from '@/components/sidebar-resize-handle'
import { branchNameFor } from './model/branch-name'
import { applyEvent, isResponding } from './model/conversation'
import { type Attachment, type ChatEvent, type Conversation, emptyConversation, type PluginRef } from './model/types'
import { scenarios } from './samples/scenarios'
import { script, type Step } from './samples/script'

/** Where a session's agent works: the project's main checkout, or one of its worktrees. */
export type Workspace = { kind: 'main' } | { kind: 'isolated'; branch: string }

/** Where a new chat will work: like a session, or in a worktree its first prompt will name. */
export type NewChatWorkspace = Workspace | { kind: 'new-worktree' }

export interface Project {
  id: string
  name: string
  /** Where the project's main checkout lives, for projects the person added. */
  path?: string
  /** Samples replay recorded conversations; the person cannot add sessions to them. */
  samples?: boolean
}

export interface Session {
  id: string
  projectId: string
  workspace: Workspace
  title: string
  /** The agent's id in {@link agents}, and the model it runs with. */
  agent: string
  model: string
  /** The recorded conversation this session replays, for sample sessions. */
  scenarioId?: string
  createdAt: number
}

export interface Draft {
  text: string
  attachments: Attachment[]
  plugins: PluginRef[]
}

/** A session's input before anything is typed. */
export const emptyDraft: Draft = { text: '', attachments: [], plugins: [] }

/** Where a chat that has not been sent yet will start: the session is created by its first prompt. */
export interface NewChat {
  projectId: string
  workspace: NewChatWorkspace
  agent: string
  model: string
}

/** The key the new chat's draft is kept under, next to the sessions' drafts. */
export const newChatKey = 'new'

/** An agent the cloud can run, and the models it offers. */
export interface Agent {
  id: string
  name: string
  models: string[]
}

/**
 * The agents and models the selector offers until v2-server reports which agents it runs and
 * their models; the samples all use the first one.
 */
export const agents: Agent[] = [
  { id: 'claude-code', name: 'Claude Code', models: ['Claude Sonnet 5.5', 'Claude Opus 5.5', 'Claude Haiku 5.5'] },
  { id: 'codex', name: 'Codex', models: ['GPT-5.5', 'GPT-5.5 Mini'] },
  { id: 'opencode', name: 'OpenCode', models: ['Claude Sonnet 5.5', 'GPT-5.5', 'Qwen3 Coder'] },
]

const defaultChoice = { agent: agents[0].id, model: agents[0].models[0] }
const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? id

/** Plugins the input can apply until the catalog comes from the backend. */
export const plugins: PluginRef[] = [
  { id: 'ui-guidelines', name: 'UI 规范' },
  { id: 'code-review', name: '代码审查' },
  { id: 'git-helper', name: 'Git 助手' },
]

/** How the history column is laid out; kept per browser. */
export const defaultSidebar: SidebarLayout = { width: 288, open: true }

interface State {
  projects: Project[]
  sessions: Session[]
  conversations: Record<string, Conversation>
  /** Prompts typed while the agent was busy, sent in order once it is free. */
  queued: Record<string, string[]>
  /** Sessions that finished a turn the person has not looked at yet. */
  unread: Record<string, true>
  /** What the person has typed but not sent, per session. */
  drafts: Record<string, Draft>
  /** Sessions whose recording (or stand-in reply) is still playing, including while it waits on an answer. */
  playing: Record<string, true>
  selectedId?: string
  /** The workflow run shown instead of a session, picked from a project's runs. */
  selectedRunId?: string
  /** The history column's width in pixels, and whether it is shown. */
  sidebar: SidebarLayout
  setSidebar: (sidebar: Partial<SidebarLayout>) => void
  select: (id: string) => void
  /** Shows a workflow run in place of the session, as desktop's workspace does. */
  selectRun: (id: string | undefined) => void
  /** What the new chat view starts; shown whenever no session is selected. */
  newChat: NewChat
  /**
   * Opens the new chat view. Without a scope it starts where the selected session works, so
   * "new chat" from inside a worktree stays in that worktree.
   */
  startNewChat: (scope?: { projectId: string; workspace?: NewChatWorkspace }) => void
  setNewChat: (patch: Partial<NewChat>) => void
  /**
   * Creates the session the new chat describes, selects it and sends its first prompt. A new
   * worktree is named from that prompt.
   */
  sendNewChat: (draft: Draft) => void
  createSession: (projectId: string, workspace: Workspace, choice?: { agent: string; model: string }) => string
  renameSession: (id: string, title: string) => void
  deleteSession: (id: string) => void
  /** Moves project `id` to where `overId` is, as a drag and drop does. */
  moveProject: (id: string, overId: string) => void
  send: (id: string, draft: Draft) => void
  stop: (id: string) => void
  answer: (id: string, permissionId: string, optionId: string) => void
  /** Switches a session's agent and model; once it has turns the switch is marked in the thread. */
  setAgentModel: (id: string, agent: string, model: string) => void
  /** Adds a project for the folder at `path`, named after its last segment; returns its id. */
  createProject: (path: string) => string
  setDraft: (id: string, draft: Draft) => void
  /** Removes the queued prompt at `index`. */
  unqueue: (id: string, index: number) => void
  /** Plays a sample session's recording again from the start. */
  replay: (id: string) => void
}

const sampleProject: Project = { id: 'samples', name: '示例', samples: true }

/**
 * The demo projects shown until the session API exists in v2-server; sending in them gets a
 * stand-in reply that says so.
 */
const demoProjects: Project[] = [
  { id: 'cloud', name: 'ora-space/cloud' },
  { id: 'desktop', name: 'ora-space/desktop' },
]

const sampleSessions: Session[] = scenarios.map((s, i) => ({
  id: `sample-${s.id}`,
  projectId: sampleProject.id,
  workspace: s.worktree ? { kind: 'isolated', branch: s.worktree } : { kind: 'main' },
  title: s.title,
  ...defaultChoice,
  scenarioId: s.id,
  createdAt: i,
}))

const demoSessions: Session[] = [
  { id: 'demo-1', projectId: 'cloud', workspace: { kind: 'main' }, title: '梳理 v2 接口缺口', ...defaultChoice, createdAt: 1 },
  { id: 'demo-2', projectId: 'cloud', workspace: { kind: 'isolated', branch: 'feat/766-v2-chat' }, title: '对话页样式', ...defaultChoice, createdAt: 2 },
  { id: 'demo-3', projectId: 'cloud', workspace: { kind: 'isolated', branch: 'feat/766-v2-chat' }, title: '左栏拖动排序', ...defaultChoice, createdAt: 3 },
  { id: 'demo-4', projectId: 'desktop', workspace: { kind: 'main' }, title: '对照 desktop 的活动块', ...defaultChoice, createdAt: 4 },
]

/**
 * How replay waits between steps. Tests set it to run without delays; the scale lets a demo
 * play faster.
 */
export const replayClock = {
  scale: 1,
  wait: (ms: number, run: () => void): (() => void) => {
    if (replayClock.scale === 0) {
      run()
      return () => {}
    }
    const timer = setTimeout(run, ms * replayClock.scale)
    return () => clearTimeout(timer)
  },
}

/** A running replay: what is left to play, and how to stop the step that is waiting. */
interface Player {
  steps: Step[]
  cancel?: () => void
  /** Set while the replay waits on a permission answer. */
  awaiting?: Extract<Step, { kind: 'await' }>
}

const players = new Map<string, Player>()
let ids = 0
const newId = (prefix: string) => `${prefix}-${Date.now().toString(36)}-${(ids++).toString(36)}`

export const useChatStore = create<State>()(
  persist(
    (set, get) => {
      const dispatch = (id: string, event: ChatEvent) => {
        set((s) => {
          const before = s.conversations[id] ?? emptyConversation
          const after = applyEvent(before, event, Date.now())
          const queued =
            event.type === 'prompt' && s.queued[id]?.[0] === event.text
              ? { ...s.queued, [id]: s.queued[id].slice(1) }
              : s.queued
          const finished = isResponding(before) && !isResponding(after) && s.selectedId !== id
          return {
            // A recording's model switch stays within the session's agent; an agent switch is
            // applied by setAgentModel and only marked here.
            sessions:
              event.type === 'model'
                ? s.sessions.map((x) => (x.id === id && agents.find((a) => a.id === x.agent)?.models.includes(event.model) ? { ...x, model: event.model } : x))
                : s.sessions,
            conversations: { ...s.conversations, [id]: after },
            queued,
            unread: finished ? { ...s.unread, [id]: true } : s.unread,
          }
        })
      }

      /** Plays steps until one has to wait; returns when the recording ends or pauses. */
      const advance = (id: string) => {
        const player = players.get(id)
        if (!player) return
        const step = player.steps.shift()
        if (!step) {
          players.delete(id)
          setPlaying(id, false)
          sendQueued(id)
          return
        }
        if (step.kind === 'await') {
          player.awaiting = step
          return
        }
        player.cancel = replayClock.wait(step.delay, () => {
          if (step.kind === 'event') dispatch(id, step.event)
          if (step.kind === 'queue') set((s) => ({ queued: { ...s.queued, [id]: [...(s.queued[id] ?? []), step.text] } }))
          advance(id)
        })
      }

      const setPlaying = (id: string, on: boolean) =>
        set((s) => {
          const { [id]: _, ...rest } = s.playing
          return { playing: on ? { ...rest, [id]: true } : rest }
        })

      const play = (id: string, steps: Step[]) => {
        players.get(id)?.cancel?.()
        players.set(id, { steps: [...steps] })
        setPlaying(id, true)
        advance(id)
      }

      /** Sends the next queued prompt once the agent is free, as the person would have. */
      const sendQueued = (id: string) => {
        const next = get().queued[id]?.[0]
        if (next === undefined || players.has(id) || isResponding(get().conversations[id] ?? emptyConversation)) return
        play(id, standInReply(next, { text: next, attachments: [], plugins: [] }).steps)
      }

      return {
        projects: [sampleProject, ...demoProjects],
        sessions: [...sampleSessions, ...demoSessions],
        conversations: {},
        queued: {},
        unread: {},
        drafts: {},
        playing: {},
        sidebar: defaultSidebar,
        setSidebar: (sidebar) => set((s) => ({ sidebar: { ...s.sidebar, ...sidebar } })),
        select: (id) => {
          set((s) => {
            const { [id]: _, ...unread } = s.unread
            return { selectedId: id, selectedRunId: undefined, unread }
          })
          const session = get().sessions.find((x) => x.id === id)
          // A sample plays the first time it is opened, so picking it is enough to see it work.
          if (session?.scenarioId && !get().conversations[id] && !players.has(id)) get().replay(id)
        },
        selectRun: (id) => set({ selectedRunId: id }),
        newChat: { projectId: demoProjects[0].id, workspace: { kind: 'main' }, ...defaultChoice },
        startNewChat: (scope) =>
          set((s) => {
            const current = s.sessions.find((x) => x.id === s.selectedId)
            const fromSelection = current && !s.projects.find((p) => p.id === current.projectId)?.samples ? current : undefined
            const where = scope
              ? { projectId: scope.projectId, workspace: scope.workspace ?? { kind: 'main' as const } }
              : fromSelection
                ? { projectId: fromSelection.projectId, workspace: fromSelection.workspace }
                : {}
            return { selectedId: undefined, selectedRunId: undefined, newChat: { ...s.newChat, ...where } }
          }),
        setNewChat: (patch) => set((s) => ({ newChat: { ...s.newChat, ...patch } })),
        sendNewChat: (draft) => {
          const { projectId, workspace, agent, model } = get().newChat
          const taken = get().sessions.flatMap((s) => (s.projectId === projectId && s.workspace.kind === 'isolated' ? [s.workspace.branch] : []))
          const where: Workspace = workspace.kind === 'new-worktree' ? { kind: 'isolated', branch: branchNameFor(draft.text, taken) } : workspace
          const id = get().createSession(projectId, where, { agent, model })
          set((s) => ({ drafts: { ...s.drafts, [newChatKey]: emptyDraft } }))
          get().send(id, draft)
        },
        createSession: (projectId, workspace, choice = defaultChoice) => {
          const id = newId('session')
          const session: Session = { id, projectId, workspace, title: '新对话', ...choice, createdAt: Date.now() }
          set((s) => ({ sessions: [...s.sessions, session], selectedId: id, selectedRunId: undefined }))
          return id
        },
        renameSession: (id, title) =>
          set((s) => ({ sessions: s.sessions.map((x) => (x.id === id && title.trim() ? { ...x, title: title.trim() } : x)) })),
        deleteSession: (id) => {
          players.get(id)?.cancel?.()
          players.delete(id)
          setPlaying(id, false)
          set((s) => {
            const { [id]: _, ...conversations } = s.conversations
            return {
              sessions: s.sessions.filter((x) => x.id !== id),
              conversations,
              selectedId: s.selectedId === id ? undefined : s.selectedId,
            }
          })
        },
        moveProject: (id, overId) =>
          set((s) => {
            const from = s.projects.findIndex((p) => p.id === id)
            const to = s.projects.findIndex((p) => p.id === overId)
            if (from === -1 || to === -1 || from === to) return s
            const projects = [...s.projects]
            projects.splice(to, 0, ...projects.splice(from, 1))
            return { projects }
          }),
        send: (id, draft) => {
          const conversation = get().conversations[id] ?? emptyConversation
          if (isResponding(conversation) || players.has(id)) {
            set((s) => ({ queued: { ...s.queued, [id]: [...(s.queued[id] ?? []), draft.text] } }))
            return
          }
          const session = get().sessions.find((x) => x.id === id)
          if (session && session.title === '新对话' && conversation.turns.length === 0) {
            get().renameSession(id, draft.text.slice(0, 24))
          }
          play(id, standInReply(draft.text, draft).steps)
        },
        stop: (id) => {
          const player = players.get(id)
          player?.cancel?.()
          players.delete(id)
          setPlaying(id, false)
          if (isResponding(get().conversations[id] ?? emptyConversation)) {
            dispatch(id, { type: 'turnEnd', stopReason: 'cancelled' })
          }
        },
        answer: (id, permissionId, optionId) => {
          const player = players.get(id)
          if (player?.awaiting?.id === permissionId) {
            const branch = player.awaiting.branches[optionId] ?? []
            player.awaiting = undefined
            player.steps.unshift(...branch)
            advance(id)
            return
          }
          dispatch(id, { type: 'permissionResolved', id: permissionId, optionId })
        },
        setAgentModel: (id, agent, model) => {
          const session = get().sessions.find((x) => x.id === id)
          if (!session || (session.agent === agent && session.model === model)) return
          set((s) => ({ sessions: s.sessions.map((x) => (x.id === id ? { ...x, agent, model } : x)) }))
          if ((get().conversations[id]?.turns.length ?? 0) > 0) {
            dispatch(id, { type: 'model', model: agent === session.agent ? model : `${agentName(agent)} · ${model}` })
          }
        },
        createProject: (path) => {
          const id = newId('project')
          const root = path.trim().replace(/[\\/]+$/, '')
          set((s) => ({ projects: [...s.projects, { id, name: root.split(/[\\/]/).at(-1) || root, path: root }] }))
          return id
        },
        setDraft: (id, draft) => set((s) => ({ drafts: { ...s.drafts, [id]: draft } })),
        unqueue: (id, index) =>
          set((s) => ({ queued: { ...s.queued, [id]: (s.queued[id] ?? []).filter((_, i) => i !== index) } })),
        replay: (id) => {
          const scenario = scenarios.find((s) => s.id === get().sessions.find((x) => x.id === id)?.scenarioId)
          if (!scenario) return
          set((s) => ({ conversations: { ...s.conversations, [id]: emptyConversation }, queued: { ...s.queued, [id]: [] } }))
          play(id, scenario.steps)
        },
      }
    },
    {
      name: 'ora-v2-chat',
      version: 1,
      storage: createJSONStorage(() => localStorage),
      // The project order moves to v2-server with the session API; the layout stays here.
      partialize: (s) => ({ projectOrder: s.projects.map((p) => p.id), sidebar: s.sidebar }),
      merge: (persisted, current) => {
        const saved = persisted as { projectOrder?: string[]; sidebar?: SidebarLayout } | undefined
        const order = saved?.projectOrder ?? []
        const rank = (id: string) => {
          const i = order.indexOf(id)
          return i === -1 ? order.length : i
        }
        return {
          ...current,
          projects: [...current.projects].sort((a, b) => rank(a.id) - rank(b.id)),
          sidebar: { ...current.sidebar, ...saved?.sidebar },
        }
      },
    },
  ),
)

/**
 * The reply a non-sample session gets until v2-server can run an agent (#758): it says plainly
 * that it is a stand-in and echoes what was sent, so the input, attachments and plugins can be
 * tried end to end.
 */
function standInReply(text: string, draft: Draft) {
  const turnId = newId('turn')
  const s = script()
  s.steps.push({ kind: 'event', delay: 0, event: { type: 'prompt', turnId, text, attachments: draft.attachments, plugins: draft.plugins } })
  s.think('后端还不能运行 Agent，这是前端的演示回复。')
  const extras = [
    draft.attachments.length ? `${draft.attachments.length} 个附件` : '',
    draft.plugins.length ? `插件：${draft.plugins.map((p) => p.name).join('、')}` : '',
  ].filter(Boolean)
  s.say(
    `这是演示回复：v2-server 接入 Agent 之后，这里会显示真实的回答。\n\n你发送的是：\n\n> ${text.replace(/\n/g, '\n> ')}${
      extras.length ? `\n\n（${extras.join('；')}）` : ''
    }`,
  )
  s.end()
  return s
}

/** Resets everything to the initial demo state; tests call it between cases. */
export function resetChatStore() {
  for (const p of players.values()) p.cancel?.()
  players.clear()
  useChatStore.setState({
    projects: [sampleProject, ...demoProjects],
    sessions: [...sampleSessions, ...demoSessions],
    conversations: {},
    queued: {},
    unread: {},
    drafts: {},
    playing: {},
    sidebar: defaultSidebar,
    newChat: { projectId: demoProjects[0].id, workspace: { kind: 'main' }, ...defaultChoice },
    selectedId: undefined,
    selectedRunId: undefined,
  })
}
