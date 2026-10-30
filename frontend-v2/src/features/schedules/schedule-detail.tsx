import { useState } from 'react'
import { Clock3, Pause, Pencil, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { formatTime, nextTimes, ruleText, type Task } from './schedule-model'

export function RunStatus() { return <span className="sp-run-status none"><Clock3 size={13} />尚未运行</span> }

export function ScheduleDetail({ task, onEdit, onToggle }: { task: Task; onEdit: () => void; onToggle: () => void }) {
  const [now] = useState(() => new Date())
  return <div className="sp-detail">
    <div className="sp-detail-toolbar"><span className={`sp-enabled ${task.enabled ? '' : 'paused'}`}>{task.enabled ? '已启用' : '已暂停'}</span><span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={onEdit}><Pencil size={14} />编辑</Button></div>
    <div className="sp-detail-scroll"><div className="sp-config-heading">执行内容</div><strong>{task.target.kind === 'chat' ? '直接对话' : '工作流'}</strong>{task.target.kind === 'chat' && <p className="sp-prompt-preview">{task.target.prompt}</p>}
      <div className="sp-config-heading">运行环境</div><dl className="sp-key-values"><div><dt>关联项目</dt><dd>{task.projectId || '不关联项目'}</dd></div><div><dt>执行频率</dt><dd>{ruleText(task.rule)}</dd></div><div><dt>时区</dt><dd>{task.rule.timezone}</dd></div></dl>
      <div className="sp-time-preview"><div><Clock3 size={15} /><strong>预计时间</strong></div>{nextTimes(task.rule, now, new Date(task.createdAt)).map((date, i) => <p key={date.toISOString()}><span>{i + 1}</span>{formatTime(date, task.rule.timezone)}</p>)}</div>
      <p className="sp-helper">配置已保存。自动执行、立即运行和运行历史尚未接入。</p>
    </div><footer className="sp-detail-footer"><span className="sp-footer-spacer" /><Button variant="outline" size="sm" onClick={onToggle}>{task.enabled ? <Pause size={14} /> : <Play size={14} />}{task.enabled ? '暂停任务' : '恢复任务'}</Button></footer>
  </div>
}
