import { ChevronRight, Folder, GitBranch, MessageSquare } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { formatTime, nextTimes, projectName, ruleText, workflowName, type Task } from './schedule-model'
import { RunStatus } from './schedule-detail'

export function ScheduleTable({ tasks, now, onOpen, onToggle }: { tasks: Task[]; now: Date; onOpen: (task: Task, tab?: 'config' | 'history') => void; onToggle: (task: Task) => void }) {
  return <div className="sp-table-surface"><Table className="sp-table">
    <TableHeader><TableRow><TableHead>任务名称</TableHead><TableHead>执行方式</TableHead><TableHead>关联项目</TableHead><TableHead>时间安排</TableHead><TableHead>最近运行</TableHead><TableHead>启用</TableHead><TableHead><span className="sp-sr-only">详情</span></TableHead></TableRow></TableHeader>
    <TableBody>{tasks.map(task => {
      const due = task.enabled ? nextTimes(task.rule, now, new Date(task.createdAt))[0] : undefined
      const latest = task.runs[0]
      return <TableRow key={task.id} className="sp-task-row" data-paused={!task.enabled}>
        <TableCell><button className="sp-task-cell" onClick={() => onOpen(task)}>
          <span className={cn('sp-task-icon', task.target.kind === 'workflow' && 'sp-task-icon-workflow')}>{task.target.kind === 'chat' ? <MessageSquare size={17} /> : <GitBranch size={17} />}</span>
          <span><strong>{task.name}</strong><small>{task.target.kind === 'chat' ? '每次创建独立对话' : `${workflowName(task.target.workflowId)} · ${task.target.snapshot}`}</small></span>
        </button></TableCell>
        <TableCell><Badge variant="outline">{task.target.kind === 'chat' ? <MessageSquare data-icon="inline-start" /> : <GitBranch data-icon="inline-start" />}{task.target.kind === 'chat' ? '直接对话' : '工作流'}</Badge></TableCell>
        <TableCell><span className={cn('sp-project-label', !task.projectId && 'muted')}>{task.projectId && <Folder size={14} />}{projectName(task.projectId)}</span></TableCell>
        <TableCell><div className="sp-cell-stack"><span>{ruleText(task.rule)}</span><small>{due ? `预计 ${formatTime(due, task.rule.timezone)}` : '已暂停自动运行'}</small></div></TableCell>
        <TableCell><button className="sp-run-cell" onClick={() => onOpen(task, 'history')}><RunStatus /><small>{latest ? formatTime(latest.startedAt, task.rule.timezone) : '执行尚未接入'}</small></button></TableCell>
        <TableCell><Switch checked={task.enabled} onCheckedChange={() => onToggle(task)} aria-label={`${task.name}启用状态`} /></TableCell>
        <TableCell><Button variant="ghost" size="icon-sm" aria-label={`查看${task.name}`} onClick={() => onOpen(task)}><ChevronRight /></Button></TableCell>
      </TableRow>
    })}</TableBody>
  </Table></div>
}
