import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { ArrowLeft, ArrowRight, ArrowUpRight, CalendarClock, Check, ChevronRight, Clock3, Folder, GitBranch, MessageSquare, Plus, RotateCcw, Search, SlidersHorizontal, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { catalog, formatTime, nextTimes, projectName, ruleText, type Draft, type Task } from './prototype-model'
import { usePrototype } from './prototype-store'
import { PrototypeDialog } from './prototype-dialog'
import { PrototypeForm } from './prototype-form'
import { PrototypeDetail, RunStatus } from './prototype-detail'
import './prototype.css'

const variants = ['A', 'B', 'C'] as const
type Variant = typeof variants[number]
type Panel = { kind: 'create' } | { kind: 'edit'; id: string } | { kind: 'detail'; id: string; tab?: 'config' | 'history' } | null

/** Three throwaway layouts share in-memory actions, on the existing schedules route. */
export default function Page() {
  const { tasks, save, toggle, remove, reset } = usePrototype()
  const [params, setParams] = useSearchParams()
  const raw = params.get('variant')
  const variant: Variant = raw === 'B' || raw === 'C' ? raw : 'A'
  const [panel, setPanel] = useState<Panel>(null)
  const [selectedId, setSelectedId] = useState(tasks[0]?.id)
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [kind, setKind] = useState('all')
  const [project, setProject] = useState('all')
  const [toast, setToast] = useState('')
  const [resetConfirm, setResetConfirm] = useState(false)
  const [now, setNow] = useState(() => new Date())
  const changeVariant = (v: Variant) => { const next = new URLSearchParams(params); next.set('variant', v); setParams(next, { replace: true }); setPanel(null) }
  useEffect(() => { const timer = setInterval(() => setNow(new Date()), 30_000); return () => clearInterval(timer) }, [])
  useEffect(() => { if (!toast) return; const timer = setTimeout(() => setToast(''), 3500); return () => clearTimeout(timer) }, [toast])
  useEffect(() => {
    if (!import.meta.env.DEV) return
    const keydown = (e: KeyboardEvent) => {
      if (panel || e.target instanceof HTMLElement && e.target.closest('input, textarea, select, button, [contenteditable="true"], [role="tab"]')) return
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
      e.preventDefault()
      const index = (variants.indexOf(variant) + (e.key === 'ArrowRight' ? 1 : 2)) % 3
      const next = new URLSearchParams(params); next.set('variant', variants[index]!); setParams(next, { replace: true })
    }
    window.addEventListener('keydown', keydown)
    return () => window.removeEventListener('keydown', keydown)
  }, [variant, params, setParams, panel])
  const filtered = tasks.filter(t => t.name.toLowerCase().includes(query.toLowerCase()) && (status === 'all' || (status === 'enabled' ? t.enabled : !t.enabled)) && (kind === 'all' || t.target.kind === kind) && (project === 'all' || (project === 'none' ? !t.projectId : t.projectId === project)))
  const selected = filtered.find(t => t.id === selectedId) ?? filtered[0]
  const panelTask = panel && 'id' in panel ? tasks.find(t => t.id === panel.id) : undefined
  const enabled = tasks.filter(t => t.enabled).length
  const failed = tasks.filter(t => t.runs[0]?.status === 'failed').length
  const nearest = tasks.filter(t => t.enabled).flatMap(t => nextTimes(t.rule, now, new Date(t.createdAt)).slice(0, 1).map(date => ({ task: t, date }))).sort((a,b) => a.date.getTime() - b.date.getTime())[0]
  const open = (task: Task, tab?: 'config' | 'history') => { setSelectedId(task.id); if (variant !== 'B' || tab) setPanel({ kind: 'detail', id: task.id, tab }) }
  const saved = (draft: Draft) => { const id = save(draft, panel?.kind === 'edit' ? panel.id : undefined); setSelectedId(id); setPanel({ kind: 'detail', id }); setToast(panel?.kind === 'edit' ? '修改已保存 · 仅本次演示' : '任务已创建并启用 · 仅本次演示') }
  const toggleTask = (task: Task) => { toggle(task.id); setToast(task.enabled ? '任务已暂停，运行记录已保留' : '任务已恢复 · 原型不会实际调度') }
  const timing = (task: Task) => task.enabled ? formatTime(nextTimes(task.rule, now, new Date(task.createdAt))[0]!, task.rule.timezone) : '暂停中'
  return <div className={`schedule-prototype sp-page sp-variant-${variant}`}>
    <header className="sp-page-header"><div><div className="sp-eyebrow">WORKSPACE / AUTOMATIONS</div><h1>定时任务<span className="sp-count" aria-hidden="true">{tasks.length}</span></h1><p>把重复的事交给 Ora，把时间留给更重要的事。</p></div><Button onClick={() => setPanel({ kind: 'create' })}><Plus size={16} />新建任务</Button></header>
    <div className="sp-overview"><div className="sp-overview-intro"><span className="sp-overview-icon"><CalendarClock size={24} strokeWidth={1.5} /></span><div><strong>让工作按时发生</strong><p>一段指令，或一套工作流。安排一次，持续跟进。</p></div></div><div className="sp-metric"><strong>{enabled}<span> / {tasks.length}</span></strong><small>已启用</small></div><div className="sp-metric"><strong>{failed.toString().padStart(2, '0')}</strong><small>最近运行失败</small></div><div className="sp-next"><small><span className="sp-live-dot" />下一次运行</small><strong>{nearest ? `${formatTime(nearest.date, nearest.task.rule.timezone)} · ${nearest.task.name}` : '暂无计划'}</strong><small>{nearest?.task.rule.timezone ?? '创建任务，安排下一步'}</small></div></div>
    <div className="sp-list-heading"><div><h2>我的任务</h2><span>{filtered.length} 项任务</span></div><p><Sparkles size={13} />模拟数据 · 刷新后重置</p></div>
    <div className="sp-filters"><label className="sp-search"><Search size={16} /><input aria-label="搜索任务" placeholder="搜索任务…" value={query} onChange={e => setQuery(e.target.value)} /></label><div className="sp-filter-selects"><SlidersHorizontal size={15} /><select aria-label="状态筛选" value={status} onChange={e => setStatus(e.target.value)}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="paused">已暂停</option></select><select aria-label="执行方式筛选" value={kind} onChange={e => setKind(e.target.value)}><option value="all">全部类型</option><option value="chat">直接对话</option><option value="workflow">工作流</option></select><select aria-label="项目筛选" value={project} onChange={e => setProject(e.target.value)}><option value="all">全部项目</option><option value="none">不关联项目</option>{catalog.projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div></div>
    {!filtered.length ? <div className="sp-empty"><CalendarClock size={34} strokeWidth={1} /><h3>{tasks.length ? '没有匹配的任务' : '安排你的第一个定时任务'}</h3><p>{tasks.length ? '试试其他关键词，或者清除筛选条件。' : '从每日简报或每周项目检查开始。'}</p><Button variant="outline" onClick={() => tasks.length ? (setQuery(''), setStatus('all'), setKind('all'), setProject('all')) : setPanel({ kind: 'create' })}>{tasks.length ? '清除筛选' : '新建任务'}</Button></div> : variant === 'A' ? <div className="sp-table-wrap"><table className="sp-table"><thead><tr><th>任务</th><th>关联项目</th><th>时间安排</th><th>最近运行</th><th>启用</th><th><span className="sp-sr-only">详情</span></th></tr></thead><tbody>{filtered.map(task => <tr key={task.id}><td><button className="sp-task-cell" onClick={() => open(task)}><span className={`sp-task-icon ${task.target.kind}`}><TaskIcon task={task} /></span><span><strong>{task.name}</strong><small>{task.target.kind === 'chat' ? '直接对话' : '工作流'}{task.target.kind === 'workflow' ? ` · ${task.target.snapshot}` : ' · 每次新建'}</small></span></button></td><td><span className={`sp-project-label ${task.projectId ? '' : 'muted'}`}><Folder size={14} />{projectName(task.projectId)}</span></td><td><div className="sp-cell-stack"><span>{ruleText(task.rule)}</span><small>{task.enabled ? '下次 ' : ''}{timing(task)}</small></div></td><td><button className="sp-run-cell" onClick={() => open(task, 'history')}><RunStatus status={task.runs[0]?.status} /><small>{task.runs[0] ? formatTime(task.runs[0].startedAt, task.rule.timezone) : '等待首次运行'}</small></button></td><td><Toggle task={task} onClick={() => toggleTask(task)} /></td><td><button className="sp-icon-button" aria-label={`查看${task.name}`} onClick={() => open(task)}><ChevronRight size={17} /></button></td></tr>)}</tbody></table><div className="sp-table-footer"><span>所有时间均按任务设置的时区显示</span><span>共 {filtered.length} 项</span></div></div> : variant === 'B' ? <div className="sp-master-detail"><div className="sp-master-list">{filtered.map(task => <button key={task.id} className={selected?.id === task.id ? 'active' : ''} onClick={() => setSelectedId(task.id)}><span className={`sp-task-icon ${task.target.kind}`}><TaskIcon task={task} /></span><span><strong>{task.name}</strong><small>{ruleText(task.rule)}</small><RunStatus status={task.runs[0]?.status} /></span><span className={`sp-tiny-dot ${task.enabled ? '' : 'paused'}`} /></button>)}</div>{selected && <PrototypeDetail key={selected.id} task={selected} embedded onEdit={() => setPanel({ kind: 'edit', id: selected.id })} onDelete={() => { remove(selected.id); setToast('已删除演示任务') }} />}</div> : <div className="sp-cards">{filtered.map(task => <article className="sp-card" key={task.id}><header><span className={`sp-task-icon ${task.target.kind}`}><TaskIcon task={task} /></span><span className="sp-badge">{task.target.kind === 'chat' ? '直接对话' : '工作流'}</span><span className="sp-footer-spacer" /><Toggle task={task} onClick={() => toggleTask(task)} /></header><button className="sp-card-title" onClick={() => open(task)}>{task.name}<ArrowUpRight size={18} /></button><p>{task.target.kind === 'chat' ? task.target.prompt : catalog.workflows.find(w => task.target.kind === 'workflow' && w.id === task.target.workflowId)?.description}</p><div className="sp-card-context"><Folder size={13} />{projectName(task.projectId)}</div><div className="sp-card-time"><Clock3 size={15} /><span>{ruleText(task.rule)}<small>{task.enabled ? '下次 ' : ''}{timing(task)}</small></span></div><footer><RunStatus status={task.runs[0]?.status} /><button onClick={() => open(task, 'history')}>查看记录<ArrowRight size={13} /></button></footer></article>)}</div>}
    <div className="sp-page-note"><span>先安排，再专注。</span><p>每次执行都有独立记录，结果随时可查。</p></div>
    {import.meta.env.DEV && <div className="sp-prototype-bar" aria-label="原型布局切换"><span className="sp-prototype-label">PROTOTYPE</span><button aria-label="上一个布局" onClick={() => changeVariant(variants[(variants.indexOf(variant) + 2) % 3]!)}><ArrowLeft size={14} /></button>{variants.map(v => <button key={v} aria-pressed={variant === v} className={variant === v ? 'active' : ''} onClick={() => changeVariant(v)}>{v}<span>{v === 'A' ? '列表' : v === 'B' ? '工作台' : '卡片'}</span></button>)}<button aria-label="下一个布局" onClick={() => changeVariant(variants[(variants.indexOf(variant) + 1) % 3]!)}><ArrowRight size={14} /></button><i /><button aria-label="重置演示" title="重置演示" onClick={() => setResetConfirm(true)}><RotateCcw size={14} /></button></div>}
    {toast && <div className="sp-toast" role="status"><Check size={16} />{toast}</div>}
    {panel && <PrototypeDialog title={panel.kind === 'create' ? '新建定时任务' : panel.kind === 'edit' ? '编辑定时任务' : panelTask?.name ?? '任务详情'} subtitle={panel.kind === 'detail' ? '任务配置与每次执行的结果，都在这里。' : '告诉 Ora 做什么，以及什么时候开始。'} onClose={() => setPanel(null)} wide={variant === 'C' && panel.kind !== 'detail'}>
      {panel.kind === 'create' || panel.kind === 'edit' ? <PrototypeForm key={panel.kind === 'edit' ? panel.id : `create-${variant}`} task={panelTask} wizard={variant === 'C'} onSave={saved} onCancel={() => setPanel(null)} /> : panelTask && <PrototypeDetail key={`${panelTask.id}-${panel.tab}`} task={panelTask} initialTab={panel.tab} onEdit={() => setPanel({ kind: 'edit', id: panelTask.id })} onDelete={() => { remove(panelTask.id); setPanel(null); setToast('已删除演示任务') }} />}
    </PrototypeDialog>}
    {resetConfirm && <PrototypeDialog title="重置演示数据" onClose={() => setResetConfirm(false)}><div className="sp-confirm-body">本次创建、修改和模拟运行记录将被清除，恢复四个样例任务。</div><footer className="sp-form-footer"><Button variant="outline" onClick={() => setResetConfirm(false)}>取消</Button><Button onClick={() => { reset(); setResetConfirm(false); setSelectedId('morning'); setQuery(''); setStatus('all'); setKind('all'); setProject('all'); setToast('已恢复样例数据') }}>确认重置</Button></footer></PrototypeDialog>}
  </div>
}
function TaskIcon({ task }: { task: Task }) { return task.target.kind === 'chat' ? <MessageSquare size={19} strokeWidth={1.6} /> : <GitBranch size={19} strokeWidth={1.6} /> }
function Toggle({ task, onClick }: { task: Task; onClick: () => void }) { return <button role="switch" aria-checked={task.enabled} aria-label={`${task.name}启用状态`} className={`sp-toggle ${task.enabled ? 'on' : ''}`} onClick={onClick}><span /></button> }
