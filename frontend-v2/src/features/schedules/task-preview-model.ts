import { create } from 'zustand'
import { emptyDraft, type Rule } from './schedule-model'

export type SourceTask = { id: string; title: string; description: string; project: string; status: string; unavailable?: string }
export type TaskTarget = { kind: 'task'; sourceTaskId: string; supplementalInstruction: string }
export type PreviewRun = {
  id: string
  at: string
  status: 'running' | 'success' | 'failed'
  source: SourceTask
  instruction: string
  output: string
}
export type PreviewSchedule = {
  id: string
  name: string
  target: TaskTarget
  rule: Rule
  enabled: boolean
  createdAt: string
  runs: PreviewRun[]
}
export const sources: SourceTask[] = [
  {
    id: 'CLOUD-128',
    title: '官网上线检查',
    description: '检查官网页面、链接和发布配置，汇总问题及上线建议。\n逐项检查首页、产品介绍与文档入口，记录影响范围和可复现步骤。',
    project: 'Ora 官网',
    status: '进行中',
  },
  {
    id: 'CLOUD-136',
    title: '研发进展周报',
    description: '整理研发进展、未解决的问题和下一步安排，为团队生成简洁周报。',
    project: '产品研发',
    status: '待处理',
  },
  {
    id: 'CLOUD-142',
    title: '项目依赖检查',
    description: '检查项目依赖和构建状态，输出需要人工跟进的事项。',
    project: '产品研发',
    status: '已完成',
  },
  {
    id: 'CLOUD-155',
    title: '旧站点巡检',
    description: '检查旧站点运行情况。',
    project: '旧站点',
    status: '已归档',
    unavailable: '关联项目已不可用',
  },
]
const example = (): PreviewSchedule => ({
  id: 'weekly-check',
  name: '官网上线检查 · 定时执行',
  target: { kind: 'task', sourceTaskId: sources[0].id, supplementalInstruction: '优先检查发布阻塞问题。' },
  rule: { ...emptyDraft().rule, kind: 'weekly', days: [5] },
  enabled: true,
  createdAt: new Date().toISOString(),
  runs: [],
})
type State = {
  sources: SourceTask[]
  schedules: PreviewSchedule[]
  save: (schedule: PreviewSchedule) => void
  toggle: (id: string) => void
  run: (id: string, fail?: boolean) => void
  reset: () => void
}
const timers = new Set<ReturnType<typeof setTimeout>>()
/** Local demonstration only: never imports an API client or the chat store. */
export const useTaskPreview = create<State>((set, get) => ({
  sources: structuredClone(sources),
  schedules: [example()],
  save: (schedule) =>
    set((s) => ({
      schedules: s.schedules.some((x) => x.id === schedule.id)
        ? s.schedules.map((x) => (x.id === schedule.id ? schedule : x))
        : [schedule, ...s.schedules],
    })),
  toggle: (id) => set((s) => ({ schedules: s.schedules.map((x) => (x.id === id ? { ...x, enabled: !x.enabled } : x)) })),
  run: (id, fail = false) => {
    const schedule = get().schedules.find((x) => x.id === id)
    const source = get().sources.find((x) => x.id === schedule?.target.sourceTaskId)
    if (!schedule || !source || source.unavailable || schedule.runs.some((r) => r.status === 'running')) return
    const run: PreviewRun = {
      id: crypto.randomUUID(),
      at: new Date().toISOString(),
      status: 'running',
      source: structuredClone(source),
      instruction: schedule.target.supplementalInstruction,
      output: '',
    }
    set((s) => ({ schedules: s.schedules.map((x) => (x.id === id ? { ...x, runs: [run, ...x.runs] } : x)) }))
    const timer = setTimeout(() => {
      timers.delete(timer)
      set((s) => ({
        schedules: s.schedules.map((x) =>
          x.id === id
            ? {
                ...x,
                runs: x.runs.map((r) =>
                  r.id === run.id
                    ? {
                        ...r,
                        status: fail ? 'failed' : 'success',
                        output: fail
                          ? '演示失败：数据源暂时不可用，请检查后重试。'
                          : '演示结果：检查已完成，建议在正式发布前复核链接与配置。未执行真实任务或读取项目文件。',
                      }
                    : r,
                ),
              }
            : x,
        ),
      }))
    }, 1200)
    timers.add(timer)
  },
  reset: () => {
    timers.forEach(clearTimeout)
    timers.clear()
    set({ sources: structuredClone(sources), schedules: [example()] })
  },
}))

export function previewCreatedAt() {
  return new Date().toISOString()
}
