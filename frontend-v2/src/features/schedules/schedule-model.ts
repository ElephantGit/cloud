/** Schedule view model. Backend request mapping is not implemented. */
export type Rule = { kind: 'daily' | 'weekdays' | 'weekly' | 'hours'; time: string; days: number[]; hours: number; timezone: string }
export type Target = { kind: 'chat'; prompt: string; agent: string; model: string } | { kind: 'workflow'; workflowId: string; snapshot: string; inputs: Record<string, string> }
export type Run = { id: string; startedAt: string; status: 'running' | 'success' | 'failed'; target: Target; projectName: string; output: string }
export type Task = { version?: number; id: string; name: string; projectId: string; target: Target; rule: Rule; enabled: boolean; createdAt: string; runs: Run[] }
export type Draft = { name: string; kind: Target['kind']; projectId: string; prompt: string; agent: string; model: string; workflowId: string; snapshot: string; inputs: Record<string, string>; rule: Rule }
export type Catalog = { projects: { id: string; name: string; description: string }[]; workflows: { id: string; name: string; description: string; versions: string[]; fields: { key: string; name: string; placeholder: string; required: boolean }[] }[] }
export const catalog: Catalog = {
  projects: [{ id: 'website', name: 'Ora 官网', description: '网站代码与发布资料' }, { id: 'product', name: '产品研发', description: '产品文档与开发工作区' }],
  workflows: [
    { id: 'release', name: '发布检查', description: '检查变更、运行检查清单，整理发布建议。', versions: ['v3', 'v2'], fields: [{ key: 'branch', name: '检查分支', placeholder: '例如 main', required: true }, { key: 'focus', name: '重点关注', placeholder: '例如兼容性与文档变更', required: false }] },
    { id: 'weekly', name: '项目周报', description: '收集本周进展，汇总风险与下一步计划。', versions: ['v1'], fields: [{ key: 'audience', name: '报告对象', placeholder: '例如产品与研发团队', required: true }] },
    { id: 'draft', name: '竞品观察', description: '整理竞品动态，尚未发布。', versions: [], fields: [] },
  ],
}
export const weekdayNames = ['日', '一', '二', '三', '四', '五', '六']
export function emptyDraft(): Draft {
  return { name: '', kind: 'chat', projectId: '', prompt: '', agent: '', model: '', workflowId: '', snapshot: '', inputs: {}, rule: { kind: 'daily', time: '09:00', days: [1], hours: 6, timezone: 'Asia/Shanghai' } }
}
export function taskDraft(task: Task): Draft {
  const d = emptyDraft()
  return { ...d, name: task.name, projectId: task.projectId, kind: task.target.kind, rule: structuredClone(task.rule), ...(task.target.kind === 'chat' ? task.target : { workflowId: task.target.workflowId, snapshot: task.target.snapshot, inputs: { ...task.target.inputs } }) }
}
export function validateDraft(d: Draft, data: Catalog = catalog): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!d.name.trim()) errors.name = '请输入任务名称'
  if (d.kind === 'chat' && !d.prompt.trim()) errors.prompt = '请告诉 AI 每次需要做什么'
  if (d.kind === 'workflow') {
    const flow = data.workflows.find(w => w.id === d.workflowId)
    if (!flow || !flow.versions.includes(d.snapshot)) errors.workflow = '请选择已发布的工作流版本'
    if (!d.projectId) errors.projectId = '运行工作流需要项目，请选择运行环境'
    flow?.fields.forEach(f => { if (f.required && !d.inputs[f.key]?.trim()) errors[f.key] = `请填写${f.name}` })
  }
  if (d.projectId && !data.projects.some(p => p.id === d.projectId)) errors.projectId = '该项目已不可用，请重新选择'
  if (d.rule.kind === 'weekly' && !d.rule.days.length) errors.days = '请至少选择一天'
  if (d.rule.kind !== 'hours' && !/^([01]\d|2[0-3]):[0-5]\d$/.test(d.rule.time)) errors.time = '请输入有效时间'
  if (d.rule.kind === 'hours' && (!Number.isInteger(d.rule.hours) || d.rule.hours < 1 || d.rule.hours > 24)) errors.hours = '间隔应为 1–24 小时的整数'
  try { new Intl.DateTimeFormat('zh-CN', { timeZone: d.rule.timezone }).format() } catch { errors.timezone = '请选择有效时区' }
  return errors
}
export function targetFromDraft(d: Draft): Target {
  if (d.kind === 'chat') return { kind: 'chat', prompt: d.prompt.trim(), agent: d.agent, model: d.model }
  const fields = catalog.workflows.find(w => w.id === d.workflowId)?.fields ?? []
  return { kind: 'workflow', workflowId: d.workflowId, snapshot: d.snapshot, inputs: Object.fromEntries(fields.map(f => [f.key, d.inputs[f.key]?.trim() ?? ''])) }
}
export function projectName(id: string) { return catalog.projects.find(p => p.id === id)?.name ?? '不关联项目' }
export function workflowName(id: string) { return catalog.workflows.find(w => w.id === id)?.name ?? '工作流不可用' }
export function ruleText(rule: Rule) {
  if (rule.kind === 'hours') return `每隔 ${rule.hours} 小时`
  const prefix = rule.kind === 'daily' ? '每天' : rule.kind === 'weekdays' ? '每个工作日' : `每周${[...rule.days].sort((a,b) => ((a+6)%7)-((b+6)%7)).map(d => weekdayNames[d]).join('、')}`
  return `${prefix} ${rule.time}`
}
export function summary(d: Draft) {
  const context = d.projectId ? `在「${projectName(d.projectId)}」中` : '不关联项目，'
  return `${ruleText(d.rule)}，${context}${d.kind === 'chat' ? '新建对话执行任务指令' : `运行「${d.workflowId ? workflowName(d.workflowId) : '待选工作流'}」${d.snapshot ? ` ${d.snapshot}` : ''}`}。`
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
    { id: 'release', name: '每周发布检查', projectId: 'website', target: { kind: 'workflow', workflowId: 'release', snapshot: 'v3', inputs: { branch: 'main', focus: '兼容性、文档与发布准备' } }, rule: { ...base, kind: 'weekly', days: [1], time: '10:00' }, enabled: true, createdAt: ago(72), runs: [] },
    { id: 'report', name: '产品周报', projectId: 'product', target: { kind: 'workflow', workflowId: 'weekly', snapshot: 'v1', inputs: { audience: '产品与研发团队' } }, rule: { ...base, kind: 'weekly', days: [5], time: '17:00' }, enabled: false, createdAt: ago(48), runs: [] },
  ]
  return items.map((t, i) => ({ ...t, runs: [{ id: `sample-${i}`, startedAt: ago(i === 3 ? 20 : i + 2), status: i === 3 ? 'failed' : 'success', target: structuredClone(t.target), projectName: projectName(t.projectId), output: i === 3 ? '演示失败：数据源暂时不可用。检查项目资料后可再次模拟运行。' : sampleOutput(t.target) }] }))
}
export function sampleOutput(target: Target) {
  return target.kind === 'chat' ? '本次任务已完成。\n\n01  重点信息已整理\n已根据任务指令汇总关键变化，保留后续需要跟进的事项。\n\n02  值得关注\n有两项内容建议进一步核对原始资料。\n\n03  下一步\n查看项目进展，并在下一次运行时重新检查。\n\n这是一份预设演示结果，未调用模型或读取真实资料。' : '检查完成，结果已汇总。\n\n✓ 输入参数已接收\n✓ 项目内容检查完成（模拟）\n✓ 结果摘要已生成\n\n建议：发布前确认变更范围与文档同步情况。\n\n这是预设工作流演示，未运行代码或访问项目文件。'
}
