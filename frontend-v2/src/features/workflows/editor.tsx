import { useEffect, useState } from 'react'
import { useBlocker, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useStore } from 'zustand'
import { Save, Upload, Download, Archive, GitBranch } from 'lucide-react'
import type { Workflow } from '@/api/generated.schemas'
import { usePostApiV1TenantsTidWorkflows, usePutApiV1TenantsTidWorkflowsWfid, usePostApiV1TenantsTidWorkflowsWfidPublish, useDeleteApiV1TenantsTidWorkflowsWfid } from '@/api/workflows/workflows'
import { Button } from '@/components/ui/button'
import { createDraftStore, parseGraph, savedFields } from './draft'
import { downloadGraph, fieldClass } from './shared'
import { ErrorNotice } from './error-notice'
import { Versions } from './versions'

export function Editor({ tenant, workflow }: { tenant: string; workflow?: Workflow }) {
  const [form] = useState(() => createDraftStore(workflow))
  const state = useStore(form)
  const [, setParams] = useSearchParams()
  const client = useQueryClient()
  const create = usePostApiV1TenantsTidWorkflows()
  const update = usePutApiV1TenantsTidWorkflowsWfid()
  const publish = usePostApiV1TenantsTidWorkflowsWfidPublish()
  const archive = useDeleteApiV1TenantsTidWorkflowsWfid()
  const busy = create.isPending || update.isPending || publish.isPending || archive.isPending
  const blocker = useBlocker(({ currentLocation, nextLocation }) => form.getState().dirty &&
    (currentLocation.pathname !== nextLocation.pathname || currentLocation.search !== nextLocation.search))
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => { if (form.getState().dirty) event.preventDefault() }
    window.addEventListener('beforeunload', guard)
    return () => window.removeEventListener('beforeunload', guard)
  }, [form])
  const change = (key: 'name' | 'description' | 'graph', value: string) => form.setState({ [key]: value, dirty: true, message: '', error: null })
  const refresh = () => client.invalidateQueries({ predicate: (q) => typeof q.queryKey[0] === 'string' && q.queryKey[0].startsWith(`/api/v1/tenants/${tenant}/workflows`) })
  async function save() {
    form.setState({ error: null, message: '' })
    try {
      const data = { name: state.name.trim(), description: state.description, graph: parseGraph(state.graph), version: state.version }
      const result = workflow
        ? await update.mutateAsync({ tid: tenant, wfid: workflow.id, data })
        : (await create.mutateAsync({ tid: tenant, data: { name: data.name, description: data.description, graph: data.graph } })).resource
      form.setState({ ...savedFields(result), dirty: false, message: '草稿已保存。' })
      await refresh()
      if (!workflow) setParams({ tenant, workflow: result.id })
    } catch (error) { form.setState({ error }) }
  }
  async function publishVersion() {
    if (!workflow || state.dirty) return
    form.setState({ error: null, message: '' })
    try {
      const result = await publish.mutateAsync({ tid: tenant, wfid: workflow.id, data: {} })
      form.setState({ message: `版本 v${result.resource.version} 已发布，内容已冻结。` })
      await refresh()
    } catch (error) { form.setState({ error }) }
  }
  async function archiveWorkflow() {
    if (!workflow) return
    try {
      await archive.mutateAsync({ tid: tenant, wfid: workflow.id, data: { version: state.version } })
      form.setState({ dirty: false })
      // Leave the archived resource before refreshing; its next GET correctly returns 404.
      setParams({ tenant })
      await refresh()
    } catch (error) { form.setState({ error, archiveConfirm: false }) }
  }
  const conflict = workflow && workflow.version !== state.version
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div><div className="mb-2 text-xs font-medium text-muted-foreground">{workflow ? `草稿 v${state.version}` : '新建草稿'}</div>
          <h2 className="text-xl font-semibold">{workflow?.name ?? '为你的流程起个名字'}</h2>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" disabled={busy || !state.name.trim()} onClick={() => void save()}><Save size={15} />{busy && (create.isPending || update.isPending) ? '保存中…' : '保存草稿'}</Button>
          <Button disabled={!workflow || state.dirty || busy || !!conflict} onClick={() => void publishVersion()}><Upload size={15} />{publish.isPending ? '发布中…' : '发布版本'}</Button>
        </div>
      </div>
      <p className="rounded-lg bg-muted/60 px-4 py-3 text-sm text-muted-foreground">当前支持草稿与版本管理，执行功能尚未开放。发布会保存独立版本，不会启动运行。</p>
      {state.error != null && <ErrorNotice error={state.error} />}
      {state.message && <p role="status" className="text-sm text-emerald-700">{state.message}</p>}
      {conflict && <div role="alert" className="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">服务器已有草稿 v{workflow.version}。你的编辑仍保留，可先导出备份。
        <Button variant="outline" size="sm" className="ml-3" onClick={() => { if (!state.dirty || window.confirm('重新载入会丢弃未保存修改，继续吗？')) form.setState({ ...savedFields(workflow), dirty: false, error: null }) }}>重新载入最新草稿</Button>
      </div>}
      <fieldset disabled={busy} className="space-y-4">
        <label className="block space-y-2 text-sm font-medium">名称<input aria-label="名称" maxLength={200} className={fieldClass} value={state.name} onChange={(e) => change('name', e.target.value)} placeholder="例如：客户反馈整理" /></label>
        <label className="block space-y-2 text-sm font-medium">描述<textarea aria-label="描述" className={fieldClass} rows={2} value={state.description} onChange={(e) => change('description', e.target.value)} placeholder="这个工作流帮助你完成什么？" /></label>
        <div className="rounded-xl border">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
            <span className="flex items-center gap-2 text-sm font-medium"><GitBranch size={16} />流程文档</span>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>{state.dirty ? '有未保存修改' : workflow ? '已保存' : '待保存'}</span>
              <Button size="sm" variant="ghost" onClick={() => downloadGraph(state.name, state.graph)}><Download size={14} />导出</Button>
            </div>
          </div>
          <div className="p-5">
            <p className="mb-3 text-sm text-muted-foreground">可粘贴已有流程文档，或从开始节点继续编辑。完整画布编辑器将在后续提供。</p>
            <textarea aria-label="流程 JSON" spellCheck={false} className={`${fieldClass} min-h-64 font-mono text-xs leading-5`} value={state.graph} onChange={(e) => change('graph', e.target.value)} />
          </div>
        </div>
      </fieldset>
      {state.dirty && <p className="text-xs text-muted-foreground">请先保存草稿，再发布版本。</p>}
      {workflow && <Versions tenant={tenant} workflow={workflow.id} />}
      {workflow && <div className="border-t pt-5">
        {!state.archiveConfirm ? <Button variant="ghost" disabled={busy || state.dirty} onClick={() => form.setState({ archiveConfirm: true })}><Archive size={14} />归档工作流</Button>
          : <div role="alertdialog" aria-label="确认归档" className="flex flex-wrap items-center gap-3 rounded-lg border p-4 text-sm">
            <p>归档后将隐藏此工作流及其版本记录。</p><Button variant="destructive" disabled={busy} onClick={() => void archiveWorkflow()}>确认归档</Button>
            <Button variant="outline" disabled={busy} onClick={() => form.setState({ archiveConfirm: false })}>取消</Button>
          </div>}
      </div>}
      {blocker.state === 'blocked' && <div role="alertdialog" aria-label="未保存的修改" className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-6">
        <div className="max-w-sm space-y-4 rounded-xl bg-background p-6 shadow-xl"><h3 className="font-semibold">仍有未保存的修改</h3><p className="text-sm text-muted-foreground">留在这里保存草稿，或放弃修改后离开。</p>
          <div className="flex gap-2"><Button onClick={() => blocker.reset()}>继续编辑</Button><Button variant="outline" onClick={() => { form.setState({ dirty: false }); blocker.proceed() }}>放弃修改并离开</Button></div>
        </div>
      </div>}
    </div>
  )
}
