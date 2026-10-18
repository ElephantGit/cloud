import { useState } from 'react'
import { Link } from 'react-router-dom'
import type { TaskConversationMessage } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { ConversationMarkdown } from '@/features/conversation-content/markdown'
import { useTimeline } from '@/features/issues/api'
import { IssueThreadPanel } from '@/features/issues/thread/thread-panel'
import type {
  CollaborationTargetSummary,
  IssueRun,
  TenantMember,
  TimelineEntry,
} from '@/features/issues/types'
import { workspacePaths } from '@/lib/paths'
import { faultCode } from '@/lib/api-client'
import {
  loadedTaskMessages,
  taskConversationError,
  taskConversationFailureCode,
  useDiscardUndeliveredTaskRun,
  useResumeTaskConversation,
  useTaskConversation,
  useUpdateTaskMessage,
} from './api'
import { ConversationComposer } from './conversation-composer'
import { useConversationDraft, type ConversationQuote } from './use-conversation-draft'

type ConversationRow =
  | { kind: 'run'; at: string; id: string; run: IssueRun }
  | { kind: 'message'; at: string; id: string; message: TaskConversationMessage }
  | { kind: 'comment'; at: string; id: string; comment: TimelineEntry }

const RUN_LABELS: Record<string, string> = {
  queued: '排队中',
  dispatched: '准备中',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

function RunContext({ input }: { input: IssueRun['input'] }) {
  const raw = input['conversationHistory']
  const history: Record<string, unknown>[] = Array.isArray(raw)
    ? raw.filter(
        (item): item is Record<string, unknown> =>
          typeof item === 'object' && item !== null && typeof item['id'] === 'string',
      )
    : []
  const incomplete = input['conversationHistoryIncomplete'] === true
  if (!history.length && !incomplete) return null
  return (
    <details className="rounded-lg border border-dashed p-3 text-xs">
      <summary className="cursor-pointer text-muted-foreground">
        本次带入历史：{history.length} 轮{incomplete ? ' · 部分旧记录未带入' : ''}
      </summary>
      <ol className="mt-3 space-y-3">
        {history.map((item) => (
          <li key={String(item['id'])} className="rounded-md bg-muted/40 p-2">
            {typeof item['user'] === 'string' && <p>用户：{item['user']}</p>}
            {typeof item['assistant'] === 'string' && <p>Agent：{item['assistant']}</p>}
            {item['assistantTruncated'] === true && (
              <p className="text-muted-foreground">此轮回复已截断</p>
            )}
          </li>
        ))}
      </ol>
    </details>
  )
}

function conversationRows(
  runs: IssueRun[],
  messages: TaskConversationMessage[],
  timeline: TimelineEntry[],
): ConversationRow[] {
  const triggered = new Set(runs.map((run) => run.triggerEvidenceRefId).filter(Boolean))
  const indexed = new Set(runs.map((run) => run.id))
  const rows: ConversationRow[] = [
    ...runs.map((run): ConversationRow => ({ kind: 'run', at: run.createdAt, id: run.id, run })),
    ...messages
      .filter((message) => !message.runId || !indexed.has(message.runId))
      .map((message): ConversationRow => ({
        kind: 'message',
        at: message.createdAt,
        id: message.id,
        message,
      })),
    ...timeline
      .filter((entry) => entry.kind === 'comment' && entry.body && !triggered.has(entry.id))
      .map((comment): ConversationRow => ({
        kind: 'comment',
        at: comment.createdAt,
        id: comment.id,
        comment,
      })),
  ]
  return rows.toSorted((a, b) => a.at.localeCompare(b.at) || a.id.localeCompare(b.id))
}

function WaitingMessage({
  message,
  name,
  own,
  tid,
  issueId,
  onQuote,
}: {
  message: TaskConversationMessage
  name: string
  own: boolean
  tid: string
  issueId: string
  onQuote: (quote: ConversationQuote) => void
}) {
  const action = useUpdateTaskMessage(tid, issueId)
  const label = {
    waiting: '等待执行或交接',
    submitted: '已提交执行',
    cancelled: '已取消',
    failed: '未能启动',
  }[message.state]
  return (
    <li className="rounded-xl border bg-card p-4">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="font-medium">你 → {name}</span>
        <span className="text-xs text-muted-foreground">{label}</span>
      </div>
      <div className="mt-2">
        <ConversationMarkdown text={message.body} />
      </div>
      {message.errorCode && (
        <p role="alert" className="mt-2 text-xs text-destructive">
          {taskConversationFailureCode(message.errorCode)}
        </p>
      )}
      <div className="mt-2 flex gap-2">
        {own && message.state === 'waiting' && (
          <Button
            variant="ghost"
            size="xs"
            disabled={action.isPending}
            onClick={() => action.mutate({ id: message.id, action: 'cancel' })}
          >
            取消等待
          </Button>
        )}
        {own && message.state === 'failed' && (
          <Button
            variant="ghost"
            size="xs"
            disabled={action.isPending}
            onClick={() => action.mutate({ id: message.id, action: 'retry' })}
          >
            重试
          </Button>
        )}
        <Button
          variant="ghost"
          size="xs"
          onClick={() => onQuote({ text: message.body, agentId: message.agentId })}
        >
          引用回复
        </Button>
      </div>
      {action.isError && (
        <p role="alert" className="text-xs text-destructive">
          {taskConversationError(action.error)}
        </p>
      )}
    </li>
  )
}

function RunConversation({
  run,
  name,
  tid,
  issueId,
  members,
  workspaceSlug,
  onQuote,
}: {
  run: IssueRun
  name: string
  tid: string
  issueId: string
  members: TenantMember[]
  workspaceSlug: string | undefined
  onQuote: (quote: ConversationQuote) => void
}) {
  const task = run.input['task']
  return (
    <li className="min-w-0 space-y-3 rounded-xl border bg-card p-4">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span className="font-medium">
          {name} · {RUN_LABELS[run.status] ?? '状态未知'}
        </span>
        {workspaceSlug && (
          <Link
            className="text-xs text-muted-foreground underline"
            to={workspacePaths(workspaceSlug).agentSession(issueId, run.id)}
          >
            完整执行记录
          </Link>
        )}
      </div>
      {typeof task === 'string' && (
        <div className="ml-6 rounded-xl bg-muted/60 p-3 sm:ml-10">
          <p className="mb-1 text-xs text-muted-foreground">你 → {name}</p>
          <ConversationMarkdown text={task} />
          <Button
            variant="ghost"
            size="xs"
            onClick={() => onQuote({ text: task, agentId: run.executorId })}
          >
            引用回复
          </Button>
        </div>
      )}
      <RunContext input={run.input} />
      <IssueThreadPanel
        tid={tid}
        issueId={issueId}
        members={members}
        selectedRunId={run.id}
        readOnly
        onQuote={(text, agentId) => onQuote({ text, agentId })}
      />
    </li>
  )
}

function ConversationRowItem({
  row,
  targets,
  tid,
  issueId,
  members,
  workspaceSlug,
  currentUserId,
  onQuote,
}: {
  row: ConversationRow
  targets: CollaborationTargetSummary[]
  tid: string
  issueId: string
  members: TenantMember[]
  workspaceSlug: string | undefined
  currentUserId: string | undefined
  onQuote: (quote: ConversationQuote) => void
}) {
  const name = (id: string) =>
    targets.find((target) => target.type === 'agent' && target.id === id)?.displayName ?? 'Agent'
  if (row.kind === 'run')
    return (
      <RunConversation
        run={row.run}
        name={name(row.run.executorId)}
        tid={tid}
        issueId={issueId}
        members={members}
        workspaceSlug={workspaceSlug}
        onQuote={onQuote}
      />
    )
  if (row.kind === 'message')
    return (
      <WaitingMessage
        message={row.message}
        name={name(row.message.agentId)}
        own={row.message.userId === currentUserId}
        tid={tid}
        issueId={issueId}
        onQuote={onQuote}
      />
    )
  return (
    <li className="rounded-xl border bg-card p-4">
      <p className="mb-2 text-xs text-muted-foreground">
        {row.comment.authorType === 'agent' ? 'Agent 回复' : '任务评论'}
      </p>
      <ConversationMarkdown text={row.comment.body ?? ''} />
    </li>
  )
}

function RecoveryButton({
  tid,
  issueId,
  runId,
  kind,
}: {
  tid: string
  issueId: string
  runId: string
  kind: 'resume' | 'discard'
}) {
  const resume = useResumeTaskConversation(tid, issueId)
  const discard = useDiscardUndeliveredTaskRun(tid, issueId)
  const action = kind === 'resume' ? resume : discard
  return (
    <>
      <Button
        className="mt-3"
        size="sm"
        variant="outline"
        disabled={action.isPending}
        onClick={() => action.mutate({ runId })}
      >
        {kind === 'resume' ? '恢复等待中的消息' : '放弃未交付成果并继续'}
      </Button>
      {action.isError && (
        <p role="alert" className="mt-2 text-destructive">
          {taskConversationError(action.error)}
        </p>
      )}
    </>
  )
}

function TaskResume({
  tid,
  issueId,
  last,
  waiting,
}: {
  tid: string
  issueId: string
  last: IssueRun | undefined
  waiting: boolean
}) {
  if (!waiting || !last) return null
  const failedRun = ['failed', 'cancelled'].includes(last.status)
  const saved = last.result?.deliveryState === 'saved' || last.result?.deliveryState === 'unchanged'
  const undelivered = ['failed', 'skipped'].includes(last.result?.deliveryState ?? '')
  let kind: 'resume' | 'discard' | null = null
  if (saved && failedRun) kind = 'resume'
  else if (undelivered) kind = 'discard'
  if (!kind) return null
  return (
    <section className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm">
      <p className="font-medium">
        {failedRun ? '上次 Agent 执行未正常完成' : '上次 Agent 执行成果交付失败'}，后续消息已暂停。
      </p>
      <p className="mt-1 text-muted-foreground">
        {kind === 'resume'
          ? '成果已保存并确认释放后，可明确恢复等待中的消息。'
          : '本次成果未交付。继续会放弃这次运行中未保存的文件修改；历史记录保留，下一轮从任务最近一次已保存成果开始。请先检查执行详情。'}
      </p>
      <RecoveryButton tid={tid} issueId={issueId} runId={last.id} kind={kind} />
    </section>
  )
}

function ConversationRows({
  rows,
  tid,
  issueId,
  members,
  targets,
  workspaceSlug,
  currentUserId,
  onQuote,
}: {
  rows: ConversationRow[]
  tid: string
  issueId: string
  members: TenantMember[]
  targets: CollaborationTargetSummary[]
  workspaceSlug: string | undefined
  currentUserId: string | undefined
  onQuote: (quote: ConversationQuote) => void
}) {
  return (
    <ol aria-label="任务对话" className="space-y-5">
      {rows.map((row) => (
        <ConversationRowItem
          key={`${row.kind}:${row.id}`}
          row={row}
          targets={targets}
          tid={tid}
          issueId={issueId}
          members={members}
          workspaceSlug={workspaceSlug}
          currentUserId={currentUserId}
          onQuote={onQuote}
        />
      ))}
    </ol>
  )
}

function OlderHistoryControls({
  olderRuns,
  loadRuns,
  conversation,
}: {
  olderRuns: boolean
  loadRuns: () => void
  conversation: ReturnType<typeof useTaskConversation>
}) {
  return (
    <>
      {olderRuns && (
        <Button variant="ghost" className="w-full" onClick={loadRuns}>
          加载更早的执行记录
        </Button>
      )}
      {conversation.hasNextPage && (
        <Button
          variant="ghost"
          className="w-full"
          disabled={conversation.isFetchingNextPage}
          onClick={() => void conversation.fetchNextPage()}
        >
          加载更早的任务消息
        </Button>
      )}
    </>
  )
}

/** Renders one durable task discussion across Run boundaries while leaving each Thread immutable. */
export function TaskConversationPanel({
  tid,
  issueId,
  members,
  targets,
  runs,
  selectedRunId,
  workspaceSlug,
  currentUserId,
  onLatest,
}: {
  tid: string
  issueId: string
  members: TenantMember[]
  targets: CollaborationTargetSummary[]
  runs: IssueRun[]
  selectedRunId?: string | undefined
  workspaceSlug?: string | undefined
  currentUserId?: string | undefined
  onLatest: () => void
}) {
  const conversation = useTaskConversation(tid, issueId)
  const timeline = useTimeline(tid, issueId)
  const [visibleRuns, setVisibleRuns] = useState(5)
  const draft = useConversationDraft()
  const agents = runs
    .filter((run) => run.executorType === 'agent')
    .toSorted((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id))
  const shown = agents.slice(-visibleRuns)
  const messages = loadedTaskMessages(conversation.data?.pages)
  const rows = conversationRows(shown, messages, timeline.data ?? [])
  const last = agents.at(-1)
  const waiting = messages.some((message) => message.state === 'waiting')
  // During a rolling upgrade an older Cloud may not have the task endpoint yet. Preserve its
  // original Thread composer until the authoritative task queue becomes available.
  if (conversation.isError && faultCode(conversation.error) === 'not_found') {
    return (
      <IssueThreadPanel
        tid={tid}
        issueId={issueId}
        members={members}
        selectedRunId={selectedRunId}
      />
    )
  }
  return (
    <div className="space-y-5">
      {selectedRunId ? (
        <IssueThreadPanel
          tid={tid}
          issueId={issueId}
          members={members}
          selectedRunId={selectedRunId}
          readOnly
          onQuote={(text, agentId) => draft.quote({ text, agentId })}
        />
      ) : (
        <>
          <OlderHistoryControls
            olderRuns={agents.length > shown.length}
            loadRuns={() => setVisibleRuns((count) => count + 5)}
            conversation={conversation}
          />
          <ConversationRows
            rows={rows}
            tid={tid}
            issueId={issueId}
            members={members}
            targets={targets}
            workspaceSlug={workspaceSlug}
            currentUserId={currentUserId}
            onQuote={draft.quote}
          />
          {!rows.length && !conversation.isPending && (
            <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
              还没有对话。选择 Agent 并发送第一条任务消息。
            </p>
          )}
          {conversation.isError && (
            <p role="alert" className="text-sm text-destructive">
              任务消息加载失败，请重试。
            </p>
          )}
          <TaskResume tid={tid} issueId={issueId} last={last} waiting={waiting} />
        </>
      )}
      <ConversationComposer
        tid={tid}
        issueId={issueId}
        targets={targets}
        latestRun={last}
        state={draft}
        onSent={onLatest}
      />
    </div>
  )
}
