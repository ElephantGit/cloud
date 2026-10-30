import { parseWorkflowVariableValueText, type WorkflowInputVariable } from '@/lib/workflows/mock'

/** Schedule view model. Backend request mapping is not implemented. */
export type Rule = { kind: 'daily' | 'weekdays' | 'weekly' | 'hours'; time: string; days: number[]; hours: number; timezone: string }
export type SourceTask = { id: string; title: string; description: string; projectId: string; projectName: string; status: string; unavailable?: string }
export type TaskTarget = { kind: 'task'; sourceTaskId: string; supplementalInstruction: string }
export type Target = TaskTarget | { kind: 'chat'; prompt: string; agent: string; model: string } | { kind: 'workflow'; workflowId: string; snapshot: string; inputs: Record<string, string> }
export type Run = { id: string; startedAt: string; status: 'running' | 'success' | 'failed'; target: Target; projectName: string; output: string; source?: SourceTask }
export type Task = { version?: number; id: string; name: string; projectId: string; target: Target; rule: Rule; enabled: boolean; createdAt: string; runs: Run[] }
export type Draft = { name: string; kind: Target['kind']; sourceTaskId: string; supplementalInstruction: string; projectId: string; prompt: string; agent: string; model: string; workflowId: string; snapshot: string; inputs: Record<string, string>; rule: Rule }
export type Catalog = { tasks?: SourceTask[]; projects: { id: string; name: string; description: string }[]; workflows: { id: string; name: string; description: string; versions: string[]; fields: { key: string; name: string; placeholder: string; required: boolean; variable?: WorkflowInputVariable }[] }[] }
export const catalog: Catalog = {
  projects: [{ id: 'website', name: 'Ora 官网', description: '网站代码与发布资料' }, { id: 'product', name: '产品研发', description: '产品文档与开发工作区' }],
  workflows: [],
}
export const weekdayNames = ['日', '一', '二', '三', '四', '五', '六']
export function emptyDraft(): Draft {
  return { name: '', kind: 'chat', sourceTaskId: '', supplementalInstruction: '', projectId: '', prompt: '', agent: '', model: '', workflowId: '', snapshot: '', inputs: {}, rule: { kind: 'daily', time: '09:00', days: [1], hours: 6, timezone: 'Asia/Shanghai' } }
}
export function taskDraft(task: Task): Draft {
  const d = emptyDraft()
  return { ...d, name: task.name, projectId: task.projectId, kind: task.target.kind, rule: structuredClone(task.rule), ...(task.target.kind !== 'workflow' ? task.target : { workflowId: task.target.workflowId, snapshot: task.target.snapshot, inputs: { ...task.target.inputs } }) }
}
export function validateDraft(d: Draft, data: Catalog = catalog): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!d.name.trim()) errors.name = '请输入任务名称'
  if (d.kind === 'chat' && !d.prompt.trim()) errors.prompt = '请告诉 AI 每次需要做什么'
  if (d.kind === 'task') {
    const source = data.tasks?.find(t => t.id === d.sourceTaskId)
    if (!source) errors.sourceTaskId = '请选择一个可用任务'
    else if (source.unavailable) errors.sourceTaskId = source.unavailable
  }
  if (d.kind === 'workflow') {
    const flow = data.workflows.find(w => w.id === d.workflowId)
    if (!flow || !flow.versions.includes(d.snapshot)) errors.workflow = '请选择已发布的工作流版本'
    if (!d.projectId) errors.projectId = '运行工作流需要项目，请选择运行环境'
    flow?.fields.forEach(f => {
      const text = d.inputs[f.key] ?? ''
      const parsed = f.variable ? parseWorkflowVariableValueText(text, f.variable.valueType) : { valid: true, value: text.trim() || undefined }
      if (!parsed.valid) errors[`input:${f.key}`] = `请输入有效的${f.name}（${f.variable?.valueType}）`
      else if (f.required && parsed.value === undefined) errors[`input:${f.key}`] = `请填写${f.name}`
      else if (f.variable?.maxLength !== undefined && typeof parsed.value === 'string' && Array.from(parsed.value).length > f.variable.maxLength) errors[`input:${f.key}`] = `${f.name}最多 ${f.variable.maxLength} 个字符`
      else if (f.variable?.options?.length && text && !f.variable.options.includes(text)) errors[`input:${f.key}`] = `请选择有效的${f.name}`
    })
  }
  if (d.kind !== 'task' && d.projectId && !data.projects.some(p => p.id === d.projectId)) errors.projectId = '该项目已不可用，请重新选择'
  if (d.rule.kind === 'weekly' && !d.rule.days.length) errors.days = '请至少选择一天'
  if (d.rule.kind !== 'hours' && !/^([01]\d|2[0-3]):[0-5]\d$/.test(d.rule.time)) errors.time = '请输入有效时间'
  if (d.rule.kind === 'hours' && (!Number.isInteger(d.rule.hours) || d.rule.hours < 1 || d.rule.hours > 24)) errors.hours = '间隔应为 1–24 小时的整数'
  try { new Intl.DateTimeFormat('zh-CN', { timeZone: d.rule.timezone }).format() } catch { errors.timezone = '请选择有效时区' }
  return errors
}
export function targetFromDraft(d: Draft, data: Catalog = catalog): Target {
  if (d.kind === 'task') return { kind: 'task', sourceTaskId: d.sourceTaskId, supplementalInstruction: d.supplementalInstruction.trim() }
  if (d.kind === 'chat') return { kind: 'chat', prompt: d.prompt.trim(), agent: d.agent, model: d.model }
  const fields = data.workflows.find(w => w.id === d.workflowId)?.fields ?? []
  return { kind: 'workflow', workflowId: d.workflowId, snapshot: d.snapshot, inputs: Object.fromEntries(fields.map(f => [f.key, d.inputs[f.key] ?? ''])) }
}
export function projectName(id: string) { return catalog.projects.find(p => p.id === id)?.name ?? '不关联项目' }
export function workflowName(id: string, data: Catalog = catalog) { return data.workflows.find(w => w.id === id)?.name ?? '工作流不可用' }
export function ruleText(rule: Rule) {
  if (rule.kind === 'hours') return `每隔 ${rule.hours} 小时`
  const prefix = rule.kind === 'daily' ? '每天' : rule.kind === 'weekdays' ? '每个工作日' : `每周${[...rule.days].sort((a,b) => ((a+6)%7)-((b+6)%7)).map(d => weekdayNames[d]).join('、')}`
  return `${prefix} ${rule.time}`
}
export function summary(d: Draft, data: Catalog = catalog) {
  if (d.kind === 'task') {
    const source = data.tasks?.find(t => t.id === d.sourceTaskId)
    return `${ruleText(d.rule)}，使用「${source?.title ?? '待选任务'}」的最新描述与项目独立执行。`
  }
  const context = d.projectId ? `在「${projectName(d.projectId)}」中` : '不关联项目，'
  return `${ruleText(d.rule)}，${context}${d.kind === 'chat' ? '新建对话执行任务指令' : `运行「${d.workflowId ? workflowName(d.workflowId, data) : '待选工作流'}」${d.snapshot ? ` ${d.snapshot}` : ''}`}。`
}
function parts(date: Date, timezone: string) {
  const p = new Intl.DateTimeFormat('en-CA', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).formatToParts(date)
  const n = (key: string) => Number(p.find(v => v.type === key)?.value)
  return { year: n('year'), month: n('month'), day: n('day'), hour: n('hour'), minute: n('minute'), second: n('second') }
}
/** Calendar schedules use wall time in the selected zone; gaps at DST are skipped. */
export function nextTimes(rule: Rule, now = new Date(), anchor = now): Date[] {
  if (rule.kind === 'hours') {
    const interval = rule.hours * 3_600_000
    if (!Number.isFinite(interval) || interval < 3_600_000) return []
    const first = anchor.getTime() + Math.max(1, Math.floor((now.getTime() - anchor.getTime()) / interval) + 1) * interval
    return [0, 1, 2].map(i => new Date(first + i * interval))
  }
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(rule.time) || (rule.kind === 'weekly' && !rule.days.length)) return []
  const local = parts(now, rule.timezone)
  const [hour, minute] = rule.time.split(':').map(Number)
  const result: Date[] = []
  for (let day = 0; day < 24 && result.length < 3; day++) {
    const calendar = new Date(Date.UTC(local.year, local.month - 1, local.day + day, hour, minute))
    const weekday = calendar.getUTCDay()
    if (rule.kind === 'weekdays' && (weekday === 0 || weekday === 6)) continue
    if (rule.kind === 'weekly' && !rule.days.includes(weekday)) continue
    let candidate = calendar.getTime()
    for (let i = 0; i < 4; i++) {
      const p = parts(new Date(candidate), rule.timezone)
      const wall = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second)
      candidate += calendar.getTime() - wall
    }
    const p = parts(new Date(candidate), rule.timezone)
    if (p.day === calendar.getUTCDate() && p.hour === hour && p.minute === minute && candidate > now.getTime()) result.push(new Date(candidate))
  }
  return result
}
export function formatTime(value: Date | string, timezone = 'Asia/Shanghai') {
  return new Intl.DateTimeFormat('zh-CN', { timeZone: timezone, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(value))
}
export function sampleTasks(now = new Date()): Task[] {
  const ago = (hours: number) => new Date(now.getTime() - hours * 3_600_000).toISOString()
  const chat: Target = { kind: 'chat', prompt: '整理过去 24 小时的重要 AI 产品动态。选出 5 条值得关注的消息，附来源和一句话解读。', agent: 'Ora Assistant', model: '演示默认模型' }
  const base = emptyDraft().rule
  const items: Task[] = [
    { id: 'morning', name: '每日 AI 简报', projectId: '', target: chat, rule: base, enabled: true, createdAt: ago(168), runs: [] },
    { id: 'summary', name: '研发进展日报', projectId: 'product', target: { ...chat, prompt: '总结今天的研发进展、未解决的问题和下一步计划。请分点列出。' }, rule: { ...base, kind: 'weekdays', time: '18:00' }, enabled: true, createdAt: ago(96), runs: [] },
  ]
  return items.map((t, i) => ({ ...t, runs: [{ id: `sample-${i}`, startedAt: ago(i === 3 ? 20 : i + 2), status: i === 3 ? 'failed' : 'success', target: structuredClone(t.target), projectName: projectName(t.projectId), output: i === 3 ? '演示失败：数据源暂时不可用。检查项目资料后可再次模拟运行。' : sampleOutput(t.target) }] }))
}
export function sampleOutput(target: Target) {
  return target.kind === 'chat' ? '本次任务已完成。\n\n01  重点信息已整理\n已根据任务指令汇总关键变化，保留后续需要跟进的事项。\n\n02  值得关注\n有两项内容建议进一步核对原始资料。\n\n03  下一步\n查看项目进展，并在下一次运行时重新检查。\n\n这是一份预设演示结果，未调用模型或读取真实资料。' : '检查完成，结果已汇总。\n\n✓ 输入参数已接收\n✓ 项目内容检查完成（模拟）\n✓ 结果摘要已生成\n\n建议：发布前确认变更范围与文档同步情况。\n\n这是预设工作流演示，未运行代码或访问项目文件。'
}
