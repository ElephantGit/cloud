import { ScheduleWorkflowProvider } from './workflow-provider'
import { useWorkflowLibrary } from '@/lib/workflows/data/workflows'
import { useUser } from '@/app/auth/user'
import { useTaskScheduleDemo, emptyDemoSchedules } from './task-demo-store'
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { statusOf } from '@/lib/api-client'
import { listScheduleConfigs, createScheduleConfig, updateScheduleConfig, setScheduleConfigEnabled } from './api/generated'
import { CalendarClock, Check, ChevronRight, Clock3, FlaskConical, Plus, Search, SlidersHorizontal } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { catalog, formatTime, nextTimes, type Catalog, type Draft, type Task } from './schedule-model'
import { ScheduleDrawer } from './schedule-drawer'
import { ScheduleForm } from './schedule-form'
import { ScheduleDetail } from './schedule-detail'
import { ScheduleTable } from './schedule-table'
import './schedules.css'

type Panel = { kind: 'create' } | { kind: 'edit'; id: string } | { kind: 'detail'; id: string; tab?: 'config' | 'history' } | null

/** Persistent schedule configuration; execution is not yet connected. */
export default function Page() {
  return <ScheduleWorkflowProvider><SchedulePageContext /></ScheduleWorkflowProvider>
}

function SchedulePageContext() {
  const tenants = useGetApiV1MeTenants()
  const user = useUser()
  const tenantId = tenants.data?.items[0]?.id ?? ''
  const scope = `${user.id}:${tenantId}`
  return <SchedulesPage key={scope} tenantId={tenantId} scope={scope} tenantsLoading={tenants.isLoading} tenantsError={tenants.isError} />
}

function SchedulesPage({ tenantId, scope, tenantsLoading, tenantsError }: { tenantId: string; scope: string; tenantsLoading: boolean; tenantsError: boolean }) {
  const closeForm = useRef<() => void>(() => setPanel(null))
  const library = useWorkflowLibrary()
  const workflowCatalog: Catalog = { ...catalog, workflows: (library.data ?? []).map(w => ({ id: w.id, name: w.name, description: '', versions: w.publishedVersion ? [w.publishedVersion] : [], fields: [] })) }
  const sources = useTaskScheduleDemo(s => s.sources)
  const demos = useTaskScheduleDemo(s => s.schedules[scope] ?? emptyDemoSchedules)
  const client = useQueryClient()
  const configs = useQuery({ queryKey: ['schedule-configs', tenantId], queryFn: () => listScheduleConfigs(tenantId), enabled: !!tenantId })
  function currentProject(task: Task) { const target = task.target; return target.kind === 'task' ? sources.find(s => s.id === target.sourceTaskId)?.projectId ?? '' : task.projectId }
  const tasks: Task[] = [...demos.map(task => ({ ...task, projectId: currentProject(task) })), ...(configs.data?.items ?? []).map(item => ({ ...item, runs: [] }))]
  const refresh = () => client.invalidateQueries({ queryKey: ['schedule-configs', tenantId] })
  const [saveError, setSaveError] = useState('')
  const [conflict, setConflict] = useState(false)
  const [editTask, setEditTask] = useState<Task>()
  const saveMutation = useMutation({ mutationFn: async ({ draft, task }: { draft: Draft; task?: Task }) => {
    if (draft.kind !== 'chat') throw new Error('工作流尚未接入')
    const data = { name: draft.name.trim(), projectId: draft.projectId, target: { kind: 'chat' as const, prompt: draft.prompt.trim(), agent: draft.agent, model: draft.model }, rule: draft.rule }
    return task ? updateScheduleConfig(tenantId, task.id, { ...data, version: task.version! }) : createScheduleConfig(tenantId, data)
  }, onSuccess: async (item, variables) => { await refresh(); setPanel(current => current && (current.kind === 'create' || (current.kind === 'edit' && current.id === variables.task?.id)) ? { kind: 'detail', id: item.id } : current); setSaveError(''); setConflict(false); setToast(variables.task ? '修改已保存' : '任务配置已保存') }, onError: error => { setConflict(statusOf(error) === 409); setSaveError(statusOf(error) === 409 ? '任务已被其他操作修改，请关闭后重新打开。' : '保存失败，请重试。'); if (statusOf(error) === 409) void refresh() } })
  const toggleMutation = useMutation({ mutationFn: (task: Task) => setScheduleConfigEnabled(tenantId, task.id, { version: task.version!, enabled: !task.enabled }), onSuccess: async () => { await refresh(); setToast('任务状态已更新') }, onError: error => { setToast(statusOf(error) === 409 ? '任务已被其他操作修改，请重试' : '更新失败，请重试'); void refresh() } })
  const [panel, setPanel] = useState<Panel>(null)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [kind, setKind] = useState('all')
  const [project, setProject] = useState('all')
  const [toast, setToast] = useState('')
  const [now, setNow] = useState(() => new Date())
  useEffect(() => { const timer = setInterval(() => setNow(new Date()), 30_000); return () => clearInterval(timer) }, [])
  useEffect(() => { if (!toast) return; const timer = setTimeout(() => setToast(''), 3500); return () => clearTimeout(timer) }, [toast])

  const filtered = tasks.filter(t => t.name.toLowerCase().includes(query.toLowerCase()) && (status === 'all' || (status === 'enabled' ? t.enabled : !t.enabled)) && (kind === 'all' || t.target.kind === kind) && (project === 'all' || (project === 'none' ? !t.projectId : t.projectId === project)))
  const panelTask = panel && 'id' in panel ? tasks.find(t => t.id === panel.id) : undefined
  const enabled = tasks.filter(t => t.enabled).length
  const nearest = tasks.filter(t => t.enabled).flatMap(t => nextTimes(t.rule, now, new Date(t.createdAt)).slice(0, 1).map(date => ({ task: t, date }))).sort((a, b) => a.date.getTime() - b.date.getTime())[0]
  const open = (task: Task, tab?: 'config' | 'history') => setPanel({ kind: 'detail', id: task.id, tab })
  const saved = (draft: Draft, data: Catalog) => {
    setSaveError('')
    if (draft.kind !== 'chat') {
      const id = useTaskScheduleDemo.getState().save(scope, draft, panel?.kind === 'edit' ? panel.id : undefined, data)
      if (!id) { setSaveError('任务或工作流配置已不可用，请重新选择。'); return }
      setPanel({ kind: 'detail', id }); setToast('演示配置已保存，刷新后清除'); return
    }
    if (editTask && editTask.target.kind !== 'chat') { setSaveError('请新建直接对话配置。'); return }
    saveMutation.mutate({ draft, task: panel?.kind === 'edit' ? editTask : undefined })
  }
  const toggleTask = (task: Task) => { if (task.target.kind !== 'chat') { useTaskScheduleDemo.getState().toggle(scope, task.id); return } if (!toggleMutation.isPending) toggleMutation.mutate(task) }
  const openCreate = () => { setConflict(false); setSaveError(''); setEditTask(undefined); setPanel({ kind: 'create' }) }
  const clearFilters = () => { setQuery(''); setStatus('all'); setKind('all'); setProject('all') }

  return <div className="schedules-feature sp-page">
    <div className="sp-workspace-bar">
      <span className="sp-breadcrumb">工作空间<ChevronRight size={13} />自动化</span>
      <Badge variant="outline"><FlaskConical data-icon="inline-start" />配置管理</Badge>
    </div>
    <div className="sp-content">
      <header className="sp-page-header">
        <div><h1>定时任务<Badge variant="secondary" aria-hidden="true">{tasks.length}</Badge></h1><p>保存任务配置；自动执行尚未接入。</p></div>
        <Button disabled={!tenantId || configs.isLoading || configs.isError} onClick={openCreate}><Plus data-icon="inline-start" />新建任务</Button>
      </header>

      <div className="sp-summary-strip" aria-label="任务概览">
        <div className="sp-summary-counts"><span><i className="sp-dot enabled" />已启用 <strong>{enabled}</strong></span><span><i className="sp-dot paused" />已暂停 <strong>{tasks.length - enabled}</strong></span></div>
        <span className="sp-next-run"><Clock3 size={14} />{nearest ? <>预计时间 <strong>{formatTime(nearest.date, nearest.task.rule.timezone)}</strong><span className="sp-next-task">· {nearest.task.name}</span></> : '暂无启用的计划'}</span>
      </div>

      <section className="sp-task-section" aria-label="定时任务列表">
        <div className="sp-filters">
          <div className="sp-search"><Search size={15} aria-hidden="true" /><Input aria-label="搜索任务" placeholder="搜索任务…" value={query} onChange={e => setQuery(e.target.value)} /></div>
          <div className="sp-filter-selects"><SlidersHorizontal size={14} aria-hidden="true" />
            <NativeSelect aria-label="状态筛选" value={status} onChange={e => setStatus(e.target.value)}><NativeSelectOption value="all">全部状态</NativeSelectOption><NativeSelectOption value="enabled">已启用</NativeSelectOption><NativeSelectOption value="paused">已暂停</NativeSelectOption></NativeSelect>
            <NativeSelect aria-label="执行方式筛选" value={kind} onChange={e => setKind(e.target.value)}><NativeSelectOption value="all">全部类型</NativeSelectOption><NativeSelectOption value="chat">直接对话</NativeSelectOption><NativeSelectOption value="workflow">工作流</NativeSelectOption><NativeSelectOption value="task">已有任务</NativeSelectOption></NativeSelect>
            <NativeSelect aria-label="项目筛选" value={project} onChange={e => setProject(e.target.value)}><NativeSelectOption value="all">全部项目</NativeSelectOption><NativeSelectOption value="none">不关联项目</NativeSelectOption>{[...new Map([...sources.map(s => ({ id: s.projectId, name: s.projectName })), ...catalog.projects].filter(p => tasks.some(t => t.projectId === p.id)).map(p => [p.id, p.name])).entries()].map(([id, name]) => <NativeSelectOption key={id} value={id}>{name}</NativeSelectOption>)}</NativeSelect>
          </div>
        </div>

        {tenantsLoading || configs.isLoading ? <p role="status">正在加载任务…</p> : tenantsError || configs.isError ? <p role="alert">加载失败，请刷新重试。</p> : !tenantId ? <p role="alert">没有可用的工作空间。</p> : filtered.length ? <ScheduleTable data={workflowCatalog} sources={sources} tasks={filtered} now={now} onOpen={open} onToggle={toggleTask} /> : <Empty className="min-h-72 border">
          <EmptyHeader><EmptyMedia variant="icon"><CalendarClock /></EmptyMedia><EmptyTitle>{tasks.length ? '没有匹配的任务' : '安排第一个定时任务'}</EmptyTitle><EmptyDescription>{tasks.length ? '调整关键词或筛选条件，再试一次。' : '从每日简报或每周项目检查开始。'}</EmptyDescription></EmptyHeader>
          <EmptyContent><Button variant="outline" onClick={() => tasks.length ? clearFilters() : openCreate()}>{tasks.length ? '清除筛选' : '新建任务'}</Button></EmptyContent>
        </Empty>}
        <footer className="sp-list-footer"><span>{filtered.length} 项任务<span className="sp-footer-divider">·</span>时间按各任务设置的时区显示</span><span>配置已保存，自动执行尚未接入</span></footer>
      </section>
    </div>

    {toast && <div className="sp-toast" role="status"><Check size={16} />{toast}</div>}
    {panel && <ScheduleDrawer title={panel.kind === 'create' ? '新建定时任务' : panel.kind === 'edit' ? '编辑定时任务' : panelTask?.name ?? '任务详情'} subtitle={panel.kind === 'detail' ? '查看任务配置与每次运行的结果。' : '配置执行内容、项目与时间安排。'} onClose={() => panel.kind === 'detail' ? setPanel(null) : closeForm.current()}>
      {panel.kind === 'create' || panel.kind === 'edit' ? <ScheduleForm onCloseReady={fn => { closeForm.current = fn }} data={{ ...catalog, tasks: sources }} saving={saveMutation.isPending} blocked={conflict} saveError={saveError} key={panel.kind === 'edit' ? panel.id : 'create'} task={panel.kind === 'edit' ? editTask : undefined} onSave={saved} onCancel={() => setPanel(null)} /> : panelTask && <ScheduleDetail data={workflowCatalog} key={`${panelTask.id}-${panel.tab}`} task={panelTask} sources={sources} initialTab={panel.tab} onSimulate={panelTask.target.kind === 'task' ? fail => useTaskScheduleDemo.getState().run(scope, panelTask.id, fail) : undefined} onEdit={() => { setEditTask(structuredClone(panelTask)); setConflict(false); setSaveError(''); setPanel({ kind: 'edit', id: panelTask.id }) }} onToggle={() => toggleTask(panelTask)} />}
    </ScheduleDrawer>}
  </div>
}
