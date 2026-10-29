import { useEffect, useState } from 'react'
import { CalendarClock, Check, ChevronRight, Clock3, FlaskConical, Plus, Search, SlidersHorizontal } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { catalog, formatTime, nextTimes, type Draft, type Task } from './schedule-model'
import { useMockSchedules } from './mock-store'
import { ScheduleDrawer } from './schedule-drawer'
import { ScheduleForm } from './schedule-form'
import { ScheduleDetail } from './schedule-detail'
import { ScheduleTable } from './schedule-table'
import './schedules.css'

type Panel = { kind: 'create' } | { kind: 'edit'; id: string } | { kind: 'detail'; id: string; tab?: 'config' | 'history' } | null

/** Schedule management with a mock data source pending backend integration. */
export default function Page() {
  const { tasks, save, toggle, remove } = useMockSchedules()
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
  const failed = tasks.filter(t => t.runs[0]?.status === 'failed').length
  const nearest = tasks.filter(t => t.enabled).flatMap(t => nextTimes(t.rule, now, new Date(t.createdAt)).slice(0, 1).map(date => ({ task: t, date }))).sort((a, b) => a.date.getTime() - b.date.getTime())[0]
  const open = (task: Task, tab?: 'config' | 'history') => setPanel({ kind: 'detail', id: task.id, tab })
  const saved = (draft: Draft) => {
    const id = save(draft, panel?.kind === 'edit' ? panel.id : undefined)
    setPanel({ kind: 'detail', id })
    setToast(panel?.kind === 'edit' ? '修改已保存 · 仅本次演示' : '任务已创建并启用 · 仅本次演示')
  }
  const toggleTask = (task: Task) => {
    toggle(task.id)
    setToast(task.enabled ? '任务已暂停，运行记录已保留' : '任务已恢复 · 模拟数据不会实际调度')
  }
  const clearFilters = () => { setQuery(''); setStatus('all'); setKind('all'); setProject('all') }

  return <div className="schedules-feature sp-page">
    <div className="sp-workspace-bar">
      <span className="sp-breadcrumb">工作空间<ChevronRight size={13} />自动化</span>
      <Badge variant="outline"><FlaskConical data-icon="inline-start" />模拟数据</Badge>
    </div>
    <div className="sp-content">
      <header className="sp-page-header">
        <div><h1>定时任务<Badge variant="secondary" aria-hidden="true">{tasks.length}</Badge></h1><p>安排重复工作，让对话与工作流按时运行。</p></div>
        <Button onClick={() => setPanel({ kind: 'create' })}><Plus data-icon="inline-start" />新建任务</Button>
      </header>

      <div className="sp-summary-strip" aria-label="任务概览">
        <div className="sp-summary-counts"><span><i className="sp-dot enabled" />已启用 <strong>{enabled}</strong></span><span><i className="sp-dot paused" />已暂停 <strong>{tasks.length - enabled}</strong></span><span><i className="sp-dot failed" />最近失败 <strong>{failed}</strong></span></div>
        <span className="sp-next-run"><Clock3 size={14} />{nearest ? <>下次运行 <strong>{formatTime(nearest.date, nearest.task.rule.timezone)}</strong><span className="sp-next-task">· {nearest.task.name}</span></> : '暂无启用的计划'}</span>
      </div>

      <section className="sp-task-section" aria-label="定时任务列表">
        <div className="sp-filters">
          <div className="sp-search"><Search size={15} aria-hidden="true" /><Input aria-label="搜索任务" placeholder="搜索任务…" value={query} onChange={e => setQuery(e.target.value)} /></div>
          <div className="sp-filter-selects"><SlidersHorizontal size={14} aria-hidden="true" />
            <NativeSelect aria-label="状态筛选" value={status} onChange={e => setStatus(e.target.value)}><NativeSelectOption value="all">全部状态</NativeSelectOption><NativeSelectOption value="enabled">已启用</NativeSelectOption><NativeSelectOption value="paused">已暂停</NativeSelectOption></NativeSelect>
            <NativeSelect aria-label="执行方式筛选" value={kind} onChange={e => setKind(e.target.value)}><NativeSelectOption value="all">全部类型</NativeSelectOption><NativeSelectOption value="chat">直接对话</NativeSelectOption><NativeSelectOption value="workflow">工作流</NativeSelectOption></NativeSelect>
            <NativeSelect aria-label="项目筛选" value={project} onChange={e => setProject(e.target.value)}><NativeSelectOption value="all">全部项目</NativeSelectOption><NativeSelectOption value="none">不关联项目</NativeSelectOption>{catalog.projects.map(p => <NativeSelectOption key={p.id} value={p.id}>{p.name}</NativeSelectOption>)}</NativeSelect>
          </div>
        </div>

        {filtered.length ? <ScheduleTable tasks={filtered} now={now} onOpen={open} onToggle={toggleTask} /> : <Empty className="min-h-72 border">
          <EmptyHeader><EmptyMedia variant="icon"><CalendarClock /></EmptyMedia><EmptyTitle>{tasks.length ? '没有匹配的任务' : '安排第一个定时任务'}</EmptyTitle><EmptyDescription>{tasks.length ? '调整关键词或筛选条件，再试一次。' : '从每日简报或每周项目检查开始。'}</EmptyDescription></EmptyHeader>
          <EmptyContent><Button variant="outline" onClick={() => tasks.length ? clearFilters() : setPanel({ kind: 'create' })}>{tasks.length ? '清除筛选' : '新建任务'}</Button></EmptyContent>
        </Empty>}
        <footer className="sp-list-footer"><span>{filtered.length} 项任务<span className="sp-footer-divider">·</span>时间按各任务设置的时区显示</span><span>模拟数据，刷新后重置</span></footer>
      </section>
    </div>

    {toast && <div className="sp-toast" role="status"><Check size={16} />{toast}</div>}
    {panel && <ScheduleDrawer title={panel.kind === 'create' ? '新建定时任务' : panel.kind === 'edit' ? '编辑定时任务' : panelTask?.name ?? '任务详情'} subtitle={panel.kind === 'detail' ? '查看任务配置与每次运行的结果。' : '配置执行内容、项目与时间安排。'} onClose={() => setPanel(null)}>
      {panel.kind === 'create' || panel.kind === 'edit' ? <ScheduleForm key={panel.kind === 'edit' ? panel.id : 'create'} task={panelTask} onSave={saved} onCancel={() => setPanel(null)} /> : panelTask && <ScheduleDetail key={`${panelTask.id}-${panel.tab}`} task={panelTask} initialTab={panel.tab} onEdit={() => setPanel({ kind: 'edit', id: panelTask.id })} onDelete={() => { remove(panelTask.id); setPanel(null); setToast('已删除演示任务') }} />}
    </ScheduleDrawer>}
  </div>
}
