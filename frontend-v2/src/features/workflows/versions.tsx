import { useState } from 'react'
import { createStore } from 'zustand/vanilla'
import { useStore } from 'zustand'
import { History, Download } from 'lucide-react'
import { useGetApiV1TenantsTidWorkflowsWfidSnapshots, useGetApiV1TenantsTidWorkflowsWfidSnapshotsSnapshotId, usePutApiV1TenantsTidWorkflowsWfidSnapshotsSnapshotIdRestore } from '@/api/workflows/workflows'
import type { Workflow } from '@/api/generated.schemas'
import { Button } from '@/components/ui/button'
import { downloadGraph } from './shared'
import { ErrorNotice } from './error-notice'
import { WorkflowCanvas } from './workflow-canvas'
import { WorkflowNodeInspector } from './workflow-node-inspector'

export function Versions({ tenant, workflow, version, dirty, onRestore, onRestoringChange }: { tenant: string; workflow: string; version: number; dirty: boolean; onRestore: (workflow: Workflow) => void; onRestoringChange: (value: boolean) => void }) {
  const [store] = useState(() => createStore(() => ({ after: '', selected: '' })))
  const state = useStore(store)
  const query = useGetApiV1TenantsTidWorkflowsWfidSnapshots(tenant, workflow, { limit: 10, ...(state.after ? { after: state.after } : {}) }, { query: { retry: false } })
  return <section className="rounded-xl border p-5" aria-label="已发布版本">
    <h3 className="mb-4 flex items-center gap-2 text-sm font-semibold"><History size={16} />已发布版本</h3>
    {query.isPending && <p className="text-sm text-muted-foreground">正在加载版本…</p>}
    {query.isError && <ErrorNotice error={query.error} retry={() => void query.refetch()} />}
    {query.data?.items.length === 0 && <p className="text-sm text-muted-foreground">还没有发布版本。保存草稿后即可发布。</p>}
    <div className="space-y-2">{query.data?.items.map((s) => <button key={s.id} type="button" className="flex w-full flex-wrap items-center justify-between gap-2 rounded-lg bg-muted/40 p-3 text-left text-sm hover:bg-muted" onClick={() => store.setState({ selected: state.selected === s.id ? '' : s.id })} aria-expanded={state.selected === s.id}>
      <span><span className="mr-3 font-mono text-xs">v{s.version}</span>{s.name}</span><span className="text-xs text-muted-foreground">{new Date(s.createdAt).toLocaleString('zh-CN')}</span>
    </button>)}</div>
    <div className="mt-3 flex gap-3">
      {state.after && <Button size="sm" variant="ghost" onClick={() => store.setState({ after: '' })}>回到首批版本</Button>}
      {query.data?.nextCursor && <Button size="sm" variant="ghost" onClick={() => store.setState({ after: query.data!.nextCursor })}>下一批版本</Button>}
    </div>
    {state.selected && <Snapshot key={state.selected} tenant={tenant} workflow={workflow} id={state.selected} version={version} dirty={dirty} onRestore={onRestore} onRestoringChange={onRestoringChange} />}
  </section>
}

function Snapshot({ tenant, workflow, id, version, dirty, onRestore, onRestoringChange }: { tenant: string; workflow: string; id: string; version: number; dirty: boolean; onRestore: (workflow: Workflow) => void; onRestoringChange: (value: boolean) => void }) {
  const restore = usePutApiV1TenantsTidWorkflowsWfidSnapshotsSnapshotIdRestore()
  const [confirm, setConfirm] = useState(false)
  const query = useGetApiV1TenantsTidWorkflowsWfidSnapshotsSnapshotId(tenant, workflow, id, { query: { retry: false } })
  if (query.isPending) return <p className="mt-4 text-sm text-muted-foreground">正在读取版本…</p>
  if (query.isError) return <ErrorNotice error={query.error} retry={() => void query.refetch()} />
  const graph = JSON.stringify(query.data.graph, null, 2)
  return <div className="mt-4 border-t pt-4">
    <div className="mb-3 flex items-center justify-between"><h4 className="text-sm font-medium">版本 v{query.data.version} · 只读</h4><Button variant="ghost" size="sm" onClick={() => downloadGraph(`${query.data.name}-v${query.data.version}`, graph)}><Download size={14} />导出此版本</Button></div>
    {restore.isError && <ErrorNotice error={restore.error} />}
    {!confirm ? <Button size="sm" variant="outline" disabled={dirty || restore.isPending} onClick={() => setConfirm(true)}>恢复为草稿</Button> : <div role="alertdialog" aria-label="确认恢复版本" className="my-3 rounded border p-3 text-sm"><p>恢复会替换当前草稿内容，已发布版本不变。</p><Button size="sm" disabled={dirty || restore.isPending} onClick={() => { onRestoringChange(true); void restore.mutateAsync({ tid: tenant, wfid: workflow, snapshotId: id, data: { version } }).then((result) => { onRestore(result); setConfirm(false) }).catch(() => {}).finally(() => onRestoringChange(false)) }}>确认恢复</Button><Button size="sm" variant="ghost" onClick={() => setConfirm(false)}>取消</Button></div>}
    {dirty && <p className="text-xs text-muted-foreground">保存当前修改后才能恢复版本。</p>}
    <WorkflowCanvas graph={graph} onChange={() => {}} readOnly renderInspector={(props) => <WorkflowNodeInspector {...props} globalVariables={query.data.graph.globalVariables} readOnly />} />
    <details><summary>版本 JSON</summary><pre aria-label="版本内容" className="max-h-80 overflow-auto rounded-lg bg-muted/50 p-4 text-xs leading-5">{graph}</pre></details>
  </div>
}
