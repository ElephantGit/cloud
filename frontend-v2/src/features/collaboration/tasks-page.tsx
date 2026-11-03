import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router'
import { ClipboardList, LayoutList, Plus, SquareKanban } from 'lucide-react'
import { useUser } from '@/app/auth/user'
import { Button } from '@/components/ui/button'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getApiV2TenantsTidProjects } from '@/lib/projects-api/generated'
import { listSquads } from '@/lib/squads-api/generated'
import { PanelDialog } from '@/components/panel/panel-dialog'
import { TaskForm, type TaskDraft } from './task-form'
import { TaskBoardColumns, TaskRows } from './task-list'
import { boardMoveMessageFor, taskMessageFor } from './task-model'
import { familyLabels, filterTasks, TASK_VIEWS, viewOf } from './task-views'
import { createTask, setTaskStatus, type Task, type TaskList, type TaskStatusProperty } from './tasks-api/generated'
import { usePendingMentions, useTaskList, useWorkspace } from './workspace'

/**
 * One scope of the team's tasks, as a board (the default) or a list. The scope and layout live in
 * the URL so a link shows a teammate the same view; `new=1` opens the create form the sidebar links to.
 */
export default function TasksPage() {
  const me = useUser()
  const { tenantId } = useWorkspace()
  const client = useQueryClient()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const view = viewOf(params.get('view'))
  const squadId = params.get('squad')
  const layout = params.get('layout') === 'list' ? 'list' : 'board'
  const creating = params.get('new') === '1'
  const [showCancelled, setShowCancelled] = useState(false)
  const [error, setError] = useState('')
  const list = useTaskList(tenantId)
  const mentions = usePendingMentions(tenantId)
  const projects = useQuery({ queryKey: ['projects', tenantId], queryFn: () => getApiV2TenantsTidProjects(tenantId, { limit: 100 }), enabled: !!tenantId })
  const squads = useQuery({ queryKey: ['squads', tenantId], queryFn: () => listSquads(tenantId), enabled: !!tenantId })
  const squad = squads.data?.items.find(s => s.id === squadId)
  const items = filterTasks(list.data?.items ?? [], view, me.id, squadId, new Set(mentions.keys()))
  const families = useMemo(() => familyLabels(list.data?.items ?? []), [list.data])
  const projectList = projects.data?.items ?? []
  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(params)
    if (value === null) next.delete(key)
    else next.set(key, value)
    setParams(next, { replace: true })
  }
  const openCreate = () => { setError(''); setParam('new', '1') }
  const closeCreate = () => { setError(''); setParam('new', null) }
  const open = (task: Task) => void navigate(`/collaboration/tasks/${task.id}`)
  const create = useMutation({
    mutationFn: (draft: TaskDraft) => createTask(tenantId, draft),
    onSuccess: async created => {
      await client.invalidateQueries({ queryKey: ['tasks', tenantId] })
      client.setQueryData(['task', tenantId, created.id], created)
      void navigate(`/collaboration/tasks/${created.id}`)
    },
    onError: e => setError(taskMessageFor(e)),
  })
  // A drop moves the card at once and puts it back if the server refuses, with the reason above
  // the board; the list is read again either way.
  const [notice, setNotice] = useState('')
  const listKey = ['tasks', tenantId]
  const move = useMutation({
    mutationFn: ({ task, status }: { task: Task; status: TaskStatusProperty }) => setTaskStatus(tenantId, task.id, { status, version: task.version }),
    onMutate: async ({ task, status }) => {
      setNotice('')
      await client.cancelQueries({ queryKey: listKey })
      const before = client.getQueryData<TaskList>(listKey)
      if (before) client.setQueryData<TaskList>(listKey, { ...before, items: before.items.map(t => (t.id === task.id ? { ...t, status } : t)) })
      return { before }
    },
    onError: (e, _vars, context) => {
      if (context?.before) client.setQueryData(listKey, context.before)
      setNotice(boardMoveMessageFor(e))
    },
    onSettled: () => client.invalidateQueries({ queryKey: listKey }),
  })
  const current = TASK_VIEWS.find(v => v.id === view)
  const title = squad ? squad.name : current?.label ?? '全部任务'
  const subtitle = squad ? '这个小队负责的任务；指派给小队不会自动分派给成员。' : current?.hint

  return <div className={layout === 'board' ? 'mc-content mc-content-board' : 'mc-content'}>
    <header className="mc-page-header">
      <div>
        <h2>{title}</h2>
        <p>{subtitle}</p>
      </div>
      <div className="mc-header-actions">
        <ToggleGroup aria-label="展示方式" value={[layout]} onValueChange={next => { const picked = next.at(-1); if (picked) setParam('layout', picked === 'list' ? 'list' : null) }}>
          <ToggleGroupItem value="board" aria-label="看板"><SquareKanban /></ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label="列表"><LayoutList /></ToggleGroupItem>
        </ToggleGroup>
        <Button onClick={openCreate} disabled={!tenantId || list.isLoading || list.isError}><Plus data-icon="inline-start" />新建任务</Button>
      </div>
    </header>

    {layout === 'board' && notice && <p role="alert" className="mc-error mc-board-notice">{notice}</p>}
    {layout === 'board' && <label className="mc-switch-line"><Switch checked={showCancelled} onCheckedChange={setShowCancelled} />显示已取消</label>}

    {list.isLoading
      ? <p role="status">正在加载任务…</p>
      : list.isError
        ? <p role="alert">任务加载失败，请刷新重试。</p>
        : items.length || layout === 'board'
          ? layout === 'board'
            ? <TaskBoardColumns tasks={items} projects={projectList} mentions={mentions} families={families} showCancelled={showCancelled} onOpen={open}
              onMove={(task, status) => move.mutate({ task, status: status as TaskStatusProperty })} onRefuse={setNotice} />
            : <TaskRows tasks={items} projects={projectList} mentions={mentions} families={families} onOpen={open} />
          : <Empty className="min-h-56 border">
            <EmptyHeader><EmptyMedia variant="icon"><ClipboardList /></EmptyMedia>
              <EmptyTitle>{view === 'all' && !squad ? '还没有任务' : '这里还没有任务'}</EmptyTitle>
              <EmptyDescription>建一个任务，指派给一个人、一个 Agent 或一个小队；指派给 Agent 的任务可以在任务里开始执行。</EmptyDescription>
            </EmptyHeader>
            <EmptyContent><Button variant="outline" onClick={openCreate} disabled={!tenantId}>创建任务</Button></EmptyContent>
          </Empty>}
    <footer className="mc-list-footer"><span>{items.length} 个任务</span><span>任务和运行全团队可见</span></footer>

    {creating && <PanelDialog icon={<ClipboardList size={14} />} eyebrow="任务" title="新建任务" subtitle="任务属于一个项目；创建后在任务里指派、执行与讨论。" onClose={closeCreate}>
      <TaskForm projects={projectList} projectsLoading={projects.isLoading} submitLabel="创建任务" saving={create.isPending} error={error} onSave={draft => create.mutate(draft)} onCancel={closeCreate} />
    </PanelDialog>}
  </div>
}
