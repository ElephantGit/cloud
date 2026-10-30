import { useScheduleWorkflow } from './workflow-catalog'
import { resolveWorkflowInputFieldType } from '@/lib/workflows/mock'
import { SourceTaskPicker } from './source-task-picker'
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Check, Clock3, GitBranch, MessageSquare, Search, ListTodo } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { catalog, emptyDraft, formatTime, nextTimes, summary, taskDraft, validateDraft, weekdayNames, type Catalog, type Draft, type Task } from './schedule-model'

export function ScheduleForm({ task, onSave, onCancel, data: baseData = catalog, saving = false, blocked = false, saveError, onCloseReady }: { task?: Task; onSave: (draft: Draft, data: Catalog) => void; onCancel: () => void; data?: Catalog; saving?: boolean; blocked?: boolean; saveError?: string; onCloseReady?: (close: () => void) => void }) {
  const [draft, setDraft] = useState(() => task ? taskDraft(task) : emptyDraft())
  const workflow = useScheduleWorkflow(draft, baseData)
  const data = workflow.data
  const [nameEdited, setNameEdited] = useState(!!task)
  const source = data.tasks?.find(t => t.id === draft.sourceTaskId)
  const effectiveDraft = { ...draft, inputs: draft.kind === 'workflow' ? workflow.inputs : draft.inputs, name: draft.kind === 'task' && source && !nameEdited ? `${source.title} · 定时执行` : draft.name, projectId: draft.kind === 'task' ? source?.projectId ?? '' : draft.projectId }
  const sourceError = draft.kind === 'task' ? validateDraft(effectiveDraft, data).sourceTaskId : undefined
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [query, setQuery] = useState('')
  const [discard, setDiscard] = useState(false)
  const [dirty, setDirty] = useState(false)
  useEffect(() => { onCloseReady?.(() => dirty ? setDiscard(true) : onCancel()) }, [onCloseReady, dirty, onCancel])
  const [previewNow] = useState(() => new Date())
  const patch = (changes: Partial<Draft>) => {
    const next = { ...draft, ...changes }
    setDraft(next)
    setDirty(true)
    setErrors(previous => {
      const current = validateDraft(next, data)
      return Object.fromEntries(Object.keys(previous).filter(key => current[key]).map(key => [key, current[key]!]))
    })
  }
  const rule = (changes: Partial<Draft['rule']>) => patch({ rule: { ...draft.rule, ...changes } })
  const flow = data.workflows.find(w => w.id === draft.workflowId)
  const times = nextTimes(draft.rule, previewNow, task ? new Date(task.createdAt) : previewNow)
  const control = (name: string) => ({ id: `schedule-${name}`, 'aria-invalid': !!errors[name], 'aria-describedby': errors[name] ? `error-${name}` : undefined })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const allErrors = validateDraft(effectiveDraft, data)
    setErrors(allErrors)
    if (Object.keys(allErrors).length) {
      const form = event.currentTarget
      requestAnimationFrame(() => form.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus())
      return
    }
    if (draft.kind === 'workflow' && (workflow.loading || workflow.error)) return
    onSave(effectiveDraft, data)
  }

  return <form className="sp-form" onSubmit={submit} noValidate>
    <div className="sp-form-scroll"><FieldGroup>
      <FieldSet className="sp-section">
        <FieldLegend variant="label"><span className="sp-section-number">01</span>执行内容</FieldLegend>
        <FieldGroup>
          <ScheduleField name="name" label="任务名称" error={errors.name}><Input placeholder="例如：每日 AI 简报" value={effectiveDraft.name} onChange={e => { setNameEdited(true); patch({ name: e.target.value }) }} {...control('name')} /></ScheduleField>
          <ToggleGroup className="sp-kind-picker w-full" aria-label="执行方式" value={[draft.kind]} onValueChange={values => { const value = values[0]; if (value === 'chat' || value === 'workflow' || value === 'task') patch({ kind: value, projectId: value === 'chat' ? '' : draft.projectId }) }} variant="outline" spacing={3}>
            <ToggleGroupItem disabled={!!task && task.target.kind !== 'chat'} className="sp-choice h-auto min-h-24 flex-1 flex-col items-start gap-2 p-4" value="chat"><MessageSquare /><strong>直接对话</strong><small>用一段指令完成工作</small>{draft.kind === 'chat' && <Check className="sp-selected-check" />}</ToggleGroupItem>
            <ToggleGroupItem disabled={!!task && task.target.kind !== 'workflow'} className="sp-choice h-auto min-h-24 flex-1 flex-col items-start gap-2 p-4" value="workflow"><GitBranch /><strong>运行工作流</strong><small>执行已发布的多步流程</small>{draft.kind === 'workflow' && <Check className="sp-selected-check" />}</ToggleGroupItem>
            <ToggleGroupItem disabled={!!task && task.target.kind !== 'task'} className="sp-choice h-auto min-h-24 flex-1 flex-col items-start gap-2 p-4" value="task"><ListTodo /><strong>已有任务</strong><small>选择 Cloud 中的任务</small>{draft.kind === 'task' && <Check className="sp-selected-check" />}</ToggleGroupItem>
          </ToggleGroup>
          {draft.kind === 'task' ? <>
            <SourceTaskPicker tasks={data.tasks ?? []} value={draft.sourceTaskId} onChange={sourceTaskId => patch({ sourceTaskId })} error={errors.sourceTaskId || (draft.sourceTaskId ? sourceError : undefined)} />
            <ScheduleField name="supplementalInstruction" label="补充说明（可选）"><Textarea rows={3} placeholder="例如：优先检查发布阻塞问题，输出简洁清单…" value={draft.supplementalInstruction} onChange={e => patch({ supplementalInstruction: e.target.value })} {...control('supplementalInstruction')} /></ScheduleField>
            <FieldDescription>每次读取任务最新描述与项目，独立执行；不携带历史对话，结果仅保存在定时任务历史。任务已完成也可继续定时运行。</FieldDescription>
          </> : draft.kind === 'chat' ? <>
            <ScheduleField name="prompt" label="任务指令" error={errors.prompt} hint="每次运行新建独立对话，不携带上次聊天记录。"><Textarea className="min-h-28" rows={4} placeholder="描述任务目标、需要关注的信息，以及期望的输出格式…" value={draft.prompt} onChange={e => patch({ prompt: e.target.value })} {...control('prompt')} /></ScheduleField>
            <FieldGroup className="grid grid-cols-2 gap-4">
              <ScheduleField name="agent" label="执行器"><NativeSelect disabled className="w-full" id="schedule-agent" value={draft.agent} onChange={e => patch({ agent: e.target.value })}><NativeSelectOption value={draft.agent}>尚未接入</NativeSelectOption></NativeSelect></ScheduleField>
              <ScheduleField name="model" label="模型"><NativeSelect disabled className="w-full" id="schedule-model" value={draft.model} onChange={e => patch({ model: e.target.value })}><NativeSelectOption value={draft.model}>尚未接入</NativeSelectOption></NativeSelect></ScheduleField>
            </FieldGroup>
          </> : <>
            <FieldDescription>从工作流页面的共享库选择已发布版本。定时配置为前端演示，刷新后清除；真实执行尚未接入。</FieldDescription>
            {workflow.loading && <FieldDescription>正在读取工作流…</FieldDescription>}
            {workflow.error && <p role="alert">{workflow.error}</p>}
            <Field data-invalid={!!errors.workflow}>
              <FieldLabel htmlFor="workflow-search">选择工作流</FieldLabel>
              <div className="sp-search"><Search size={15} aria-hidden="true" /><Input id="workflow-search" placeholder="搜索工作流名称" value={query} onChange={e => setQuery(e.target.value)} /></div>
              <ToggleGroup className="sp-workflow-list w-full" orientation="vertical" aria-label="可选工作流" value={[draft.workflowId]} spacing={1} onValueChange={values => { const selected = data.workflows.find(w => w.id === values[0]); if (selected?.versions.length) patch({ workflowId: selected.id, snapshot: selected.versions[0], inputs: {} }) }}>
                {data.workflows.filter(w => `${w.name}${w.description}`.toLowerCase().includes(query.trim().toLowerCase())).map(w => <ToggleGroupItem className="sp-workflow-option h-auto w-full justify-start gap-3 p-3" key={w.id} value={w.id} disabled={!w.versions.length}><GitBranch /><span><strong>{w.name}</strong><small>{w.description}</small></span><Badge variant="outline">{w.versions[0] ?? '未发布'}</Badge>{draft.workflowId === w.id && <Check />}</ToggleGroupItem>)}
              </ToggleGroup>
              {!data.workflows.some(w => `${w.name}${w.description}`.toLowerCase().includes(query.trim().toLowerCase())) && <p className="sp-empty-small">没有匹配的工作流</p>}
              {!data.workflows.some(w => w.versions.length) && <FieldDescription>暂无已发布工作流，请先在工作流页面完成发布。</FieldDescription>}
              <FieldError id="error-workflow">{errors.workflow}</FieldError>
            </Field>
            {flow?.versions.length ? <FieldGroup className="sp-flow-inputs">
              <ScheduleField name="snapshot" label="固定执行版本" hint="后续发布不会自动升级此任务，可通过编辑切换版本。"><NativeSelect className="w-full" id="schedule-snapshot" value={draft.snapshot} onChange={e => patch({ snapshot: e.target.value, inputs: {} })}>{flow.versions.map(v => <NativeSelectOption key={v} value={v}>{v}</NativeSelectOption>)}</NativeSelect></ScheduleField>
              {flow.fields.map(f => {
                const type = f.variable ? resolveWorkflowInputFieldType(f.variable) : 'text-input'
                const value = effectiveDraft.inputs[f.key] ?? ''
                const update = (value: string) => patch({ inputs: { ...effectiveDraft.inputs, [f.key]: value } })
                return <ScheduleField key={f.key} name={`input:${f.key}`} label={`${f.name}${f.required ? ' *' : '（可选）'}`} error={errors[`input:${f.key}`]}>
                  {type === 'select' ? <NativeSelect value={value} onChange={e => update(e.target.value)} {...control(`input:${f.key}`)}><NativeSelectOption value="">请选择</NativeSelectOption>{f.variable?.options?.map(option => <NativeSelectOption key={option} value={option}>{option}</NativeSelectOption>)}</NativeSelect>
                    : type === 'checkbox' ? <Input type="checkbox" className="size-4" checked={value === 'true'} onChange={e => update(String(e.target.checked))} {...control(`input:${f.key}`)} />
                    : ['paragraph', 'file-list', 'json'].includes(type) ? <Textarea value={value} placeholder={f.placeholder} onChange={e => update(e.target.value)} {...control(`input:${f.key}`)} />
                    : <Input type={type === 'number' ? 'number' : 'text'} value={value} placeholder={f.placeholder} onChange={e => update(e.target.value)} {...control(`input:${f.key}`)} />}
                </ScheduleField>
              })}
            </FieldGroup> : null}
          </>}
        </FieldGroup>
      </FieldSet>

      <FieldSet className="sp-section">
        <FieldLegend variant="label"><span className="sp-section-number">02</span>运行环境</FieldLegend>
        <FieldGroup>
          {draft.kind === 'task' ? <ScheduleField name="sourceProject" label="关联项目" hint="自动沿用来源任务的最新项目。"><Input id="schedule-sourceProject" readOnly value={source?.projectName ?? ''} placeholder="选择任务后自动带入" /></ScheduleField> : <>
          <ScheduleField name="projectId" label="关联项目" error={errors.projectId} hint={draft.kind === 'workflow' ? '必选。项目为工作流提供文件与运行环境。' : draft.projectId ? '本次任务可使用所选项目的文件与工作环境。' : '可选。不关联项目时独立执行，不使用项目文件。'}>
            <NativeSelect className="w-full" value={draft.projectId} onChange={e => patch({ projectId: e.target.value })} {...control('projectId')}><NativeSelectOption value="">{draft.kind === 'workflow' ? '请选择运行项目' : '不关联项目'}</NativeSelectOption>{(draft.kind === 'workflow' ? data.projects : []).map(p => <NativeSelectOption key={p.id} value={p.id}>{p.name}</NativeSelectOption>)}</NativeSelect>
          </ScheduleField>
          {!data.projects.length && <FieldDescription>暂无可用项目。直接对话可独立执行，工作流需先创建项目。</FieldDescription>}
          </>}
        </FieldGroup>
      </FieldSet>

      <FieldSet className="sp-section">
        <FieldLegend variant="label"><span className="sp-section-number">03</span>时间安排</FieldLegend>
        <FieldGroup>
          <FieldGroup className="grid grid-cols-2 gap-4">
            <ScheduleField name="frequency" label="重复频率"><NativeSelect className="w-full" id="schedule-frequency" value={draft.rule.kind} onChange={e => rule({ kind: e.target.value as Draft['rule']['kind'] })}><NativeSelectOption value="daily">每天</NativeSelectOption><NativeSelectOption value="weekdays">工作日（周一至周五）</NativeSelectOption><NativeSelectOption value="weekly">每周</NativeSelectOption><NativeSelectOption value="hours">每隔若干小时</NativeSelectOption></NativeSelect></ScheduleField>
            {draft.rule.kind === 'hours' ? <ScheduleField name="hours" label="间隔小时数" error={errors.hours}><Input type="number" min={1} max={24} value={draft.rule.hours} onChange={e => rule({ hours: Number(e.target.value) })} {...control('hours')} /></ScheduleField> : <ScheduleField name="time" label="执行时间" error={errors.time}><Input type="time" value={draft.rule.time} onChange={e => rule({ time: e.target.value })} {...control('time')} /></ScheduleField>}
          </FieldGroup>
          {draft.rule.kind === 'weekly' && <Field data-invalid={!!errors.days}><ToggleGroup className="sp-weekdays w-full" aria-label="执行星期" multiple variant="outline" value={draft.rule.days.map(String)} onValueChange={values => rule({ days: values.map(Number) })} spacing={2}>{[1, 2, 3, 4, 5, 6, 0].map(day => <ToggleGroupItem className="flex-1" key={day} value={String(day)} aria-label={`星期${weekdayNames[day]}`}>{weekdayNames[day]}</ToggleGroupItem>)}</ToggleGroup><FieldError id="error-days">{errors.days}</FieldError></Field>}
          <ScheduleField name="timezone" label="时区" error={errors.timezone}><NativeSelect className="w-full" value={draft.rule.timezone} onChange={e => rule({ timezone: e.target.value })} {...control('timezone')}><NativeSelectOption value="Asia/Shanghai">Asia/Shanghai · 北京时间</NativeSelectOption><NativeSelectOption value="Asia/Tokyo">Asia/Tokyo · 东京</NativeSelectOption><NativeSelectOption value="Europe/London">Europe/London · 伦敦</NativeSelectOption><NativeSelectOption value="America/New_York">America/New_York · 纽约</NativeSelectOption><NativeSelectOption value="UTC">UTC · 协调世界时</NativeSelectOption></NativeSelect></ScheduleField>
          <div className="sp-time-preview"><div><Clock3 size={15} /><strong>接下来三次</strong><span>{draft.rule.timezone}</span></div>{times.length ? times.map((time, i) => <p key={time.toISOString()}><span>{String(i + 1).padStart(2, '0')}</span>{formatTime(time, draft.rule.timezone)}</p>) : <p>填写完整时间规则后预览</p>}<small>{draft.rule.kind === 'hours' ? '从创建时间起，按固定小时数间隔计算。' : '按所选时区计算，跳过夏令时不存在的时刻。'}</small></div>
        </FieldGroup>
      </FieldSet>
      <div className="sp-summary"><Clock3 size={17} /><div><strong>任务预览</strong><p>{summary(effectiveDraft, data)}</p><small>{draft.kind !== 'chat' ? '此配置为前端演示，仅在本次浏览会话中保存，刷新后清除；不会执行真实任务。' : '配置已保存后可随时编辑；自动执行尚未接入。'}</small></div></div>
    </FieldGroup></div>
    {saveError && <p role="alert" className="sp-helper">{saveError}</p>}
    <footer className="sp-form-footer">
      {discard ? <><span>放弃未保存的修改？</span><Button type="button" variant="outline" onClick={() => setDiscard(false)}>继续编辑</Button><Button type="button" onClick={onCancel}>放弃修改</Button></> : <><Button type="button" variant="outline" onClick={() => dirty ? setDiscard(true) : onCancel()}>取消</Button><span className="sp-footer-spacer" /><Button type="submit" disabled={saving || blocked || !!sourceError || (draft.kind === 'workflow' && (workflow.loading || !!workflow.error))}>{saving ? '保存中…' : task ? '保存修改' : '创建并启用'}</Button></>}
    </footer>
  </form>
}

function ScheduleField({ name, label, error, hint, children }: { name: string; label: string; error?: string; hint?: string; children: ReactNode }) {
  return <Field data-invalid={!!error}><FieldLabel htmlFor={`schedule-${name}`}>{label}</FieldLabel>{children}{hint && <FieldDescription>{hint}</FieldDescription>}<FieldError id={`error-${name}`}>{error}</FieldError></Field>
}
