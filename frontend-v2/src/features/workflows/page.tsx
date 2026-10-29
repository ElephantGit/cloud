import { useState } from 'react'
import { GitBranch, Plus, ArrowRight, RefreshCw } from 'lucide-react'
import { Link, useSearchParams } from 'react-router'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { useGetApiV1TenantsTidWorkflows, useGetApiV1TenantsTidWorkflowsWfid } from '@/api/workflows/workflows'
import { Button } from '@/components/ui/button'
import { Editor } from './editor'
import { ErrorNotice } from './error-notice'

export default function Page() {
  const tenants = useGetApiV1MeTenants(undefined, { query: { retry: false } })
  const [params, setParams] = useSearchParams()
  const active = tenants.data?.items.filter((t) => t.status === 'active') ?? []
  const tenant = active.find((t) => t.id === params.get('tenant')) ?? active[0]
  return (
    <div className="flex min-h-full flex-col">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b px-8 py-6">
        <div><h1 className="text-2xl font-semibold tracking-tight">工作流</h1><p className="mt-1 text-sm text-muted-foreground">从一个想法开始，保存草稿，留下每个版本。</p></div>
        {tenant && <label className="text-sm text-muted-foreground">团队
          <select aria-label="团队" className="ml-3 rounded-md border bg-background px-3 py-2 text-foreground" value={tenant.id} onChange={(e) => setParams({ tenant: e.target.value })}>
            {active.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
        </label>}
      </header>
      {tenants.isPending && <p className="p-8 text-sm text-muted-foreground">正在加载团队…</p>}
      {tenants.isError && <div className="p-8"><ErrorNotice error={tenants.error} retry={() => void tenants.refetch()} /></div>}
      {tenants.isSuccess && !tenant && <p className="p-8 text-muted-foreground">加入一个团队后即可创建工作流。</p>}
      {tenant && <Workspace key={tenant.id} tenant={tenant.id} />}
    </div>
  )
}

function Workspace({ tenant }: { tenant: string }) {
  const [params] = useSearchParams()
  const [search, setSearch] = useState('')
  const selected = params.get('workflow') ?? ''
  const after = params.get('after') ?? ''
  const list = useGetApiV1TenantsTidWorkflows(tenant, { limit: 20, ...(!search && after ? { after } : {}) }, { query: { retry: false } })
  const href = (workflow: string, cursor = after) => {
    const next = new URLSearchParams({ tenant })
    if (workflow) next.set('workflow', workflow)
    if (cursor) next.set('after', cursor)
    return `?${next}`
  }
  return (
    <div className="grid flex-1 lg:grid-cols-[300px_minmax(0,1fr)]">
      <aside className="border-b bg-muted/20 p-5 lg:border-r lg:border-b-0">
        <div className="mb-5 flex items-center justify-between">
          <span className="text-xs font-medium tracking-wider text-muted-foreground">我的工作流</span>
          <Link to={href('new')} className="flex items-center gap-1 rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground"><Plus size={14} />新建工作流</Link>
        </div>
        <input aria-label="搜索工作流" placeholder="搜索工作流" className="mb-4 w-full rounded-md border bg-background px-3 py-2 text-sm" value={search} onChange={(event) => setSearch(event.target.value)} />
        {list.isPending && <p className="text-sm text-muted-foreground">正在加载工作流…</p>}
        {list.isError && <ErrorNotice error={list.error} retry={() => void list.refetch()} />}
        {list.data?.items.length === 0 && <p className="py-10 text-center text-sm text-muted-foreground">{after ? '这一页没有工作流。' : '还没有工作流，创建第一个吧。'}</p>}
        <nav aria-label="工作流列表" className="space-y-2">
          {list.data?.items.filter((w) => w.name.toLocaleLowerCase().includes(search.toLocaleLowerCase())).map((w) => (
            <Link key={w.id} to={href(w.id)} className={`block rounded-lg border p-4 transition-colors ${w.id === selected ? 'border-foreground/30 bg-background shadow-sm' : 'border-transparent hover:bg-background'}`}>
              <span className="flex items-center gap-2 text-sm font-medium"><GitBranch size={16} className="shrink-0 text-muted-foreground" /><span className="truncate">{w.name}</span></span>
              <span className="mt-2 block truncate text-xs text-muted-foreground">{w.description || '暂无描述'}</span>
              <span className="mt-3 block text-xs text-muted-foreground">草稿 v{w.version} · {new Date(w.updatedAt).toLocaleDateString('zh-CN')}</span>
            </Link>
          ))}
          {search && list.data?.nextCursor && <SearchMore tenant={tenant} cursor={list.data.nextCursor} search={search} href={href} />}
        </nav>
        <div className="mt-4 flex justify-between text-sm">
          {!search && after && <Link to={href(selected, '')}>回到第一页</Link>}
          {!search && list.data?.nextCursor && <Link to={href(selected, list.data.nextCursor)} className="ml-auto">下一页 →</Link>}
        </div>
      </aside>
      <section className="min-w-0 p-5 md:p-8">
        {!selected ? <div className="mx-auto flex max-w-md flex-col items-center py-20 text-center">
          <div className="mb-6 rounded-2xl border bg-muted/30 p-5"><GitBranch size={32} strokeWidth={1.5} /></div>
          <h2 className="text-xl font-medium">把想法变成可复用的流程</h2>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">创建工作流，整理输入与节点。每次发布都会留下独立版本，之后可以随时查看。</p>
          <Link to={href('new')} className="mt-6 flex items-center gap-2 text-sm font-medium">创建第一个工作流 <ArrowRight size={16} /></Link>
        </div> : selected === 'new' ? <Editor key="new" tenant={tenant} /> : <Existing key={selected} tenant={tenant} id={selected} />}
      </section>
    </div>
  )
}

function SearchMore({ tenant, cursor, search, href }: { tenant: string; cursor: string; search: string; href: (workflow: string) => string }) {
  const query = useGetApiV1TenantsTidWorkflows(tenant, { limit: 20, after: cursor }, { query: { retry: false } })
  if (query.isPending) return <p className="text-xs text-muted-foreground">正在搜索更多工作流…</p>
  if (query.isError) return <ErrorNotice error={query.error} retry={() => void query.refetch()} />
  return <>{query.data?.items.filter((item) => item.name.toLocaleLowerCase().includes(search.toLocaleLowerCase())).map((item) => <Link key={item.id} to={href(item.id)} className="block rounded-lg border border-transparent p-4 text-sm hover:bg-background">{item.name}</Link>)}{query.data?.nextCursor && <SearchMore tenant={tenant} cursor={query.data.nextCursor} search={search} href={href} />}</>
}

function Existing({ tenant, id }: { tenant: string; id: string }) {
  const query = useGetApiV1TenantsTidWorkflowsWfid(tenant, id, { query: { retry: false, refetchOnWindowFocus: false } })
  if (query.isPending) return <p className="text-sm text-muted-foreground">正在打开草稿…</p>
  if (!query.data) return <ErrorNotice error={query.error} retry={() => void query.refetch()} />
  return <>{query.isError && <ErrorNotice error={query.error} retry={() => void query.refetch()} />}<Editor key={id} tenant={tenant} workflow={query.data} />
    <Button variant="ghost" className="mt-6 text-muted-foreground" onClick={() => void query.refetch()}><RefreshCw size={14} />检查最新版本</Button>
  </>
}
