import { type FormEvent, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getApiV2TenantsTidProjects } from '@/lib/projects-api/generated'
import { listDashboardSessions } from '../api/generated'
import { failureMessage, sessionSource, sessionStatus, useAnalysisScope } from './queries'

/** Real persisted Agent sessions, independent of the team's demo mock switch. */
export default function SessionList() {
  const scope = useAnalysisScope()
  const [params, setParams] = useSearchParams()
  const filters = Object.fromEntries(
    ['project', 'agent', 'source', 'status', 'from', 'to'].flatMap((key) =>
      params.get(key) ? [[key, params.get(key)!]] : []
    ),
  )
  const [validation, setValidation] = useState('')
  const sessions = useInfiniteQuery({
    queryKey: ['dashboard-sessions', scope.user, scope.tenant, filters],
    enabled: !!scope.tenant,
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      listDashboardSessions(
        scope.tenant,
        { ...filters, ...(pageParam ? { after: pageParam } : {}) },
        undefined,
        signal,
      ),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
  })
  const projects = useQuery({
    queryKey: ['dashboard-projects', scope.user, scope.tenant],
    enabled: !!scope.tenant,
    queryFn: ({ signal }) => getApiV2TenantsTidProjects(scope.tenant, undefined, undefined, signal),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget), next = new URLSearchParams()
    for (const key of ['project', 'agent', 'source', 'status', 'from', 'to']) {
      let value = String(form.get(key) ?? '').trim()
      if (value && (key === 'from' || key === 'to')) {
        const date = new Date(value)
        if (!Number.isFinite(date.getTime())) {
          setValidation('请输入有效的本地日期和时间。')
          return
        }
        value = date.toISOString()
      }
      if (value) next.set(key, value)
    }
    if (next.get('from') && next.get('to') && next.get('from')! >= next.get('to')!) {
      setValidation('结束时间必须晚于开始时间。')
      return
    }
    setValidation('')
    setParams(next)
  }
  function local(value: string | undefined) {
    if (!value) return ''
    const date = new Date(value)
    if (!Number.isFinite(date.getTime())) return ''
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
  }
  return (
    <main className='space-y-5 p-4 md:p-6'>
      <header className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h1 className='text-xl font-semibold'>会话分析</h1>
          <p className='mt-1 text-sm text-muted-foreground'>
            查看自己的普通对话和协作任务的运行记录、时间轴与模型请求。
          </p>
        </div>
        <Button
          variant='outline'
          disabled={sessions.isFetching}
          onClick={() => void sessions.refetch()}
        >
          刷新会话
        </Button>
      </header>
      <form
        key={params.toString()}
        onSubmit={submit}
        className='flex flex-wrap items-end gap-3 rounded-xl border p-4'
      >
        <label className='text-sm'>
          来源<select
            aria-label='来源'
            name='source'
            defaultValue={filters.source ?? ''}
            className='ml-2 rounded border p-2'
          >
            <option value=''>全部来源</option>
            {Object.entries(sessionSource).map(([value, label]) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </label>
        <label className='text-sm'>
          项目<select
            aria-label='项目'
            name='project'
            defaultValue={filters.project ?? ''}
            className='ml-2 rounded border p-2'
          >
            <option value=''>全部项目</option>
            {projects.data?.items.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </select>
        </label>
        <label className='text-sm'>
          Agent<Input
            aria-label='Agent 标识'
            name='agent'
            defaultValue={filters.agent}
            placeholder='例如 official/ora-space.opencode'
            className='w-80'
          />
        </label>
        <label className='text-sm'>
          状态<select
            aria-label='状态'
            name='status'
            defaultValue={filters.status ?? ''}
            className='ml-2 rounded border p-2'
          >
            <option value=''>全部状态</option>
            {Object.entries(sessionStatus).map(([value, label]) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </label>
        <label className='text-sm'>
          开始时间（本地，含）<Input
            aria-label='开始时间'
            type='datetime-local'
            name='from'
            defaultValue={local(filters.from)}
          />
        </label>
        <label className='text-sm'>
          结束时间（本地，不含）<Input
            aria-label='结束时间'
            type='datetime-local'
            name='to'
            defaultValue={local(filters.to)}
          />
        </label>
        <Button type='submit'>应用筛选</Button>
        <Button type='button' variant='ghost' onClick={() => setParams({})}>清除筛选</Button>
      </form>
      {validation && <p role='alert'>{validation}</p>}
      {(scope.error || sessions.isError)
        ? <p role='alert'>{failureMessage(sessions.error)}</p>
        : scope.pending || sessions.isPending
        ? <p role='status'>正在读取会话…</p>
        : (
          <>
            <ul aria-label='Agent 会话' className='divide-y rounded-xl border'>
              {sessions.data?.pages.flatMap((page) => page.items).map((session) => (
                <li
                  key={session.id}
                  className='flex flex-wrap items-center justify-between gap-3 p-4'
                >
                  <div>
                    <Link
                      className='font-medium underline-offset-4 hover:underline'
                      to={`/dashboard/sessions/${encodeURIComponent(session.id)}`}
                    >
                      {session.title || session.id}
                    </Link>
                    <p className='mt-1 text-xs text-muted-foreground'>
                      {session.projectName || '未绑定项目'} · {session.agent} · {session.model} ·
                      {' '}
                      {sessionSource[session.source] ?? session.source}
                    </p>
                  </div>
                  <div className='text-right text-sm'>
                    {sessionStatus[session.status] ?? session.status}
                    <p className='text-xs text-muted-foreground'>
                      {new Date(session.createdAt).toLocaleString()}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
            {sessions.data?.pages[0]?.items.length === 0 && <p>暂无符合条件的会话。</p>}
            {sessions.hasNextPage && (
              <Button
                variant='outline'
                disabled={sessions.isFetchingNextPage}
                onClick={() => void sessions.fetchNextPage()}
              >
                加载更多会话
              </Button>
            )}
          </>
        )}
    </main>
  )
}
