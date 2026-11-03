import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router'
import { ChevronLeft, ClipboardList, Pencil } from 'lucide-react'
import { useUser } from '@/app/auth/user'
import { Button } from '@/components/ui/button'
import { getApiV2TenantsTidProjects } from '@/lib/projects-api/generated'
import { listSquads } from './api/generated'
import { PanelDialog } from './panel-dialog'
import { RunPane } from './run-pane'
import { TaskComposer, type CommentDraft } from './task-composer'
import { TaskDetailView } from './task-detail'
import { TaskDiscussion } from './task-discussion'
import { TaskForm, type TaskDraft } from './task-form'
import { projectLabel, taskMessageFor, taskRunMessageFor, taskStatusLabel } from './task-model'
import {
  createTaskComment, getTask, getTaskTimeline, listTaskExecutors, setTaskAssignee, setTaskStatus, startTaskRun, stopTaskRun, updateTask,
  type Task, type TaskAssigneeRef, type TaskStatusProperty,
} from './tasks-api/generated'
import { useWorkspace } from './workspace'

/** Fresh idempotency key for one submission: a retry of the same click reuses it. */
const keyed = () => ({ headers: { 'Idempotency-Key': crypto.randomUUID() } })

/**
 * One task as the team works on it: the Agent's runs, live, beside the team's discussion, one
 * input for both, and the task's owner, execution and status on the side. The page reads the task
 * and its timeline again every few seconds while a run is live, since nothing pushes changes.
 */
export default function TaskPage() {
  const { taskId = '' } = useParams()
  const me = useUser()
  const { tenantId } = useWorkspace()
  const client = useQueryClient()
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') === 'discussion' ? 'discussion' : 'runs'
  const [pickedRun, setPickedRun] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [composeError, setComposeError] = useState('')
  const [editing, setEditing] = useState(false)
  const [pickedExecutor, setExecutorId] = useState('')
  const live = (task?: Task) => !!task?.activeRun
  const taskQuery = useQuery({
    queryKey: ['task', tenantId, taskId], queryFn: () => getTask(tenantId, taskId), enabled: !!tenantId && !!taskId,
    refetchInterval: q => (q.state.data?.activeRun?.state === 'running' ? 2000 : 10000),
  })
  const task = taskQuery.data
  const timeline = useQuery({
    queryKey: ['task-timeline', tenantId, taskId], queryFn: () => getTaskTimeline(tenantId, taskId), enabled: !!tenantId && !!taskId,
    refetchInterval: live(task) ? 3000 : 10000,
  })
  const projects = useQuery({ queryKey: ['projects', tenantId], queryFn: () => getApiV2TenantsTidProjects(tenantId, { limit: 100 }), enabled: !!tenantId })
  const squads = useQuery({ queryKey: ['squads', tenantId], queryFn: () => listSquads(tenantId), enabled: !!tenantId })
  const agentTask = task?.assignee?.type === 'agent'
  const executors = useQuery({
    queryKey: ['task-executors', tenantId, taskId, task?.assignee?.id], queryFn: () => listTaskExecutors(tenantId, taskId), enabled: !!tenantId && agentTask,
  })
  const executorList = executors.data?.items ?? []
  // Until someone picks, the Agent's configured model, as the server defaults, before its fixture.
  const executorId = executorList.some(x => x.id === pickedExecutor)
    ? pickedExecutor
    : (executorList.find(x => !x.id.endsWith(':fixture')) ?? executorList[0])?.id ?? ''

  // The task is read most often; when its run changes state, the timeline and the sidebar's counts
  // follow at once instead of on their own slower beat.
  const runState = task ? `${task.activeRun?.id ?? ''}:${task.activeRun?.state ?? ''}:${task.status}` : ''
  useEffect(() => {
    if (!runState) return
    void client.invalidateQueries({ queryKey: ['task-timeline', tenantId, taskId] })
    void client.invalidateQueries({ queryKey: ['tasks', tenantId] })
  }, [runState, client, tenantId, taskId])
  const runs = timeline.data?.runs ?? []
  const comments = timeline.data?.comments ?? []
  const selected = runs.find(r => r.id === pickedRun) ?? runs.at(-1)
  const refresh = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['task', tenantId, taskId] }),
      client.invalidateQueries({ queryKey: ['task-timeline', tenantId, taskId] }),
      client.invalidateQueries({ queryKey: ['tasks', tenantId] }),
    ])
  }
  const showTab = (next: 'runs' | 'discussion') => {
    const p = new URLSearchParams(params)
    if (next === 'runs') p.delete('tab')
    else p.set('tab', next)
    setParams(p, { replace: true })
  }
  const showRun = (id: string) => { setPickedRun(id); showTab('runs') }

  const settle = (updated: Task) => {
    client.setQueryData(['task', tenantId, updated.id], updated)
    setError('')
    void client.invalidateQueries({ queryKey: ['tasks', tenantId] })
  }
  const save = useMutation({
    mutationFn: ({ draft, target }: { draft: TaskDraft; target: Task }) => updateTask(tenantId, target.id, { ...draft, version: target.version }),
    onSuccess: updated => { settle(updated); setEditing(false) },
    onError: e => setError(taskMessageFor(e)),
  })
  const move = useMutation({
    mutationFn: ({ status, target }: { status: TaskStatusProperty; target: Task }) => setTaskStatus(tenantId, target.id, { status, version: target.version }),
    onSuccess: updated => { settle(updated); void refresh() },
    onError: e => setError(taskRunMessageForStatus(e)),
  })
  const assign = useMutation({
    mutationFn: ({ assignee, target }: { assignee: TaskAssigneeRef | null; target: Task }) => setTaskAssignee(tenantId, target.id, { assignee, version: target.version }),
    onSuccess: settle,
    onError: e => setError(taskMessageFor(e)),
  })
  const start = useMutation({
    mutationFn: () => startTaskRun(tenantId, taskId, { executorId }, keyed()),
    onSuccess: async run => { setError(''); setPickedRun(run.id); showTab('runs'); await refresh() },
    onError: e => setError(taskRunMessageFor(e)),
  })
  const stop = useMutation({
    mutationFn: (runId: string) => stopTaskRun(tenantId, taskId, runId),
    onSuccess: async () => { setError(''); await refresh() },
    onError: e => setError(taskRunMessageFor(e)),
  })
  const comment = useMutation({
    mutationFn: (draft: CommentDraft) => createTaskComment(tenantId, taskId, { ...draft, executorId: draft.toAgent ? executorId : undefined }, keyed()),
    onSuccess: async (created, draft) => {
      setComposeError('')
      // Replying answers any @ waiting on the caller here.
      void client.invalidateQueries({ queryKey: ['task-mentions', tenantId] })
      if (draft.toAgent && created.runId) showRun(created.runId)
      else showTab('discussion')
      await refresh()
    },
    onError: e => setComposeError(taskRunMessageFor(e)),
  })

  if (taskQuery.isLoading || !tenantId) return <div className="mc-content"><p role="status">正在加载任务…</p></div>
  if (taskQuery.isError || !task) {
    return <div className="mc-content">
      <p role="alert">任务不存在或你已无权查看。</p>
      <Link to="/collaboration" className="mc-link">返回任务列表</Link>
    </div>
  }
  const projectList = projects.data?.items ?? []
  return <div className="mc-task-page">
    <header className="mc-task-bar">
      <Link to="/collaboration" className="mc-back" aria-label="返回任务列表"><ChevronLeft size={16} /></Link>
      <span className={`mc-status ${task.status}`}>{taskStatusLabel(task.status)}</span>
      <h2>{task.title}</h2>
      <Button type="button" variant="outline" size="sm" onClick={() => { setError(''); setEditing(true) }} disabled={save.isPending}><Pencil data-icon="inline-start" />编辑</Button>
    </header>
    <div className="mc-task-body">
      <section className="mc-task-main-col" aria-label="运行与讨论">
        {task.description && <p className="mc-description">{task.description}</p>}
        <div className="mc-tabs" role="tablist" aria-label="任务内容">
          <button type="button" role="tab" aria-selected={tab === 'runs'} onClick={() => showTab('runs')}>Agent 运行 <b>{runs.length}</b></button>
          <button type="button" role="tab" aria-selected={tab === 'discussion'} onClick={() => showTab('discussion')}>讨论 <b>{comments.length}</b></button>
        </div>
        <div className="mc-tab-body">
          {timeline.isError
            ? <p role="alert">运行与讨论加载失败，请刷新重试。</p>
            : tab === 'runs'
              ? <RunPane tenantId={tenantId} taskId={task.id} runs={runs} selected={selected} onSelect={setPickedRun} />
              : <TaskDiscussion comments={comments} runs={runs} onRun={showRun} />}
        </div>
        <TaskComposer tenantId={tenantId} task={task} me={me.id} sending={comment.isPending} error={composeError}
          onSend={async draft => { try { await comment.mutateAsync(draft); return true } catch { return false } }} />
      </section>
      <aside className="mc-task-side-col" aria-label="任务属性">
        <TaskDetailView task={task} projectName={projectLabel(projectList, task.projectId)} squads={squads.data?.items ?? []}
          busy={move.isPending || assign.isPending} error={error}
          run={{
            executors: executorList, executorId, onExecutor: setExecutorId, hasRuns: runs.length > 0,
            starting: start.isPending, stopping: stop.isPending,
            onStart: () => { setError(''); start.mutate() },
            onStop: () => { if (task.activeRun) { setError(''); stop.mutate(task.activeRun.id) } },
          }}
          onStatus={status => { setError(''); move.mutate({ status, target: task }) }}
          onAssign={next => { setError(''); assign.mutate({ assignee: next, target: task }) }} />
      </aside>
    </div>

    {editing && <PanelDialog icon={<ClipboardList size={14} />} eyebrow="任务" title={`编辑 ${task.title}`} subtitle="修改标题、描述与所属项目；状态与指派有各自的入口。" onClose={() => setEditing(false)}>
      <TaskForm initial={{ title: task.title, description: task.description, projectId: task.projectId }} projects={projectList} projectsLoading={projects.isLoading} submitLabel="保存修改" saving={save.isPending} error={error} onSave={draft => save.mutate({ draft, target: task })} onCancel={() => setEditing(false)} />
    </PanelDialog>}
  </div>
}

/** A refused manual move: the run's own reason when it owns the status, else the task's. */
function taskRunMessageForStatus(error: unknown): string {
  const message = taskRunMessageFor(error)
  return message === '操作失败，请重试。' ? taskMessageFor(error) : message
}
