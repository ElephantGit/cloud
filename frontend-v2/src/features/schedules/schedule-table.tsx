import { ChevronRight, Folder, GitBranch, ListTodo, MessageSquare } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { formatTime, nextTimes, projectName, ruleText, workflowName, type SourceTask, type Task } from './schedule-model'
import { RunStatus } from './schedule-detail'

export function ScheduleTable({ tasks, sources = [], now, onOpen, onToggle }: { tasks: Task[]; sources?: SourceTask[]; now: Date; onOpen: (task: Task, tab?: 'config' | 'history') => void; onToggle: (task: Task) => void }) {
  return <div className="sp-table-surface"><Table className="sp-table">
    <TableHeader><TableRow><TableHead>任务名称</TableHead><TableHead>执行方式</TableHead><TableHead>关联项目</TableHead><TableHead>时间安排</TableHead><TableHead>最近运行</TableHead><TableHead>启用</TableHead><TableHead><span className="sp-sr-only">详情</span></TableHead></TableRow></TableHeader>
    <TableBody>{tasks.map(task => {
      const due = task.enabled ? nextTimes(task.rule, now, new Date(task.createdAt))[0] : undefined
      const latest = task.runs[0]
      const target = task.target
      const source = target.kind === 'task' ? sources.find(s => s.id === target.sourceTaskId) : undefined
      const Icon = target.kind === 'chat' ? MessageSquare : target.kind === 'task' ? ListTodo : GitBranch
      const label = target.kind === 'chat' ? '直接对话' : target.kind === 'task' ? '已有任务' : '工作流'
      return <TableRow key={task.id} className="sp-task-row" data-paused={!task.enabled}>
        <TableCell><button className="sp-task-cell" onClick={() => onOpen(task)}>
          <span className={cn('sp-task-icon', target.kind !== 'chat' && 'sp-task-icon-workflow')}><Icon size={17} /></span>
          <span><strong>{task.name}</strong><small>{target.kind === 'chat' ? '每次创建独立对话' : target.kind === 'task' ? `${source?.title ?? '来源任务不可用'} · 演示` : `${workflowName(target.workflowId)} · ${target.snapshot} · 演示`}</small></span>
        </button></TableCell>
        <TableCell><Badge variant="outline"><Icon data-icon="inline-start" />{label}</Badge></TableCell>
        <TableCell><span className={cn('sp-project-label', !task.projectId && 'muted')}>{task.projectId && <Folder size={14} />}{target.kind === 'task' ? source?.projectName ?? '来源不可用' : projectName(task.projectId)}</span></TableCell>
        <TableCell><div className="sp-cell-stack"><span>{ruleText(task.rule)}</span><small>{due ? `预计 ${formatTime(due, task.rule.timezone)}` : '已暂停自动运行'}</small></div></TableCell>
        <TableCell><button className="sp-run-cell" onClick={() => onOpen(task, 'history')}><RunStatus run={latest} /><small>{latest ? formatTime(latest.startedAt, task.rule.timezone) : target.kind === 'task' ? '前端演示' : '执行尚未接入'}</small></button></TableCell>
        <TableCell><Switch checked={task.enabled} onCheckedChange={() => onToggle(task)} aria-label={`${task.name}启用状态`} /></TableCell>
        <TableCell><Button variant="ghost" size="icon-sm" aria-label={`查看${task.name}`} onClick={() => onOpen(task)}><ChevronRight /></Button></TableCell>
      </TableRow>
    })}</TableBody>
  </Table></div>
}
