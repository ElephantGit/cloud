import { useState, type FormEvent } from 'react'
import { Check, Clock3, Folder, GitBranch, MessageSquare, Search, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { catalog, emptyDraft, formatTime, nextTimes, summary, taskDraft, validateDraft, weekdayNames, type Catalog, type Draft, type Task } from './schedule-model'

export function ScheduleForm({ task, onSave, onCancel, data = catalog }: { task?: Task; onSave: (draft: Draft) => void; onCancel: () => void; data?: Catalog }) {
  const [draft, setDraft] = useState(() => task ? taskDraft(task) : emptyDraft())
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [query, setQuery] = useState('')
  const [discard, setDiscard] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [previewNow] = useState(() => new Date())
  const patch = (changes: Partial<Draft>) => { setDraft(d => ({ ...d, ...changes })); setDirty(true); setErrors({}) }
  const rule = (changes: Partial<Draft['rule']>) => patch({ rule: { ...draft.rule, ...changes } })
  const flow = data.workflows.find(w => w.id === draft.workflowId)
  const times = nextTimes(draft.rule, previewNow, task ? new Date(task.createdAt) : previewNow)
  const err = (field: string) => errors[field] ? <span role="alert" className="sp-field-error" id={`error-${field}`}>{errors[field]}</span> : null
  const invalid = (field: string) => ({ 'aria-invalid': !!errors[field], 'aria-describedby': errors[field] ? `error-${field}` : undefined })
  function submit(event: FormEvent) {
    event.preventDefault()
    const allErrors = validateDraft(draft, data)
    setErrors(allErrors)
    if (Object.keys(allErrors).length) {

      return
    }
    onSave(draft)
  }
  return <form className="sp-form" onSubmit={submit} noValidate>
    <div className="sp-form-scroll">
      <>
        <section className="sp-section"><h3><span>01</span>执行什么</h3>
          <label className="sp-field">任务名称<input placeholder="例如：每日 AI 简报" value={draft.name} onChange={e => patch({ name: e.target.value })} {...invalid('name')} />{err('name')}</label>
          <div className="sp-kind-picker" role="group" aria-label="执行方式">
            <button type="button" aria-pressed={draft.kind === 'chat'} className={draft.kind === 'chat' ? 'selected' : ''} onClick={() => patch({ kind: 'chat' })}><MessageSquare size={21} /><strong>直接对话</strong><small>给 AI 一段指令，定时执行</small>{draft.kind === 'chat' && <Check className="sp-selected-check" size={15} />}</button>
            <button type="button" aria-pressed={draft.kind === 'workflow'} className={draft.kind === 'workflow' ? 'selected' : ''} onClick={() => patch({ kind: 'workflow' })}><GitBranch size={21} /><strong>运行工作流</strong><small>按既定步骤完成多阶段任务</small>{draft.kind === 'workflow' && <Check className="sp-selected-check" size={15} />}</button>
          </div>
        </section>
        <section className="sp-section"><h3><span>02</span>任务内容</h3>
          {draft.kind === 'chat' ? <>
            <label className="sp-field">任务指令<textarea rows={5} placeholder="你希望 AI 定时为你做什么？说明目标、关注的信息和期望的输出格式。" value={draft.prompt} onChange={e => patch({ prompt: e.target.value })} {...invalid('prompt')} />{err('prompt')}</label>
            <p className="sp-helper"><Sparkles size={14} /> 每次运行都会新建独立对话，不携带上次聊天记录。</p>
            <div className="sp-two-col"><label className="sp-field">执行器<select value={draft.agent} onChange={e => patch({ agent: e.target.value })}><option>Ora Assistant</option><option>Research Assistant</option></select></label><label className="sp-field">模型<select value={draft.model} onChange={e => patch({ model: e.target.value })}><option>演示默认模型</option><option>演示深度分析模型</option></select></label></div>
          </> : <>
            <label className="sp-field">选择工作流<div className="sp-search"><Search size={15} /><input placeholder="搜索工作流名称" value={query} onChange={e => setQuery(e.target.value)} /></div></label>
            <div className="sp-workflow-list" role="group" aria-label="可选工作流">
              {data.workflows.filter(w => `${w.name}${w.description}`.includes(query)).map(w => <button type="button" key={w.id} disabled={!w.versions.length} aria-pressed={draft.workflowId === w.id} className={`sp-workflow-option ${draft.workflowId === w.id ? 'selected' : ''}`} onClick={() => patch({ workflowId: w.id, snapshot: w.versions[0] ?? '' })}><GitBranch size={18} /><span><strong>{w.name}</strong><small>{w.description}</small></span><span className="sp-badge">{w.versions[0] ?? '未发布'}</span>{draft.workflowId === w.id && <Check size={15} />}</button>)}
              {!data.workflows.some(w => `${w.name}${w.description}`.includes(query)) && <p className="sp-empty-small">没有匹配的工作流</p>}
            </div>
            {!data.workflows.some(w => w.versions.length) && <p className="sp-field-error">暂无已发布工作流，请先在工作流页面完成发布。</p>}{err('workflow')}
            {flow?.versions.length ? <div className="sp-flow-inputs"><label className="sp-field">固定执行版本<select value={draft.snapshot} onChange={e => patch({ snapshot: e.target.value })}>{flow.versions.map(v => <option key={v} value={v}>{v}{v === flow.versions[0] ? ' · 最新发布' : ''}</option>)}</select><small>后续发布不会自动改变此任务，升级需编辑任务。</small></label>{flow.fields.map(f => <label key={f.key} className="sp-field">{f.name}{f.required ? ' *' : '（可选）'}<input value={draft.inputs[f.key] ?? ''} placeholder={f.placeholder} onChange={e => patch({ inputs: { ...draft.inputs, [f.key]: e.target.value } })} {...invalid(f.key)} />{err(f.key)}</label>)}</div> : null}
          </>}
        </section>
      </>
      <section className="sp-section"><h3><span>03</span>关联项目 <em>{draft.kind === 'workflow' ? '必选' : '可选'}</em></h3>
        <label className="sp-field"><span className="sp-sr-only">关联项目</span><div className="sp-select-icon"><Folder size={16} /><select value={draft.projectId} onChange={e => patch({ projectId: e.target.value })} {...invalid('projectId')}><option value="">{draft.kind === 'workflow' ? '请选择运行项目' : '不关联项目'}</option>{data.projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></div>{err('projectId')}</label>
        <p className="sp-helper">{draft.kind === 'workflow' ? '项目为工作流提供文件和运行环境。同一工作流可以用于不同项目。' : draft.projectId ? '本次任务可使用所选项目的文件与工作环境。' : '独立执行任务，不使用任何项目文件。'}</p>
        {!data.projects.length && <p className="sp-field-error">暂无可用项目。直接对话可不关联项目；工作流需先创建项目。</p>}
      </section>
      <section className="sp-section"><h3><span>04</span>何时执行</h3>
        <div className="sp-two-col"><label className="sp-field">重复频率<select value={draft.rule.kind} onChange={e => rule({ kind: e.target.value as Draft['rule']['kind'] })}><option value="daily">每天</option><option value="weekdays">工作日（周一至周五）</option><option value="weekly">每周</option><option value="hours">每隔若干小时</option></select></label>
          {draft.rule.kind === 'hours' ? <label className="sp-field">间隔小时数<input type="number" min={1} max={24} value={draft.rule.hours} onChange={e => rule({ hours: Number(e.target.value) })} {...invalid('hours')} />{err('hours')}</label> : <label className="sp-field">执行时间<input type="time" value={draft.rule.time} onChange={e => rule({ time: e.target.value })} {...invalid('time')} />{err('time')}</label>}
        </div>
        {draft.rule.kind === 'weekly' && <div className="sp-weekdays" role="group" aria-label="执行星期">{[1, 2, 3, 4, 5, 6, 0].map(day => <button key={day} type="button" aria-label={`星期${weekdayNames[day]}`} aria-pressed={draft.rule.days.includes(day)} className={draft.rule.days.includes(day) ? 'selected' : ''} onClick={() => rule({ days: draft.rule.days.includes(day) ? draft.rule.days.filter(d => d !== day) : [...draft.rule.days, day] })}>{weekdayNames[day]}</button>)}{err('days')}</div>}
        <label className="sp-field">时区<select value={draft.rule.timezone} onChange={e => rule({ timezone: e.target.value })}><option value="Asia/Shanghai">Asia/Shanghai · 北京时间</option><option value="Asia/Tokyo">Asia/Tokyo · 东京</option><option value="Europe/London">Europe/London · 伦敦</option><option value="America/New_York">America/New_York · 纽约</option><option value="UTC">UTC · 协调世界时</option></select>{err('timezone')}</label>
        <div className="sp-time-preview"><div><Clock3 size={15} /><strong>接下来三次</strong></div>{times.length ? times.map((time, i) => <p key={time.toISOString()}><span>{i + 1}</span>{formatTime(time, draft.rule.timezone)}</p>) : <p>填写完整时间规则后预览</p>}<small>{draft.rule.kind === 'hours' ? '从任务创建时间起，按固定小时数间隔计算。' : '按所选时区计算；夏令时跳过的时刻不执行。'}</small></div>
      </section>
      <div className="sp-summary"><Sparkles size={17} /><div><strong>任务预览</strong><p>{summary(draft)}</p><small>原型演示 · 不会实际调度或调用 AI</small></div></div>
    </div>
    <footer className="sp-form-footer">
      {discard ? <><span>放弃未保存的修改？</span><Button type="button" variant="outline" onClick={() => setDiscard(false)}>继续编辑</Button><Button type="button" onClick={onCancel}>放弃修改</Button></> : <><Button type="button" variant="outline" onClick={() => dirty ? setDiscard(true) : onCancel()}>取消</Button><span className="sp-footer-spacer" /><Button type="submit">{task ? '保存修改' : '创建并启用'}</Button></>}
    </footer>
  </form>
}
