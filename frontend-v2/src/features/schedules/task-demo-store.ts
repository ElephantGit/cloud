import { create } from 'zustand'
import { catalog, targetFromDraft, validateDraft, type Catalog, type Draft, type Run, type SourceTask, type Task } from './schedule-model'

/** Sample task catalog until Cloud v2 exposes its task directory. These are never presented as live records. */
export const demoSources: SourceTask[] = [
  {
    id: 'CLOUD-128',
    title: '官网上线检查',
    description: '检查官网页面、链接和发布配置，汇总问题及上线建议。\n逐项检查首页、产品介绍与文档入口，记录影响范围和可复现步骤。',
    projectId: 'website',
    projectName: 'Ora 官网',
    status: '进行中',
  },
  {
    id: 'CLOUD-136',
    title: '研发进展周报',
    description: '整理研发进展、未解决的问题和下一步安排，为团队生成简洁周报。',
    projectId: 'product',
    projectName: '产品研发',
    status: '待处理',
  },
  {
    id: 'CLOUD-142',
    title: '项目依赖检查',
    description: '检查项目依赖和构建状态，输出需要人工跟进的事项。',
    projectId: 'product',
    projectName: '产品研发',
    status: '已完成',
  },
  {
    id: 'CLOUD-155',
    title: '旧站点巡检',
    description: '检查旧站点运行情况。',
    projectId: 'legacy',
    projectName: '旧站点',
    status: '已归档',
    unavailable: '关联项目已不可用',
  },
]

export const emptyDemoSchedules: Task[] = []
type State = {
  sources: SourceTask[]
  schedules: Record<string, Task[]>
  save: (scope: string, draft: Draft, id?: string, data?: Catalog) => string | undefined
  toggle: (scope: string, id: string) => void
  run: (scope: string, id: string, fail?: boolean) => void
  reset: () => void
}
const timers = new Set<ReturnType<typeof setTimeout>>()

/** In-memory task demonstrations, scoped to user and tenant, independent of API and chat state. */
export const useTaskScheduleDemo = create<State>((set, get) => ({
  sources: structuredClone(demoSources),
  schedules: {},
  save: (scope, draft, id, data = catalog) => {
    const { sources, schedules } = get()
    const source = sources.find((s) => s.id === draft.sourceTaskId)
    const existing = schedules[scope]?.find((t) => t.id === id)
    if (
      draft.kind === 'chat' ||
      (draft.kind === 'task' && !source) ||
      (id && !existing) ||
      Object.keys(validateDraft(draft, { ...data, tasks: sources })).length
    )
      return
    const task: Task = {
      id: id ?? `demo-${crypto.randomUUID()}`,
      name: draft.name.trim(),
      projectId: draft.kind === 'task' ? source!.projectId : draft.projectId,
      target: targetFromDraft(draft, data),
      rule: structuredClone(draft.rule),
      enabled: existing?.enabled ?? true,
      createdAt: existing?.createdAt ?? new Date().toISOString(),
      runs: existing?.runs ?? [],
    }
    set((s) => ({
      schedules: {
        ...s.schedules,
        [scope]: existing ? s.schedules[scope].map((t) => (t.id === id ? task : t)) : [task, ...(s.schedules[scope] ?? [])],
      },
    }))
    return task.id
  },
  toggle: (scope, id) =>
    set((s) => ({
      schedules: { ...s.schedules, [scope]: (s.schedules[scope] ?? []).map((t) => (t.id === id ? { ...t, enabled: !t.enabled } : t)) },
    })),
  run: (scope, id, fail = false) => {
    const task = get().schedules[scope]?.find((t) => t.id === id)
    const target = task?.target
    if (!task || target?.kind !== 'task' || task.runs.some((r) => r.status === 'running')) return
    const source = get().sources.find((s) => s.id === target.sourceTaskId)
    if (!source || source.unavailable) return
    const run: Run = {
      id: crypto.randomUUID(),
      startedAt: new Date().toISOString(),
      status: 'running',
      target: structuredClone(target),
      source: structuredClone(source),
      projectName: source.projectName,
      output: '',
    }
    set((s) => ({
      schedules: { ...s.schedules, [scope]: s.schedules[scope].map((t) => (t.id === id ? { ...t, runs: [run, ...t.runs] } : t)) },
    }))
    const timer = setTimeout(() => {
      timers.delete(timer)
      set((s) => ({
        schedules: {
          ...s.schedules,
          [scope]: (s.schedules[scope] ?? []).map((t) =>
            t.id === id
              ? {
                  ...t,
                  runs: t.runs.map((r) =>
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
              : t,
          ),
        },
      }))
    }, 1200)
    timers.add(timer)
  },
  reset: () => {
    timers.forEach(clearTimeout)
    timers.clear()
    set({ sources: structuredClone(demoSources), schedules: {} })
  },
}))
