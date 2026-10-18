import { useId } from 'react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useRuns } from '@/features/issues/api'
import type { IssueRun, TenantMember } from '@/features/issues/types'
import { memberNameById } from '@/features/issues/present'
import { RunDelivery } from './run-delivery'
import { RunResume } from './run-resume'
import { RunPreparation, SessionFailure } from './run-preparation'
import { useLoadOlderThread, useThread, type ThreadRef, type ThreadSnapshot } from './thread-api'
import { ThreadComposer } from './thread-composer'
import { latestAgentRun, threadStateLabel } from './thread-entries'
import { ThreadMessages } from './thread-messages'

function DeclaredThread({
  threadRef,
  snapshot,
  names,
  preparation,
  readOnly,
}: {
  threadRef: ThreadRef
  snapshot: Extract<ThreadSnapshot, { declared: true }>
  names: ReadonlyMap<string, string>
  preparation: IssueRun['preparation']
  readOnly: boolean
}) {
  const loadOlder = useLoadOlderThread(threadRef)
  const oldest = snapshot.entries[0]?.seq
  return (
    <>
      {snapshot.failureCode && <SessionFailure code={snapshot.failureCode} />}
      {snapshot.threadState === 'pending' && preparation && (
        <RunPreparation progress={preparation} />
      )}
      <ThreadMessages
        entries={snapshot.entries}
        hasOlder={oldest !== undefined && oldest > 1}
        loadingOlder={loadOlder.isPending}
        onLoadOlder={() => {
          if (oldest !== undefined) loadOlder.mutate(oldest)
        }}
      />
      {loadOlder.isError && <p className="text-xs text-destructive">加载更早的消息失败</p>}
      <p className="text-xs text-muted-foreground">
        发起者：
        {snapshot.initiatorUserId ? (names.get(snapshot.initiatorUserId) ?? '用户') : '历史会话'}
      </p>
      {snapshot.model && (
        <p className="break-all text-xs text-muted-foreground">
          {snapshot.model.connectionName} · {snapshot.model.modelName}（{snapshot.model.modelId}）
        </p>
      )}
      {!readOnly && (
        <ThreadComposer
          threadRef={threadRef}
          threadState={snapshot.threadState}
          canAppend={snapshot.canAppend}
          canEnd={snapshot.canEnd}
        />
      )}
    </>
  )
}

function UndeclaredRun({ run }: { run: IssueRun }) {
  const settled = ['completed', 'failed', 'cancelled'].includes(run.status)
  if (!settled || run.preparation) return <RunPreparation progress={run.preparation ?? null} />
  return (
    <p role="status" className="text-sm text-muted-foreground">
      运行已结束，未产生会话记录。执行状态：{run.status === 'failed' ? '失败' : '已结束'}
    </p>
  )
}

function RunThread({
  threadRef,
  run,
  names,
  runs,
  readOnly,
}: {
  threadRef: ThreadRef
  run: IssueRun
  names: ReadonlyMap<string, string>
  runs: readonly IssueRun[]
  readOnly: boolean
}) {
  const settled = ['completed', 'failed', 'cancelled'].includes(run.status)
  const thread = useThread(threadRef, settled)
  const headingId = useId()
  const snapshot = thread.data

  return (
    <section aria-labelledby={headingId} className="space-y-3 rounded-lg border p-3">
      <div className="flex items-center justify-between gap-2">
        <h2 id={headingId} className="text-sm font-semibold">
          Agent 会话
        </h2>
        {snapshot?.declared && (
          <Badge variant="secondary" aria-live="polite">
            {snapshot.failureCode ? '运行失败' : threadStateLabel(snapshot.threadState)}
          </Badge>
        )}
      </div>
      <RunResume run={run} runs={runs} />
      {thread.isPending && <Skeleton className="h-16 w-full" />}
      {thread.isError && <p className="text-sm text-destructive">会话加载失败</p>}
      {snapshot?.declared === false && <UndeclaredRun run={run} />}
      {snapshot?.declared && (
        <DeclaredThread
          threadRef={threadRef}
          snapshot={snapshot}
          names={names}
          preparation={run.preparation}
          readOnly={readOnly}
        />
      )}
      <RunDelivery
        run={run}
        threadEnded={snapshot?.declared === true && snapshot.threadState === 'ended'}
      />
    </section>
  )
}

/**
 * The issue page's "Agent 会话" column: the Thread of the issue's most recent
 * agent run, with paging, live updates and the composer. It renders nothing
 * when the issue has no agent run, so issues worked by people, teams or
 * workflows keep their layout. The Thread is keyed by run, so a newer agent
 * run starts from its own tail instead of inheriting the previous one's rows.
 */
export function IssueThreadPanel({
  tid,
  issueId,
  members,
  selectedRunId,
}: {
  tid: string
  issueId: string
  members?: TenantMember[]
  /** A historical run chosen by the task page; never silently falls back to another run. */
  selectedRunId?: string | undefined
}) {
  const { data: runs = [], isPending, isError } = useRuns(tid, issueId)
  const latest = latestAgentRun(runs)
  const run = selectedRunId
    ? runs.find((candidate) => candidate.id === selectedRunId && candidate.executorType === 'agent')
    : latest
  if (isError) return <p role="alert">执行记录加载失败，请稍后重试。</p>
  if (selectedRunId && !isPending && !run) return <p role="alert">该执行记录不存在或已不可访问。</p>
  if (!run) return null
  return (
    <div className="min-w-0 w-full">
      {selectedRunId && run.id !== latest?.id && (
        <p className="mb-3 text-xs text-muted-foreground">
          正在查看历史执行；继续对话请返回最新执行。
        </p>
      )}
      <RunThread
        key={run.id}
        run={run}
        runs={runs}
        threadRef={{ tid, issueId, runId: run.id }}
        names={memberNameById(members)}
        readOnly={!!selectedRunId && run.id !== latest?.id}
      />
    </div>
  )
}
