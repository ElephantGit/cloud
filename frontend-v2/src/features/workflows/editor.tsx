import { useEffect, useRef, useState } from 'react'
import { useBlocker, useSearchParams } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useStore } from 'zustand'
import { Save, Upload, Download, Archive, GitBranch, Copy } from 'lucide-react'
import type { Workflow } from '@/api/generated.schemas'
import { usePostApiV1TenantsTidWorkflows, usePutApiV1TenantsTidWorkflowsWfid, usePostApiV1TenantsTidWorkflowsWfidPublish, useDeleteApiV1TenantsTidWorkflowsWfid } from '@/api/workflows/workflows'
import { Button } from '@/components/ui/button'
import { createDraftStore, parseGraph, savedFields } from './draft'
import { downloadGraph, fieldClass } from './shared'
import { ErrorNotice } from './error-notice'
import { Versions } from './versions'
import { WorkflowCanvas } from './workflow-canvas'
import { validateCanvasGraph } from './workflow-graph-validation'
import { WorkflowNodeInspector } from './workflow-node-inspector'
import { WorkflowGlobalVariables } from './workflow-global-variables'

export function Editor({ tenant, workflow }: { tenant: string; workflow?: Workflow }) {
  const [form] = useState(() => createDraftStore(workflow))
  const [globalOpen, setGlobalOpen] = useState(false)
  const [importPreview, setImportPreview] = useState<{ file: string; graph: string; name: string; nodes: number; edges: number } | null>(null)
  const [importError, setImportError] = useState('')
  const [externalEdit, setExternalEdit] = useState<{ before: string; after: string; id: number; nameBefore?: string; nameAfter?: string }>()
  const [detailsOpen, setDetailsOpen] = useState(!workflow)
  const [restoring, setRestoring] = useState(false)
  const [publishingRequested, setPublishingRequested] = useState(false)
  const generation = useRef(0)
  const importCounter = useRef(0)
  const pending = useRef<Promise<boolean> | null>(null)
  const saveRef = useRef<() => Promise<boolean>>(async () => false)
  const state = useStore(form)
  const [, setParams] = useSearchParams()
  const client = useQueryClient()
  const create = usePostApiV1TenantsTidWorkflows()
  const update = usePutApiV1TenantsTidWorkflowsWfid()
  const publish = usePostApiV1TenantsTidWorkflowsWfidPublish()
  const archive = useDeleteApiV1TenantsTidWorkflowsWfid()
  const busy = create.isPending || update.isPending || publish.isPending || archive.isPending || restoring || publishingRequested
  const blocker = useBlocker(({ currentLocation, nextLocation }) => (form.getState().dirty || restoring || publishingRequested) &&
    (currentLocation.pathname !== nextLocation.pathname || currentLocation.search !== nextLocation.search))
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => { if (form.getState().dirty) event.preventDefault() }
    window.addEventListener('beforeunload', guard)
    return () => window.removeEventListener('beforeunload', guard)
  }, [form])
  const change = (key: 'name' | 'description' | 'graph', value: string) => { generation.current++; form.setState({ [key]: value, dirty: true, message: '', error: null }) }
  const refresh = () => client.invalidateQueries({ predicate: (q) => typeof q.queryKey[0] === 'string' && q.queryKey[0].startsWith(`/api/v1/tenants/${tenant}/workflows`) })
  function save(openCreated = true): Promise<boolean> {
    if (pending.current) return pending.current
    const task = (async () => {
      do {
        const current = form.getState()
        if (!current.dirty || !current.name.trim()) return !current.dirty
        const captured = generation.current
        form.setState({ error: null, message: '' })
        try {
          const data = { name: current.name.trim(), description: current.description, graph: parseGraph(current.graph), version: current.version }
          const result = workflow
            ? await update.mutateAsync({ tid: tenant, wfid: workflow.id, data })
            : (await create.mutateAsync({ tid: tenant, data: { name: data.name, description: data.description, graph: data.graph } })).resource
          if (generation.current === captured) form.setState({ ...savedFields(result), dirty: false, message: '草稿已保存。' })
          else form.setState({ version: result.version })
          await refresh()
          if (!workflow) { if (openCreated) setParams({ tenant, workflow: result.id }); return true }
        } catch (error) { form.setState({ error }); return false }
      } while (form.getState().dirty)
      return true
    })()
    pending.current = task
    void task.finally(() => { if (pending.current === task) pending.current = null })
    return task
  }
  useEffect(() => { saveRef.current = save })
  useEffect(() => {
    if (!workflow || !state.dirty || !state.name.trim() || state.error) return
    const timer = window.setTimeout(() => { void saveRef.current() }, 1000)
    return () => window.clearTimeout(timer)
  }, [workflow, state.dirty, state.name, state.graph, state.description, state.error])
  async function previewImport(file: File) {
    setImportError('')
    setImportPreview(null)
    try {
      const document = parseGraph(await file.text())
      if (!Array.isArray(document.nodes) || !Array.isArray(document.edges)) throw new Error('导入文件需要包含 nodes 和 edges 数组。')
      const problem = validateCanvasGraph(JSON.stringify(document))
      if (problem) throw new Error(`导入文件无法显示：${problem}。`)
      setImportPreview({ file: file.name, graph: JSON.stringify(document, null, 2), name: typeof document.name === 'string' ? document.name : '', nodes: document.nodes.length, edges: document.edges.length })
    } catch (error) { setImportError(error instanceof Error ? error.message : '无法读取导入文件。') }
  }
  function confirmImport() {
    if (!importPreview) return
    const current = form.getState()
    const importedName = !workflow && !current.name.trim() && importPreview.name ? importPreview.name : undefined
    setExternalEdit({ before: current.graph, after: importPreview.graph, id: ++importCounter.current, ...(importedName ? { nameBefore: current.name, nameAfter: importedName } : {}) })
    generation.current++
    form.setState({ graph: importPreview.graph, name: importedName ?? current.name, dirty: true, error: null, message: '' })
    setImportPreview(null)
  }
  function changeCanvas(graph: string) {
    generation.current++
    const current = form.getState()
    const name = externalEdit?.nameAfter && graph === externalEdit.before && current.name === externalEdit.nameAfter ? externalEdit.nameBefore : externalEdit?.nameAfter && graph === externalEdit.after && current.name === externalEdit.nameBefore ? externalEdit.nameAfter : current.name
    form.setState({ graph, name: name ?? current.name, dirty: true, message: '', error: null })
  }
  async function publishVersion() {
    if (!workflow) return
    setPublishingRequested(true)
    try {
      if (!(await save())) return
      form.setState({ error: null, message: '' })
      const result = await publish.mutateAsync({ tid: tenant, wfid: workflow.id, data: {} })
      form.setState({ message: `版本 v${result.resource.version} 已发布，内容已冻结。` })
      await refresh()
    } catch (error) { form.setState({ error }) } finally { setPublishingRequested(false) }
  }
  async function copyWorkflow() {
    if (!workflow || !(await save())) return
    const current = form.getState()
    try {
      const result = await create.mutateAsync({ tid: tenant, data: { name: current.name + ' - 副本', description: current.description, graph: parseGraph(current.graph) } })
      await refresh()
      setParams({ tenant, workflow: result.resource.id })
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
  const globalVariables = (() => { try { return parseGraph(state.graph).globalVariables } catch { return undefined } })()
  const conflict = workflow && workflow.version !== state.version
  return (
    <div className="mx-auto max-w-7xl space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div><div className="text-xs font-medium text-muted-foreground">{workflow ? `草稿 v${state.version}` : '新建草稿'}</div>
          <h2 className="text-xl font-semibold">{workflow?.name ?? '为你的流程起个名字'}</h2>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" disabled={busy || !state.name.trim()} onClick={() => void save()}><Save size={15} />{busy && (create.isPending || update.isPending) ? '保存中…' : '保存草稿'}</Button>
          <Button variant="outline" disabled={!workflow || busy || !!conflict} onClick={() => void copyWorkflow()}><Copy size={15} />复制</Button>
          <Button variant="outline" disabled title="当前服务暂不支持工作流执行">运行</Button>
          <Button variant="outline" disabled title="当前服务暂不支持工作流运行历史">运行历史</Button>
          <Button disabled={!workflow || busy || !!conflict} onClick={() => void publishVersion()}><Upload size={15} />{publish.isPending ? '发布中…' : '发布版本'}</Button>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">当前支持草稿与版本管理，执行功能尚未开放。发布会保存独立版本，不会启动运行。</p>
      {state.error != null && <ErrorNotice error={state.error} />}
      {state.message && <p role="status" className="text-sm text-emerald-700">{state.message}</p>}
      {conflict && <div role="alert" className="rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">服务器已有草稿 v{workflow.version}。你的编辑仍保留，可先导出备份。
        <Button variant="outline" size="sm" className="ml-3" onClick={() => { if (!state.dirty || window.confirm('重新载入会丢弃未保存修改，继续吗？')) form.setState({ ...savedFields(workflow), dirty: false, error: null }) }}>重新载入最新草稿</Button>
      </div>}
      <fieldset inert={restoring || create.isPending || publish.isPending || archive.isPending || publishingRequested || undefined} disabled={create.isPending || publish.isPending || archive.isPending || restoring} className="space-y-4">
        <details open={detailsOpen} onToggle={(event) => setDetailsOpen(event.currentTarget.open)} className="rounded-lg border px-3 py-2"><summary className="cursor-pointer text-sm font-medium">工作流信息</summary><div className="grid gap-3 pt-3 md:grid-cols-2"><label className="block space-y-2 text-sm font-medium">名称<input aria-label="名称" maxLength={200} className={fieldClass} value={state.name} onChange={(e) => change('name', e.target.value)} placeholder="例如：客户反馈整理" /></label>
        <label className="block space-y-2 text-sm font-medium">描述<textarea aria-label="描述" className={fieldClass} rows={2} value={state.description} onChange={(e) => change('description', e.target.value)} placeholder="这个工作流帮助你完成什么？" /></label></div></details>
        <div className="rounded-xl border">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b px-3 py-2">
            <span className="flex items-center gap-2 text-sm font-medium"><GitBranch size={16} />流程文档</span>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>{state.dirty ? '有未保存修改' : workflow ? '已保存' : '待保存'}</span>
              <Button size="sm" variant="ghost" onClick={() => { try { downloadGraph(state.name, state.graph, state.description, workflow?.id) } catch (error) { form.setState({ error }) } }}><Download size={14} />导出</Button>
            </div>
          </div>
          <div className="p-2">
            <Button type="button" variant="outline" size="sm" className="mb-1" onClick={() => setGlobalOpen(true)}>全局变量</Button>
            <WorkflowCanvas graph={state.graph} externalEdit={externalEdit} onChange={changeCanvas} renderInspector={(props) => <WorkflowNodeInspector {...props} globalVariables={globalVariables} />} />
            {globalOpen && globalVariables !== undefined && !Array.isArray(globalVariables) ? <div role="alertdialog" aria-label="全局变量格式不支持" className="rounded-lg border p-4 text-sm"><p>全局变量格式无法在编辑器中显示，原始数据已保留。请在流程 JSON 中查看。</p><Button type="button" onClick={() => setGlobalOpen(false)}>关闭</Button></div> : globalOpen && <WorkflowGlobalVariables variables={globalVariables} onClose={() => setGlobalOpen(false)} onSave={(variables) => { try { change('graph', JSON.stringify({ ...parseGraph(state.graph), globalVariables: variables }, null, 2)); setGlobalOpen(false) } catch (error) { form.setState({ error }) } }} />}
            <details className="mt-4"><summary className="cursor-pointer text-sm">流程 JSON 和导入</summary>
            <label className="my-3 block text-sm">导入 JSON 文件 <input type="file" accept="application/json,.json" onChange={(event) => { const file = event.target.files?.[0]; if (file) void previewImport(file); event.target.value = '' }} /></label>
            {importError && <p role="alert" className="text-sm text-red-600">{importError}</p>}
            {importPreview && <div role="dialog" aria-label="导入预览" className="my-3 rounded-lg border bg-background p-4 text-sm"><h3 className="font-semibold">导入预览</h3><p>文件：{importPreview.file}</p><p>名称：{importPreview.name || '未命名'}</p><p>{importPreview.nodes} 个节点 · {importPreview.edges} 条连线</p><p className="text-muted-foreground">确认后替换当前画布，可使用撤销恢复。</p><div className="mt-3 flex gap-2"><Button type="button" onClick={confirmImport}>确认导入</Button><Button type="button" variant="outline" onClick={() => setImportPreview(null)}>取消</Button></div></div>}
            <textarea aria-label="流程 JSON" spellCheck={false} className={`${fieldClass} min-h-64 font-mono text-xs leading-5`} value={state.graph} onChange={(e) => change('graph', e.target.value)} />
            </details>
          </div>
        </div>
      </fieldset>
      {state.dirty && <p className="text-xs text-muted-foreground">请先保存草稿，再发布版本。</p>}
      {workflow && <Versions tenant={tenant} workflow={workflow.id} version={state.version} dirty={state.dirty || update.isPending} onRestoringChange={setRestoring} onRestore={(result) => { generation.current++; form.setState({ ...savedFields(result), dirty: false, error: null, message: '历史版本已恢复为草稿。' }); void refresh() }} />}
      {workflow && <div className="border-t pt-5">
        {!state.archiveConfirm ? <Button variant="ghost" disabled={busy || state.dirty} onClick={() => form.setState({ archiveConfirm: true })}><Archive size={14} />归档工作流</Button>
          : <div role="alertdialog" aria-label="确认归档" className="flex flex-wrap items-center gap-3 rounded-lg border p-4 text-sm">
            <p>归档后将隐藏此工作流及其版本记录。</p><Button variant="destructive" disabled={busy} onClick={() => void archiveWorkflow()}>确认归档</Button>
            <Button variant="outline" disabled={busy} onClick={() => form.setState({ archiveConfirm: false })}>取消</Button>
          </div>}
      </div>}
      {blocker.state === 'blocked' && <div role="alertdialog" aria-label="未保存的修改" className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-6">
        <div className="max-w-sm space-y-4 rounded-xl bg-background p-6 shadow-xl"><h3 className="font-semibold">仍有未保存的修改</h3><p className="text-sm text-muted-foreground">留在这里保存草稿，或放弃修改后离开。</p>
          <div className="flex gap-2"><Button onClick={() => blocker.reset()}>继续编辑</Button><Button disabled={busy || !state.name.trim()} onClick={() => void save(false).then((saved) => { if (saved) blocker.proceed() })}>保存并离开</Button><Button variant="outline" disabled={restoring || publishingRequested || create.isPending || update.isPending} onClick={() => { form.setState({ dirty: false }); blocker.proceed() }}>放弃修改并离开</Button></div>
        </div>
      </div>}
    </div>
  )
}
