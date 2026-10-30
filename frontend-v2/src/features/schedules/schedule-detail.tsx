import { useState } from 'react'
import { Check, Clock3, FlaskConical, ListTodo, Pause, Pencil, Play, TriangleAlert } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { catalog, type Catalog, formatTime, nextTimes, projectName, workflowName, ruleText, type Run, type SourceTask, type Task } from './schedule-model'

export function RunStatus({ run }: { run?: Run }) {
  const Icon = run?.status === 'success' ? Check : run?.status === 'failed' ? TriangleAlert : Clock3
  return <span className={`sp-run-status ${run?.status ?? 'none'}`}><Icon size={13} />{!run ? '尚未运行' : run.status === 'running' ? '进行中' : run.status === 'success' ? '成功' : '失败'}</span>
}

export function ScheduleDetail({ data = catalog, task, sources = [], initialTab = 'config', onEdit, onToggle, onSimulate }: { data?: Catalog; task: Task; sources?: SourceTask[]; initialTab?: 'config' | 'history'; onEdit: () => void; onToggle: () => void; onSimulate?: (fail?: boolean) => void }) {
  const [now] = useState(() => new Date())
  const [tab, setTab] = useState(initialTab)
  const [runId, setRunId] = useState('')
  const target = task.target
  const source = target.kind === 'task' ? sources.find(s => s.id === target.sourceTaskId) : undefined
  const sourceError = target.kind === 'task' ? (!source ? '来源任务不可用，请重新选择。' : source.unavailable) : undefined
  const history = task.runs.find(r => r.id === runId) ?? task.runs[0]
  const running = task.runs.some(r => r.status === 'running')
  const simulate = (fail = false) => { onSimulate?.(fail); setRunId(''); setTab('history') }
  return <div className="sp-detail">
    <div className="sp-detail-toolbar"><span className={`sp-enabled ${task.enabled ? '' : 'paused'}`}>{task.enabled ? '已启用' : '已暂停'}</span>{target.kind !== 'chat' && <Badge variant="outline"><FlaskConical data-icon="inline-start" />演示</Badge>}<span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={onEdit}><Pencil size={14} />编辑</Button></div>
    {target.kind === 'task' && <div className="sp-tabs" aria-label="详情视图"><button aria-pressed={tab === 'config'} onClick={() => setTab('config')}>任务配置</button><button aria-pressed={tab === 'history'} onClick={() => setTab('history')}>运行历史 <span>{task.runs.length}</span></button></div>}
    <div className="sp-detail-scroll">{target.kind === 'task' && tab === 'history' ? <>
      <p className="sp-helper">每次独立运行，当次内容保留快照；演示结果不会出现在普通对话中。</p>
      {!history ? <p className="sp-empty-small">尚无运行记录，可点击「模拟运行」查看效果。</p> : <>
        <div className="sp-history-list">{task.runs.map(r => <button key={r.id} className={history.id === r.id ? 'active' : ''} aria-pressed={history.id === r.id} onClick={() => setRunId(r.id)}><RunStatus run={r} /><span>{formatTime(r.startedAt, task.rule.timezone)}</span><small>演示</small></button>)}</div>
        <div className="sp-config-heading">当次任务内容</div><strong>{history.source?.title}</strong><p className="sp-helper">{history.source?.id} · {history.projectName}</p><p className="sp-prompt-preview">{history.source?.description}</p>
        {history.target.kind === 'task' && history.target.supplementalInstruction && <><div className="sp-config-heading">当次补充说明</div><p className="sp-prompt-preview">{history.target.supplementalInstruction}</p></>}
        <div className="sp-config-heading">运行结果（演示）</div><p className="sp-prompt-preview" role="status">{history.status === 'running' ? '正在模拟运行…' : history.output}</p>
      </>}
    </> : <>
      <div className="sp-config-heading">执行内容</div>
      {target.kind === 'task' ? <><div className="sp-target-title"><ListTodo size={17} /><strong>{source?.title ?? '来源任务不可用'}</strong><Badge variant="outline">已有任务</Badge></div><p className="sp-helper">{target.sourceTaskId} · {source?.status}</p>{source && <p className="sp-prompt-preview">{source.description}</p>}{target.supplementalInstruction && <><div className="sp-config-heading">补充说明</div><p className="sp-prompt-preview">{target.supplementalInstruction}</p></>}</> : <><strong>{target.kind === 'chat' ? '直接对话' : '工作流'}</strong>{target.kind === 'chat' ? <p className="sp-prompt-preview">{target.prompt}</p> : <><p>{workflowName(target.workflowId, data)} · {target.snapshot}</p><dl className="sp-key-values">{Object.entries(target.inputs).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value || '未填写'}</dd></div>)}</dl></>}</>}
      <div className="sp-config-heading">运行环境</div><dl className="sp-key-values"><div><dt>关联项目</dt><dd>{target.kind === 'task' ? source?.projectName ?? '来源不可用' : projectName(task.projectId)}</dd></div><div><dt>执行频率</dt><dd>{ruleText(task.rule)}</dd></div><div><dt>时区</dt><dd>{task.rule.timezone}</dd></div></dl>
      <div className="sp-time-preview"><div><Clock3 size={15} /><strong>预计时间</strong></div>{nextTimes(task.rule, now, new Date(task.createdAt)).map((date, i) => <p key={date.toISOString()}><span>{i + 1}</span>{formatTime(date, task.rule.timezone)}</p>)}</div>
      <p className="sp-helper">{target.kind === 'task' ? '前端演示，刷新后清除。每次使用最新任务描述与项目，不读取历史对话；原任务完成不会暂停此安排。' : target.kind === 'workflow' ? '工作流配置为前端演示，刷新后清除。真实执行与运行历史尚未接入。' : '配置已保存。自动执行、立即运行和运行历史尚未接入。'}</p>
    </>}
      {sourceError && <p className="sp-helper" role="alert">{sourceError} 既有历史仍可查看。</p>}
      {target.kind === 'task' && onSimulate && <details className="sp-demo-options"><summary>演示选项</summary><p>仅模拟界面状态，不调用模型或后端执行。</p><Button size="sm" variant="outline" disabled={!!sourceError || running} onClick={() => simulate(true)}>模拟失败</Button></details>}
    </div>
    <footer className="sp-detail-footer">{target.kind === 'task' && onSimulate && <Button size="sm" disabled={!!sourceError || running} onClick={() => simulate()}><Play data-icon="inline-start" />{running ? '模拟运行中…' : '模拟运行'}</Button>}<span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={onToggle}>{task.enabled ? <Pause size={14} /> : <Play size={14} />}{task.enabled ? '暂停任务' : '恢复任务'}</Button></footer>
  </div>
}
