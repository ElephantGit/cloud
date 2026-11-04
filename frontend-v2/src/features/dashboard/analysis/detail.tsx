import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'
import { Button } from '@/components/ui/button'
import {
  getDashboardHistory,
  getDashboardSession,
  getDashboardStatistics,
  listDashboardRequests,
} from '../api/generated'
import { adaptArchive } from './data'
import { SessionRecords, SessionTimeline, ToolSummary } from './timeline'
import { SessionMetrics } from './metrics'
import { accessFailure, failureMessage, sessionStatus, useAnalysisScope } from './queries'

const tabs = ['概览', '模型请求', '时间轴', '工具统计', '完整事件'] as const

export default function SessionDetail() {
  const { id = '' } = useParams()
  return <SessionDetailBody key={id} id={id} />
}

function SessionDetailBody({ id }: { id: string }) {
  const scope = useAnalysisScope()
  const [tab, setTab] = useState<typeof tabs[number]>('概览')
  const [cursors, setCursors] = useState([''])
  const enabled = !!scope.tenant && !!id
  const key = ['dashboard-detail', scope.user, scope.tenant, id]
  const metadata = useQuery({
    queryKey: [...key, 'metadata'],
    enabled,
    retry: false,
    queryFn: ({ signal }) => getDashboardSession(scope.tenant, id, undefined, signal),
  })
  const history = useQuery({
    queryKey: [...key, 'history'],
    enabled: enabled && metadata.isSuccess,
    retry: false,
    queryFn: ({ signal }) => getDashboardHistory(scope.tenant, id, undefined, signal),
  })
  const statistics = useQuery({
    queryKey: [...key, 'statistics'],
    enabled: enabled && metadata.isSuccess && tab === '模型请求',
    retry: false,
    queryFn: ({ signal }) => getDashboardStatistics(scope.tenant, id, undefined, signal),
  })
  const calls = useQuery({
    queryKey: [...key, 'calls', cursors.at(-1)],
    enabled: enabled && metadata.isSuccess && tab === '模型请求',
    retry: false,
    queryFn: ({ signal }) =>
      listDashboardRequests(
        scope.tenant,
        id,
        { after: cursors.at(-1) || undefined },
        undefined,
        signal,
      ),
  })
  const model = useMemo(
    () =>
      history.data
        ? adaptArchive({
          runId: id,
          records: history.data.records,
          frozenModel: { id: metadata.data?.model },
        })
        : null,
    [id, history.data, metadata.data?.model],
  )
  const queries = [metadata, history, statistics, calls]
  const denied = queries.some((query) => accessFailure(query.error))
  const busy = queries.some((query) => query.isFetching)
  async function refresh() {
    const read = await metadata.refetch()
    if (read.isError) return
    await Promise.all([
      history.refetch(),
      ...(statistics.isFetched ? [statistics.refetch(), calls.refetch()] : []),
    ])
  }
  if (scope.error || denied || metadata.isError) {
    return (
      <main className='p-6'>
        <Link to='/dashboard/sessions'>返回会话分析</Link>
        <p role='alert'>
          {failureMessage(metadata.error ?? queries.find((q) => accessFailure(q.error))?.error)}
        </p>
        <Button onClick={() => void refresh()}>刷新</Button>
      </main>
    )
  }
  return (
    <main className='space-y-5 p-4 md:p-6'>
      <header className='flex flex-wrap justify-between gap-3'>
        <div>
          <Link className='text-sm text-muted-foreground underline' to='/dashboard/sessions'>
            会话分析
          </Link>
          <h1 className='mt-2 text-xl font-semibold'>{metadata.data?.title || 'Agent 会话'}</h1>
        </div>
        <div className='flex items-center gap-3'>
          {metadata.data?.href && (
            <Link className='text-sm underline' to={metadata.data.href}>返回来源</Link>
          )}
          <Button variant='outline' disabled={busy} onClick={() => void refresh()}>刷新</Button>
        </div>
      </header>
      <p className='text-xs text-muted-foreground'>
        来源：任务执行时保存的记录。{history.data?.incomplete
          ? '过程观测不完整。'
          : '显示已定型的事件。'}
      </p>
      <nav className='flex flex-wrap gap-1' aria-label='会话详情'>
        {tabs.map((value) => (
          <Button
            key={value}
            variant={tab === value ? 'secondary' : 'ghost'}
            aria-pressed={tab === value}
            onClick={() => setTab(value)}
          >
            {value}
          </Button>
        ))}
      </nav>
      {tab === '模型请求'
        ? statistics.isError || calls.isError
          ? <p role='alert'>{failureMessage(statistics.error ?? calls.error)}</p>
          : statistics.data && calls.data
          ? (
            <SessionMetrics
              statistics={statistics.data}
              attempts={calls.data.items}
              pageIndex={cursors.length - 1}
              pageSize={25}
              hasNextPage={!!calls.data.nextCursor}
              isFetching={busy}
              onPrevious={() => setCursors((old) => old.slice(0, Math.max(1, old.length - 1)))}
              onNext={() => {
                if (calls.data?.nextCursor && !busy) {
                  setCursors(
                    (old) => [...old, calls.data!.nextCursor],
                  )
                }
              }}
            />
          )
          : <p role='status'>正在读取模型请求…</p>
        : history.isError
        ? <p role='alert'>历史读取失败；仍可独立查看模型请求。请刷新重试。</p>
        : !model || metadata.isPending
        ? <p role='status'>正在读取会话…</p>
        : (
          <>
            {tab === '概览' && (
              <section aria-label='会话概览' className='space-y-4 rounded-xl border p-5'>
                <h2 className='font-medium'>
                  {sessionStatus[metadata.data?.status ?? ''] ?? '未知状态'}
                </h2>
                <p className='text-sm'>
                  {metadata.data?.agent} · 冻结模型 {metadata.data?.model} ·{' '}
                  {metadata.data?.projectName}
                </p>
                <dl className='grid grid-cols-2 gap-3 text-sm'>
                  <dt>已加载原始记录</dt>
                  <dd>{model.stats.rawRecordCount}</dd>
                  <dt>重放事件</dt>
                  <dd>{model.stats.recordCount}</dd>
                  <dt>工具记录</dt>
                  <dd>{model.stats.toolCount}</dd>
                  <dt>思考记录</dt>
                  <dd>{model.stats.thoughtCount}</dd>
                </dl>
                <p className='text-xs text-muted-foreground'>
                  事件条数不代表模型 HTTP
                  请求数；Token、耗时与覆盖率见「模型请求」。未采集不代表零消耗。
                </p>
              </section>
            )}
            {tab === '时间轴' && <SessionTimeline model={model} />}
            {tab === '工具统计' && <ToolSummary model={model} />}
            {tab === '完整事件' && <SessionRecords model={model} />}
          </>
        )}
    </main>
  )
}
