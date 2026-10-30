import { getApiV1TenantsTidProjects } from '@/api/projects/projects'
import { getApiV1TenantsTidProjectsPidWorkspaces } from '@/api/workspaces/workspaces'
import { statusOf } from '@/lib/api-client'
import { isAxiosError } from 'axios'
import {
  type ChatConversation,
  type ChatEvent,
  type ChatExecutor,
  createChatConversation,
  deleteChatConversation,
  endChatSession,
  getChatProjectOrder,
  listChatConversations,
  listChatExecutors,
  renameChatConversation,
  sendChatMessage,
  setChatProjectOrder,
  stopChatConversation,
} from './api/generated'

/** A project the backend can run chats in, with its worktree workspaces by branch. */
export interface RemoteProject {
  id: string
  name: string
  worktrees: { id: string; branch: string }[]
}

/** Everything the chat page loads once it knows the tenant. */
export interface RemoteState {
  executors: ChatExecutor[]
  conversations: ChatConversation[]
  projects: RemoteProject[]
  projectOrder: string[]
}

/**
 * Why a request failed, in the terms the page shows: the workspace is held (by one of the
 * person's conversations when `occupant` is set), the backend cannot run Agents yet, or something
 * else went wrong.
 */
export type Failure =
  | { kind: 'busy'; occupant?: { id: string; title: string } }
  | { kind: 'unavailable' }
  | { kind: 'error'; message: string }

/** Turns a failed request into a {@link Failure}. */
export function failureOf(error: unknown): Failure {
  const status = statusOf(error)
  const body = isAxiosError(error) ? (error.response?.data as { code?: string; params?: Record<string, unknown> } | undefined) : undefined
  if (status === 409 && body?.code === 'workspace_busy') {
    const { conversationId, title } = body.params ?? {}
    return typeof conversationId === 'string' ? { kind: 'busy', occupant: { id: conversationId, title: String(title ?? '') } } : { kind: 'busy' }
  }
  if (status === 503 && body?.code === 'execution_unavailable') return { kind: 'unavailable' }
  if (status === 503 && body?.code === 'stop_unconfirmed') return { kind: 'error', message: '还没确认停下，请稍后再试。' }
  if (status === 400 && body?.code === 'unsupported_content') return { kind: 'error', message: '首版只能发送文字消息。' }
  return { kind: 'error', message: '请求失败，请重试。' }
}

/** Whether a request's outcome is unknown and must be retried with the same idempotency key. */
const uncertain = (error: unknown) => {
  const body = isAxiosError(error) ? (error.response?.data as { code?: string } | undefined) : undefined
  return statusOf(error) === 503 && body?.code === 'admission_unknown'
}

/**
 * Runs an admission with one idempotency key, retrying while the backend says its outcome is
 * unknown: the same key settles it without running the prompt twice.
 */
async function admit<T>(run: (headers: Record<string, string>) => Promise<T>): Promise<T> {
  const headers = { 'Idempotency-Key': crypto.randomUUID() }
  for (let attempt = 0; ; attempt++) {
    try {
      return await run(headers)
    } catch (error) {
      if (!uncertain(error) || attempt >= 3) throw error
      await new Promise((r) => setTimeout(r, 500 * (attempt + 1)))
    }
  }
}

/**
 * The chat backend the store talks to. Conversations and their live event streams are client
 * state driven by Server-Sent Events, so the store owns them instead of TanStack Query; every
 * call still goes through the generated client.
 */
export const remote = {
  /** Loads what the page needs; projects come from the Cloud contract, the rest from v2-server. */
  async load(tid: string): Promise<RemoteState> {
    const [executors, conversations, projects, order] = await Promise.all([
      listChatExecutors(tid),
      listChatConversations(tid),
      getApiV1TenantsTidProjects(tid),
      getChatProjectOrder(tid),
    ])
    const withWorktrees = await Promise.all(
      projects.items.map(async (p): Promise<RemoteProject> => {
        const workspaces = await getApiV1TenantsTidProjectsPidWorkspaces(tid, p.id)
        const worktrees = workspaces.items.flatMap((w) => (w.kind === 'isolated' && w.branchName ? [{ id: w.id, branch: w.branchName }] : []))
        return { id: p.id, name: p.name, worktrees }
      }),
    )
    return { executors: executors.items, conversations: conversations.items, projects: withWorktrees, projectOrder: order.projectIds }
  },
  create: (tid: string, input: { projectId: string; workspaceId: string; executorId: string; prompt: string }) =>
    admit((headers) => createChatConversation(tid, input, { headers })),
  send: (tid: string, cid: string, prompt: string) => admit((headers) => sendChatMessage(tid, cid, { prompt }, { headers })),
  conversations: async (tid: string) => (await listChatConversations(tid)).items,
  stop: (tid: string, cid: string) => stopChatConversation(tid, cid),
  release: (tid: string, cid: string) => endChatSession(tid, cid),
  rename: (tid: string, cid: string, title: string) => renameChatConversation(tid, cid, { title }),
  remove: (tid: string, cid: string) => deleteChatConversation(tid, cid),
  saveOrder: (tid: string, projectIds: string[]) => setChatProjectOrder(tid, { projectIds }),

  /**
   * Follows one session's events. The browser's EventSource resumes after the last event id on
   * its own; `end` closes the stream when the session has ended and `failure` when the backend
   * cannot continue it. Returns the function that stops following.
   */
  follow(tid: string, cid: string, sid: string, after: number, on: { event: (e: ChatEvent) => void; end: () => void; failure: () => void }): () => void {
    const source = remote.eventSource(`/api/v2/tenants/${tid}/chat/conversations/${cid}/sessions/${sid}/events${after ? `?after=${after}` : ''}`)
    source.onmessage = (m: MessageEvent<string>) => on.event(JSON.parse(m.data) as ChatEvent)
    source.addEventListener('end', () => (source.close(), on.end()))
    source.addEventListener('failure', () => (source.close(), on.failure()))
    return () => source.close()
  },
  /** Opens an event stream; the demo backend replaces it with its own. */
  eventSource: (url: string): EventSource => new EventSource(url),
}
