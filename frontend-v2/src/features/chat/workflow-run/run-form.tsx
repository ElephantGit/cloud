import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { isAxiosError } from 'axios'
import { useGetApiV1TenantsTidMembers } from '@/api/members/members'
import { DtsReceipt } from './dts-receipt'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel, FieldError } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getWorkflowForm, saveWorkflowFormDraft, submitWorkflowForm, retryWorkflowFormTasks, type WorkflowForm } from '@/lib/workflows/api/generated'
import { TaskAssigneePicker } from '@/features/collaboration/task-assignee'
import type { TaskAssigneeRef } from '@/features/collaboration/tasks-api/generated'
import { listSquads } from '@/lib/squads-api/generated'

type Answer = { key: string; decision: string; assignee?: TaskAssigneeRef; values: Record<string, string> }
const text = (v: unknown) => typeof v === 'string' ? v : ''

/** Reads a durable review; refreshing never invents a new form or task. */
export function RunForm({ tenantId, runId, runStatus }: { tenantId: string; runId: string; runStatus: string }) {
  const query = useQuery({ queryKey: ['workflow-form', tenantId, runId], queryFn: () => getWorkflowForm(tenantId, runId), refetchInterval: 3000, retry: false })
  if (query.isPending) return <p role="status">正在读取确认表单…</p>
  if (!query.data) return <div role="status">表单尚未生成，巡检完成后会自动显示。<Button variant="outline" onClick={() => void query.refetch()}>刷新</Button></div>
  return <FormEditor key={query.data.id} form={query.data} tenantId={tenantId} runStatus={runStatus} />
}

function FormEditor({ form, tenantId, runStatus }: { form: WorkflowForm; tenantId: string; runStatus: string }) {
  const roster=useGetApiV1TenantsTidMembers(tenantId,undefined,{query:{enabled:!!tenantId}})
  const client = useQueryClient()
  const [version, setVersion] = useState(form.version)
  const [answers, setAnswers] = useState<Answer[]>(() => form.findings.map(row => {
    const saved = form.answers.find(a => a.key === row.key)
    return { key: text(row.key), decision: text(saved?.decision) || 'pending', values: (saved?.values ?? {}) as Record<string, string>, assignee: (saved?.assignee ?? form.existingTasks?.find(t=>t.key===row.key)?.assignee) as TaskAssigneeRef | undefined }
  }))
  const [notice, setNotice] = useState('')
  const [errors,setErrors]=useState<Record<string,string>>({})
  const [pendingOnly, setPendingOnly] = useState(false)
  const [submitKey] = useState(() => crypto.randomUUID())
  const squads = useQuery({ queryKey: ['squads', tenantId], queryFn: () => listSquads(tenantId) })
  const mutation = useMutation({ mutationFn: (final: boolean) => final
    ? submitWorkflowForm(tenantId, form.runId, { version, answers }, { headers: { 'Idempotency-Key': submitKey } })
    : saveWorkflowFormDraft(tenantId, form.runId, { version, answers }),
    onError: (error) => {
      if(isAxiosError(error)){const data=error.response?.data as {key?:string;field?:string}|undefined;if(data?.key&&data.field)setErrors({[`${data.key}:${data.field}`]:'请检查这一项的输入。'})}
    },
    onSuccess: (saved, final) => {
      setVersion(saved.version)
      setNotice(final ? '确认已提交，正在创建或关联任务。' : '草稿已保存。')
      void client.invalidateQueries({ queryKey: ['workflow-form', tenantId, form.runId] })
      void client.invalidateQueries({ queryKey: ['workflow-runs', tenantId] })
    },
  })
  const retry = useMutation({mutationFn:()=>retryWorkflowFormTasks(tenantId,form.runId),onSuccess:()=>client.invalidateQueries({queryKey:['workflow-form',tenantId,form.runId]})})
  const readOnly = !form.canEdit || mutation.isPending
  const remaining = answers.filter(a => a.decision === 'pending').length
  const complete = remaining === 0 && answers.every(a => a.decision !== 'confirmed' || a.assignee || form.existingTasks?.some(t=>t.key===a.key))
  const fields = Array.isArray(form.config.fields) ? form.config.fields as Record<string, unknown>[] : []
  function patch(key: string, change: Partial<Answer>) { setErrors({}); setAnswers(current => current.map(a => a.key === key ? { ...a, ...change } : a)); setNotice('有未保存的修改') }
  function save(final:boolean){
    const found:Record<string,string>={}
    if(final)for(const a of answers){
      if(a.decision==='pending')found[`${a.key}:decision`]='请选择确认结果。'
      if(a.decision==='confirmed'){
        if(!a.assignee&&!form.existingTasks?.some(t=>t.key===a.key))found[`${a.key}:assignee`]='请选择负责人。'
        for(const f of fields)if(f.required===true&&f.type!=='readonly'&&!a.values[text(f.name)]?.trim())found[`${a.key}:${text(f.name)}`]='请填写这一项。'
      }
    }
    setErrors(found);if(Object.keys(found).length)return;mutation.mutate(final)
  }
  return <section className="mx-auto flex w-full max-w-5xl flex-col gap-5 overflow-auto p-4 md:p-6" aria-label="巡检报告与问题确认">
    <div><h2 className="text-xl font-semibold">{text(form.config.title) || '问题确认'}</h2><p className="text-sm text-muted-foreground">{form.status === 'cancelled' ? '运行已取消，表单不可提交' : form.status === 'submitted' ? '已提交 · 任务由后续节点处理' : `已确认 ${answers.length - remaining}/${answers.length} 项`}</p></div>
    {form.status==='submitted' && <p className="text-sm text-muted-foreground">提交人：{roster.data?.items.find(m=>m.userId===form.submittedBy)?.displayName ?? '确认人'}{form.submittedAt ? ` · ${new Date(form.submittedAt).toLocaleString()}` : ''}</p>}
    <details open><summary className="cursor-pointer font-medium">巡检报告</summary><div className="mt-3 overflow-x-auto rounded-lg border p-4 text-sm [&_h1]:mb-3 [&_h1]:text-lg [&_h2]:my-3 [&_p]:my-2 [&_td]:border [&_td]:p-2 [&_th]:border [&_th]:p-2"><ReactMarkdown remarkPlugins={[remarkGfm]}>{form.report}</ReactMarkdown></div></details>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={pendingOnly} onChange={e => setPendingOnly(e.target.checked)} />只看待确认问题</label>
    {form.findings.length === 0 && <p>本次未发现需要人工处理的问题。</p>}
    {form.findings.map(row => {
      const key = text(row.key), answer = answers.find(a => a.key === key)!
      if (pendingOnly && answer.decision !== 'pending') return null
      const result = form.results.find(r => r.key === key)
      const existing = form.existingTasks?.find(r=>r.key===key)
      return <article key={key} className="rounded-xl border bg-card p-4">
        <h3 className="font-semibold">{text(row.title)}</h3><p className="text-sm text-muted-foreground">{text(row.module)} · {text(row.severity)}</p>
        <p className="my-2 text-sm">{text(row.description)}</p>
        <details className="mb-4 text-sm"><summary className="cursor-pointer">查看证据</summary><p className="whitespace-pre-wrap">{text(row.evidence)}</p></details>
        <FieldGroup>
          <Field><FieldLabel htmlFor={`${key}-decision`}>是否为问题</FieldLabel><NativeSelect id={`${key}-decision`} disabled={readOnly} value={answer.decision} onChange={e => patch(key, { decision: e.target.value })}><NativeSelectOption value="pending">待确认</NativeSelectOption><NativeSelectOption value="confirmed">是问题</NativeSelectOption><NativeSelectOption value="rejected">不是问题</NativeSelectOption></NativeSelect></Field>
          {existing && !result && <p className="text-sm">确认后将关联已有任务，沿用其负责人：<Link className="underline" to={`/collaboration/tasks/${text(existing.taskId)}`}>查看任务</Link></p>}
          {answer.decision === 'confirmed' && !existing && <Field><FieldLabel>负责人</FieldLabel><TaskAssigneePicker tenantId={tenantId} squads={squads.data?.items ?? []} value={answer.assignee ?? null} disabled={readOnly} onAssign={assignee => patch(key, { assignee: assignee ?? undefined })} /></Field>}
          {fields.map(f => {
            const name = text(f.name), value = answer.values[name] ?? ''
            if(f.type==='readonly')return <Field key={name}><FieldLabel>{text(f.label)}</FieldLabel><p className="text-sm whitespace-pre-wrap">{text(row[name])}</p></Field>
            const update = (value: string) => patch(key, { values: { ...answer.values, [name]: value } })
            return <Field key={name} data-invalid={!!errors[`${key}:${name}`]}><FieldLabel htmlFor={`${key}-${name}`}>{text(f.label)}{f.required === true ? ' *' : ''}</FieldLabel>
              {f.type === 'select' ? <NativeSelect id={`${key}-${name}`} aria-invalid={!!errors[`${key}:${name}`]} disabled={readOnly} value={value} onChange={e => update(e.target.value)}><NativeSelectOption value="">请选择</NativeSelectOption>{(Array.isArray(f.options) ? f.options : []).map(o => <NativeSelectOption key={String(o)} value={String(o)}>{String(o)}</NativeSelectOption>)}</NativeSelect> : f.type === 'textarea' ? <Textarea id={`${key}-${name}`} aria-invalid={!!errors[`${key}:${name}`]} disabled={readOnly} value={value} onChange={e => update(e.target.value)} /> : <Input id={`${key}-${name}`} aria-invalid={!!errors[`${key}:${name}`]} disabled={readOnly} value={value} onChange={e => update(e.target.value)} />}
              <FieldError>{errors[`${key}:${name}`]}</FieldError>
            </Field>
          })}
        </FieldGroup>
        {result && <p className="mt-4 text-sm">{result.action === 'created' ? '已创建任务' : '已关联现有任务'} · <Link className="underline" to={`/collaboration/tasks/${text(result.taskId)}`}>打开任务</Link></p>}
        {result && <DtsReceipt taskId={text(result.taskId)} title={text(row.title)} description={text(row.description)} severity={text(row.severity)} module={text(row.module)} runId={form.runId} />}
      </article>
    })}
    {mutation.isError && <p role="alert">保存失败，请检查必填项或刷新核对最新版本。你的输入仍保留在页面中。</p>}
    {notice && <p role="status">{form.status==='submitted' && form.results.length>0 ? '任务已创建或关联，可打开查看。' : form.status==='submitted' && !answers.some(a=>a.decision==='confirmed') ? '确认已提交，无需创建任务。' : notice}</p>}
    {runStatus==='failed' && form.status==='submitted' && form.results.length===0 && answers.some(a=>a.decision==='confirmed') && <Button variant="outline" disabled={retry.isPending} onClick={()=>retry.mutate()}>重试任务创建</Button>}
    {retry.isError && <p role="alert">任务当前不可重试，请查看运行状态。</p>}
    {form.status === 'open' && <footer className="sticky bottom-0 flex flex-wrap items-center gap-3 border-t bg-background py-3"><Button variant="outline" disabled={readOnly} onClick={() => save(false)}>保存草稿</Button><Button disabled={readOnly || !complete} onClick={() => save(true)}>{mutation.isPending ? '正在保存…' : text(form.config.submitLabel) || '确认并生成任务'}</Button>{!form.canEdit && <p className="text-sm">仅指定确认人或管理员可以提交。</p>}</footer>}
  </section>
}
