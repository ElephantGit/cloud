import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCollaborationTargets, useRuns } from '@/features/issues/api'
import { useSession } from '@/features/auth/session'
import { TaskConversationPanel } from '@/features/issues/conversation/conversation-panel'
import type { CollaborationTargetSummary, IssueRun, TenantMember } from '@/features/issues/types'
import { ActivityPanel } from './activity-panel'
import { workspacePaths } from '@/lib/paths'

const STATUS: Record<string, string> = {
  queued: '排队中',
  dispatched: '准备中',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

function RunCard({ run, name, onOpen }: { run: IssueRun; name: string; onOpen: () => void }) {
  const task = run.input['task']
  return (
    <li className="rounded-xl border bg-card p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">{name}</span>
        <span className="text-xs text-muted-foreground">{STATUS[run.status] ?? '状态未知'}</span>
      </div>
      {typeof task === 'string' && (
        <p className="mt-2 line-clamp-2 break-words text-sm text-muted-foreground">{task}</p>
      )}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <time dateTime={run.createdAt}>
          {run.createdAt.replace('T', ' ').replace(/\.\d+Z$|Z$/, ' UTC')}
        </time>
        <Button variant="outline" size="sm" onClick={onOpen}>
          查看执行详情
        </Button>
      </div>
      {run.revision && (
        <p className="mt-2 text-xs text-muted-foreground">
          已保存成果 · {run.revision.finalCommit.slice(0, 7)}
        </p>
      )}
      {run.result?.deliveryState === 'failed' && (
        <p className="mt-2 text-xs text-destructive">成果保存失败</p>
      )}
    </li>
  )
}

function RunHeading({
  run,
  targets,
  workspaceSlug,
  issueId,
}: {
  run: IssueRun | undefined
  targets: CollaborationTargetSummary[]
  workspaceSlug: string | undefined
  issueId: string
}) {
  if (!run) return null
  const agent = targets.find((target) => target.type === 'agent' && target.id === run.executorId)
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
      <span className="font-medium">{agent?.displayName ?? 'Agent'}</span>
      {workspaceSlug && (
        <Link
          className="text-xs text-muted-foreground underline underline-offset-4"
          to={workspacePaths(workspaceSlug).agentSession(issueId, run.id)}
        >
          完整执行记录与归档
        </Link>
      )}
    </div>
  )
}

/** Task-level navigation preserves selected historical execution in the URL without starting work. */
export function IssueWorkspace({
  tid,
  issueId,
  members,
}: {
  tid: string
  issueId: string
  members: TenantMember[]
}) {
  const [params, setParams] = useSearchParams()
  const { session } = useSession()
  const { workspaceSlug } = useParams<{ workspaceSlug: string }>()
  const { data: runs = [], isError } = useRuns(tid, issueId)
  const { data: targets = [] } = useCollaborationTargets(tid)
  const selected = params.get('run') ?? undefined
  const requestedTab = params.get('view')
  const tab = requestedTab === 'runs' || requestedTab === 'activity' ? requestedTab : 'conversation'
  const agents = runs
    .filter((run) => run.executorType === 'agent')
    .toSorted((a, b) => b.createdAt.localeCompare(a.createdAt) || b.id.localeCompare(a.id))
  const viewedRun = selected ? agents.find((run) => run.id === selected) : agents[0]
  function navigate(view: string, runId?: string) {
    setParams((previous) => {
      const next = new URLSearchParams(previous)
      next.set('view', view)
      if (runId) next.set('run', runId)
      else next.delete('run')
      return next
    })
  }
  return (
    <Tabs value={tab} onValueChange={(value) => navigate(String(value))} className="min-w-0">
      <TabsList variant="line">
        <TabsTrigger value="conversation">对话</TabsTrigger>
        <TabsTrigger value="runs">执行记录{agents.length ? ` · ${agents.length}` : ''}</TabsTrigger>
        <TabsTrigger value="activity">任务动态</TabsTrigger>
      </TabsList>
      <TabsContent value="conversation" className="min-w-0 space-y-4 pt-4">
        <RunHeading
          run={viewedRun}
          targets={targets}
          workspaceSlug={workspaceSlug}
          issueId={issueId}
        />
        {selected && (
          <Button variant="ghost" size="sm" onClick={() => navigate('conversation')}>
            返回最新执行
          </Button>
        )}
        <TaskConversationPanel
          tid={tid}
          issueId={issueId}
          members={members}
          targets={targets}
          runs={runs}
          selectedRunId={selected}
          workspaceSlug={workspaceSlug}
          currentUserId={session.status === 'signed-in' ? session.user.id : undefined}
          onLatest={() => navigate('conversation')}
        />
      </TabsContent>
      <TabsContent value="runs" className="pt-4">
        {isError && <p role="alert">执行记录加载失败，请稍后重试。</p>}
        <ol aria-label="执行记录" className="space-y-3">
          {agents.map((run) => (
            <RunCard
              key={run.id}
              run={run}
              name={
                targets.find((target) => target.type === 'agent' && target.id === run.executorId)
                  ?.displayName ?? 'Agent'
              }
              onOpen={() => navigate('conversation', run.id)}
            />
          ))}
        </ol>
        {!agents.length && !isError && (
          <p className="py-6 text-sm text-muted-foreground">暂无 Agent 执行记录</p>
        )}
      </TabsContent>
      <TabsContent value="activity">
        <ActivityPanel
          slug={tid}
          issueId={issueId}
          members={members}
          onOpenRun={(runId) => navigate('conversation', runId)}
          onTaskStarted={() => navigate('conversation')}
        />
      </TabsContent>
    </Tabs>
  )
}
