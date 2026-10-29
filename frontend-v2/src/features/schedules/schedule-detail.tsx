import { useState } from 'react'
import { ArrowUpRight, Check, ChevronRight, CircleAlert, Clock3, GitBranch, LoaderCircle, MessageSquare, Pause, Pencil, Play, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { catalog, formatTime, nextTimes, projectName, ruleText, workflowName, type Task } from './schedule-model'
import { useMockSchedules } from './mock-store'

export function RunStatus({ status }: { status?: Task['runs'][number]['status'] }) {
  return <span className={`sp-run-status ${status ?? 'none'}`}>{status === 'running' ? <LoaderCircle size={13} className="sp-spin" /> : status === 'success' ? <Check size={13} /> : status === 'failed' ? <CircleAlert size={13} /> : <Clock3 size={13} />}{status === 'running' ? '运行中' : status === 'success' ? '成功' : status === 'failed' ? '失败' : '尚未运行'}</span>
}
export function ScheduleDetail({ task, onEdit, onDelete, initialTab = 'config' }: { task: Task; onEdit: () => void; onDelete: () => void; initialTab?: 'config' | 'history' }) {
  const [tab, setTab] = useState(initialTab)
  const [runId, setRunId] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [previewNow] = useState(() => new Date())
  const { toggle, simulate } = useMockSchedules()
  const selected = task.runs.find(r => r.id === runId) ?? task.runs[0]
  const busy = task.runs.some(r => r.status === 'running')
  const run = (fail = false) => { simulate(task.id, fail); setTab('history'); setRunId(null) }
  return <div className="sp-detail">
    <div className="sp-detail-toolbar"><span className={`sp-enabled ${task.enabled ? '' : 'paused'}`}><i />{task.enabled ? '已启用' : '已暂停'}</span><span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={onEdit}><Pencil size={14} />编辑</Button><Button size="sm" disabled={busy} onClick={() => run()}>{busy ? <LoaderCircle size={14} className="sp-spin" /> : <Play size={14} />}{busy ? '模拟中' : '模拟运行'}</Button></div>
    <div className="sp-tabs" role="tablist" aria-label="任务详情"><button role="tab" aria-selected={tab === 'config'} onClick={() => setTab('config')}>任务配置</button><button role="tab" aria-selected={tab === 'history'} onClick={() => setTab('history')}>运行记录 <span>{task.runs.length}</span></button></div>
    <div className="sp-detail-scroll" role="tabpanel" aria-label={tab === 'config' ? '任务配置' : '运行记录'}>
      {tab === 'config' ? <>
        <div className="sp-config-heading">执行内容</div>
        <div className="sp-target-title">{task.target.kind === 'chat' ? <MessageSquare size={20} /> : <GitBranch size={20} />}<strong>{task.target.kind === 'chat' ? '直接对话' : workflowName(task.target.workflowId)}</strong>{task.target.kind === 'workflow' && <Badge variant="secondary">{task.target.snapshot} · 固定版本</Badge>}</div>
        {task.target.kind === 'chat' ? <><p className="sp-prompt-preview">{task.target.prompt}</p><p className="sp-helper">每次创建独立对话 · {task.target.agent} · {task.target.model}</p></> : <dl className="sp-key-values">{Object.entries(task.target.inputs).map(([key, value]) => <div key={key}><dt>{catalog.workflows.find(w => task.target.kind === 'workflow' && w.id === task.target.workflowId)?.fields.find(f => f.key === key)?.name ?? key}</dt><dd>{value || '未填写'}</dd></div>)}</dl>}
        <div className="sp-config-heading">运行环境</div><dl className="sp-key-values"><div><dt>关联项目</dt><dd>{projectName(task.projectId)}</dd></div><div><dt>执行频率</dt><dd>{ruleText(task.rule)}</dd></div><div><dt>时区</dt><dd>{task.rule.timezone}</dd></div></dl>
        <div className="sp-time-preview"><div><Clock3 size={15} /><strong>{task.enabled ? '接下来三次' : '恢复后的预计时间'}</strong></div>{nextTimes(task.rule, previewNow, new Date(task.createdAt)).map((date, i) => <p key={date.toISOString()}><span>{i + 1}</span>{formatTime(date, task.rule.timezone)}</p>)}{!task.enabled && <small>任务已暂停，不会自动触发。手动模拟不会恢复任务。</small>}</div>
      </> : <>
        <p className="sp-helper">每次运行独立保存结果。以下内容均为模拟数据。</p>
        {!selected ? <div className="sp-empty-small">还没有运行记录。点击“模拟运行”查看效果。</div> : <>
          <div className="sp-history-list">{task.runs.map(r => <button key={r.id} className={selected.id === r.id ? 'active' : ''} onClick={() => setRunId(r.id)}><Clock3 size={15} /><span>{formatTime(r.startedAt, task.rule.timezone)}</span><small>模拟触发</small><RunStatus status={r.status} /><ChevronRight size={14} /></button>)}</div>
          <div className="sp-result"><div className="sp-result-header"><strong>{selected.target.kind === 'chat' ? '独立对话结果' : '工作流执行结果'}</strong><Badge variant="secondary">模拟结果</Badge></div><p className="sp-helper">{selected.projectName} · {selected.target.kind === 'workflow' ? `${workflowName(selected.target.workflowId)} ${selected.target.snapshot}` : selected.target.model}</p>
            {selected.target.kind === 'chat' && <div className="sp-message-user">{selected.target.prompt}</div>}
            {selected.target.kind === 'workflow' && <div className="sp-flow-steps">{['开始', '检查与整理', '输出结果'].map((name, i) => <span key={name} className={selected.status === 'failed' && i === 1 ? 'failed' : ''}>{selected.status === 'running' ? <LoaderCircle size={12} className="sp-spin" /> : selected.status === 'failed' && i >= 1 ? <CircleAlert size={12} /> : <Check size={12} />}{name}{i < 2 && <ChevronRight size={12} />}</span>)}</div>}
            <div className={`sp-message-output ${selected.status === 'failed' ? 'failed' : ''}`}>{selected.status === 'running' ? <div className="sp-running"><LoaderCircle size={20} className="sp-spin" />正在模拟执行，结果即将出现在这里…</div> : selected.output}</div>
            {selected.status === 'failed' && <Button variant="outline" size="sm" disabled={busy} onClick={() => run()}><Play size={14} />重新模拟</Button>}
          </div>
        </>}
        <details className="sp-demo-options"><summary>演示选项<ArrowUpRight size={12} /></summary><p>用于检查失败时的提示和恢复操作。</p><Button variant="outline" size="sm" disabled={busy} onClick={() => run(true)}>模拟一次失败</Button></details>
      </>}
    </div>
    <footer className="sp-detail-footer">{confirmDelete ? <><span>删除任务及模拟记录？</span><Button variant="outline" size="sm" onClick={() => setConfirmDelete(false)}>保留任务</Button><Button variant="destructive" size="sm" onClick={onDelete}>确认删除</Button></> : <><Button variant="ghost" size="sm" onClick={() => setConfirmDelete(true)}><Trash2 size={14} />删除任务</Button><span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={() => toggle(task.id)}>{task.enabled ? <Pause size={14} /> : <Play size={14} />}{task.enabled ? '暂停任务' : '恢复任务'}</Button></>}</footer>
  </div>
}
