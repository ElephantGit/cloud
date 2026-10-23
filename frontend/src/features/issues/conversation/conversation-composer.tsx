import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { CollaborationTargetSummary, IssueRun } from '@/features/issues/types'
import { taskConversationError } from './api'
import { useComposerSubmit } from './use-composer-submit'
import type { useConversationDraft } from './use-conversation-draft'
import type { TaskAgentReadiness } from '@/api/generated.schemas'
import { AgentReadiness } from './agent-readiness'

function AgentRecipient({
  agents,
  agentId,
  busy,
  mode,
  select,
  toggle,
}: {
  agents: CollaborationTargetSummary[]
  agentId: string
  busy: boolean
  mode: 'agent' | 'comment'
  select: (id: string) => void
  toggle: () => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <label htmlFor="task-conversation-agent" className="font-medium">
        发送给
      </label>
      <select
        id="task-conversation-agent"
        value={agents.some((agent) => agent.id === agentId) ? agentId : ''}
        disabled={mode === 'comment' || busy}
        onChange={(event) => select(event.target.value)}
        className="h-9 min-w-36 rounded-md border bg-background px-2"
      >
        <option value="">选择 Agent</option>
        {agents.map((target) => (
          <option key={target.id} value={target.id}>
            {target.displayName}
          </option>
        ))}
      </select>
      <Button
        type="button"
        size="sm"
        variant={mode === 'comment' ? 'secondary' : 'ghost'}
        aria-pressed={mode === 'comment'}
        onClick={toggle}
      >
        仅评论
      </Button>
    </div>
  )
}

function ComposerFeedback({
  handoff,
  agentName,
  agentMissing,
  sendError,
  commentError,
}: {
  handoff: boolean
  agentName: string
  agentMissing: boolean
  sendError: unknown
  commentError: unknown
}) {
  return (
    <>
      {handoff && (
        <p className="text-xs text-muted-foreground">
          将等待当前 Agent 完成、保存成果并释放工作区后交接给 {agentName}。
        </p>
      )}
      {agentMissing && (
        <p className="text-xs text-muted-foreground">
          先选择可用 Agent；切换目标本身不会启动执行。
        </p>
      )}
      {sendError && (
        <p role="alert" className="text-xs text-destructive">
          {taskConversationError(sendError)}
        </p>
      )}
      {commentError && (
        <p role="alert" className="text-xs text-destructive">
          评论发送失败，请重试。
        </p>
      )}
    </>
  )
}

function waitingForHandoff(
  mode: 'agent' | 'comment',
  agent: CollaborationTargetSummary | undefined,
  latest: IssueRun | undefined,
): boolean {
  return (
    mode === 'agent' &&
    !!agent &&
    !!latest &&
    latest.executorId !== agent.id &&
    ['queued', 'dispatched', 'running'].includes(latest.status)
  )
}

function mentionAtEnd(draft: string): { start: number; query: string } | null {
  const match = /(^|\s)@([\p{L}\p{N}_-]*)$/u.exec(draft)
  if (!match) return null
  return { start: match.index + (match[1]?.length ?? 0), query: match[2] ?? '' }
}

function MentionPicker({
  agents,
  draft,
  choose,
}: {
  agents: CollaborationTargetSummary[]
  draft: string
  choose: (agentId: string, start: number) => void
}) {
  const mention = mentionAtEnd(draft)
  if (!mention) return null
  const matches = agents.filter((target) =>
    target.displayName.toLocaleLowerCase().includes(mention.query.toLocaleLowerCase()),
  )
  if (!matches.length) return null
  return (
    <div aria-label="选择提及的 Agent" className="flex flex-wrap gap-1 rounded-md border p-2">
      {matches.map((target) => (
        <Button
          key={target.id}
          type="button"
          size="xs"
          variant="ghost"
          onClick={() => choose(target.id, mention.start)}
        >
          {target.displayName}
        </Button>
      ))}
    </div>
  )
}

/** One task composer for Agent turns and notes, with an explicit recipient and handoff state. */
export function ConversationComposer({
  tid,
  issueId,
  targets,
  latestRun,
  state,
  onSent,
  readiness,
  settingsPath,
}: {
  tid: string
  issueId: string
  targets: CollaborationTargetSummary[]
  latestRun: IssueRun | undefined
  state: ReturnType<typeof useConversationDraft>
  onSent: () => void
  readiness: TaskAgentReadiness[] | undefined
  settingsPath: string | undefined
}) {
  const { mode, setMode, selected, setSelected, draft, setDraft, input } = state
  const agents = targets.filter((target) => target.type === 'agent')
  const agentId = selected ?? latestRun?.executorId ?? ''
  const agent = agents.find((target) => target.id === agentId)
  const handoff = waitingForHandoff(mode, agent, latestRun)
  const recipient = readiness?.find((item) => item.agentId === agentId)
  const submission = useComposerSubmit({
    tid,
    issueId,
    draft,
    mode,
    agent,
    readiness: recipient,
    clearDraft: () => setDraft(''),
    onSent,
  })
  const { busy, canSubmit, submit } = submission

  let buttonLabel = '发送'
  if (busy) buttonLabel = '发送中…'
  else if (mode === 'comment') buttonLabel = '发表评论'

  return (
    <form onSubmit={submit} className="space-y-2">
      <AgentRecipient
        agents={agents}
        agentId={agentId}
        busy={busy}
        mode={mode}
        select={setSelected}
        toggle={() => setMode((current) => (current === 'agent' ? 'comment' : 'agent'))}
      />
      <Textarea
        ref={input}
        aria-label="任务对话输入"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        placeholder={mode === 'comment' ? '写下任务评论…' : '给 Agent 发送消息…'}
        rows={3}
        className="max-h-40 resize-none [field-sizing:content]"
        disabled={busy}
      />
      {mode === 'agent' && (
        <MentionPicker
          agents={agents}
          draft={draft}
          choose={(targetId, start) => {
            setSelected(targetId)
            setDraft(draft.slice(0, start).trimEnd())
            input.current?.focus()
          }}
        />
      )}
      <ComposerFeedback
        handoff={handoff}
        agentName={agent?.displayName ?? 'Agent'}
        agentMissing={mode === 'agent' && !agent}
        sendError={submission.sendError}
        commentError={submission.commentError}
      />
      {mode === 'agent' && <AgentReadiness readiness={recipient} settingsPath={settingsPath} />}
      <div className="flex justify-end">
        <Button type="submit" disabled={!canSubmit}>
          {buttonLabel}
        </Button>
      </div>
    </form>
  )
}
