import { useQuery } from '@tanstack/react-query'
import { Bot, Users, TrendingUp, RefreshCw, ArrowUpRight } from 'lucide-react'
import { Link } from 'react-router'
import { Button } from '@/components/ui/button'
import { statusOf } from '@/lib/api-client'
import { Badge } from '@/components/ui/badge'
import { getTaskTimeline, listTasks } from '@/lib/tasks-api/generated'
import { useAnalysisScope } from './analysis/queries'
import { overviewModel, type OverviewSnapshot } from './overview-model'

/** Read the same authorized workspace resources as the task board, including every task timeline. */
async function readOverview(tenant: string, signal?: AbortSignal): Promise<OverviewSnapshot> {
  const list = await listTasks(tenant, undefined, signal)
  if (list.nextCursor) throw new Error('Incomplete task list')
  const timelines: OverviewSnapshot['timelines'] = {}
  // Bound concurrent reads while this preview uses the existing per-task timeline contract.
  for (let index = 0; index < list.items.length; index += 4) {
    const batch = await Promise.all(list.items.slice(index, index + 4).map(async task => [task.id, await getTaskTimeline(tenant, task.id, undefined, signal)] as const))
    for (const [id, timeline] of batch) timelines[id] = timeline
  }
  return { tasks: list.items, timelines, fetchedAt: new Date().toISOString() }
}

export default function RealOverview() {
  const scope = useAnalysisScope()
  const query = useQuery({
    queryKey: ['real-team-overview', scope.user, scope.tenant],
    enabled: !!scope.tenant,
    queryFn: ({ signal }) => readOverview(scope.tenant, signal),
    retry: false,
    refetchInterval: 30000,
  })
  const refresh = <Button variant='outline' disabled={query.isFetching || !scope.tenant} onClick={() => void query.refetch()}><RefreshCw size={14} />刷新概览</Button>
  if (scope.error || query.isError) return <div className='db-panel p-6'><h2 className='db-section-title'>团队概览</h2><p role='alert'>{scope.error ? '读取工作区失败，请刷新重试。' : [401, 403, 404].includes(statusOf(query.error) ?? 0) ? '团队数据不可访问，请确认登录与工作区权限。' : '团队统计暂不可用，请刷新重试。'}</p>{refresh}</div>
  if (!scope.pending && !scope.tenant) return <div className='db-panel p-6'><h2 className='db-section-title'>团队概览</h2><p>暂无可访问的工作区。</p></div>
  if (scope.pending || query.isPending || !query.data) return <p role='status'>正在读取团队任务与运行记录…</p>
  const snapshot = query.data
  const model = overviewModel(snapshot)
  const stats = [
    { label: '进行中', value: model.counts.running, note: `${model.counts.pending} 项待处理` },
    { label: '待验收', value: model.counts.review, note: '等待人工检查与确认' },
    { label: '已完成', value: model.counts.done, note: '当前处于已完成状态的任务' },
    { label: '本周任务 Agent 运行', value: model.weekRuns.length, note: `${model.weekRuns.filter(run => run.lastResult === 'completed').length} 次最新一轮执行成功` },
  ]
  return <>
    <header className='db-page-header'>
      <div><h2 className='db-section-title'>团队概览</h2><p>当前工作区 · {snapshot.tasks.length} 项任务 · 更新于 {new Date(snapshot.fetchedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</p></div>
      <div className='flex items-center gap-3'><Badge variant='outline'>真实数据</Badge>{refresh}</div>
    </header>
    <section className='db-stats' aria-label='任务概览'>
      {stats.map(stat => <article className='db-stat' key={stat.label}><span className='db-stat-label'>{stat.label}</span><strong className='db-stat-value'>{stat.value}</strong><span className='db-stat-note'>{stat.note}</span></article>)}
    </section>
    {snapshot.tasks.length === 0 && <div className='db-panel p-6 text-center'><p>还没有团队任务。</p><Link to='/collaboration' className='underline'>前往协作创建任务</Link></div>}
    <div className='db-panels'>
      <section className='db-panel' aria-label='团队进度'>
        <header className='db-panel-head'><h2><Users size={16} />团队进度</h2><span>按当前负责人汇总；完成率不含已取消任务</span></header>
        <ul className='db-members'>
          {model.owners.map(owner => {
            const total = owner.pending + owner.running + owner.review + owner.done
            const rate = total ? Math.round(owner.done / total * 100) : 0
            return <li className={`db-member ${owner.kind}`} key={owner.id}>
              <span className='db-member-name'>{owner.kind === 'agent' ? <Bot size={15} /> : <Users size={15} />}<b>{owner.name}</b><em>{({ human: '成员', agent: 'Agent', squad: '小队', unassigned: '' } as Record<string, string>)[owner.kind]}</em></span>
              <span className='db-member-counts'><span>待处理 {owner.pending}</span><span>进行中 {owner.running}</span><span>待验收 {owner.review}</span><span>已完成 {owner.done}</span></span>
              <span className='db-member-rate'><span className='db-bar' role='progressbar' aria-label={`${owner.name} 完成率`} aria-valuenow={rate} aria-valuemin={0} aria-valuemax={100}><i style={{ width: `${rate}%` }} /></span><b>{rate}%</b></span>
            </li>
          })}
        </ul>
        {!model.owners.length && <p className='p-6 text-sm text-muted-foreground'>暂无待统计的任务负责人。</p>}
      </section>
      <section className='db-panel' aria-label='Agent 产出'>
        <header className='db-panel-head'><h2><Bot size={16} />Agent 产出</h2><span>执行成功且处于待验收或已完成的任务</span></header>
        <ul className='db-outputs'>{model.outputs.slice(0, 6).map(({ task, run }) => <li className='db-output' key={task.id}>
          <span className='db-output-head'><Link to={`/collaboration/tasks/${encodeURIComponent(task.id)}`} className='flex min-w-0 items-center gap-1 font-medium hover:underline'>{task.title}<ArrowUpRight size={14} className='shrink-0' /></Link><Badge variant={task.status === 'review' ? 'secondary' : 'outline'}>{task.status === 'review' ? '待验收' : '已完成'}</Badge></span>
          <span className='db-output-summary line-clamp-2'>任务说明：{task.description || '暂无描述，打开任务查看执行记录。'}</span>
          <span className='db-output-meta'>{run.executorName} · 任务更新 {new Date(task.updatedAt).toLocaleString()}</span>
        </li>)}</ul>
        {!model.outputs.length && <p className='p-6 text-sm text-muted-foreground'>暂无符合条件的 Agent 交付任务。</p>}
      </section>
    </div>
    <section className='db-panel db-trend' aria-label='近7天任务更新趋势'>
      <header className='db-panel-head'><h2><TrendingUp size={16} />近 7 天任务更新趋势</h2><span>按最后更新日期计数，包含编辑与状态变化</span></header>
      <ol className='db-bars' style={{ gridTemplateColumns: 'repeat(7, minmax(0, 1fr))' }}>{model.days.map(day => <li key={day.label}><span className='db-bar-track' aria-hidden='true'><i style={{ height: `${day.count / model.max * 100}%` }} /></span><b>{day.count}</b><small>{day.label}</small></li>)}</ol>
    </section>
  </>
}
