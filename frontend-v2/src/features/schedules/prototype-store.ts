import { create } from 'zustand'
import { projectName, sampleOutput, sampleTasks, targetFromDraft, type Draft, type Task } from './prototype-model'

type Store = { tasks: Task[]; save: (draft: Draft, id?: string) => string; toggle: (id: string) => void; remove: (id: string) => void; simulate: (id: string, fail?: boolean) => void; reset: () => void }
const pending = new Map<string, ReturnType<typeof setTimeout>>()
/** Only in-memory sample state; deliberately has no persistence or HTTP port. */
export const usePrototype = create<Store>((set, get) => ({
  tasks: sampleTasks(),
  save: (draft, id) => {
    const existing = get().tasks.find(t => t.id === id)
    const task: Task = { id: id ?? crypto.randomUUID(), createdAt: existing?.createdAt ?? new Date().toISOString(), enabled: existing?.enabled ?? true, runs: existing?.runs ?? [], name: draft.name.trim(), projectId: draft.projectId, target: targetFromDraft(draft), rule: structuredClone(draft.rule) }
    set(s => ({ tasks: existing ? s.tasks.map(t => t.id === task.id ? task : t) : [task, ...s.tasks] }))
    return task.id
  },
  toggle: id => set(s => ({ tasks: s.tasks.map(t => t.id === id ? { ...t, enabled: !t.enabled } : t) })),
  remove: id => { clearTimeout(pending.get(id)); pending.delete(id); set(s => ({ tasks: s.tasks.filter(t => t.id !== id) })) },
  simulate: (id, fail = false) => {
    const task = get().tasks.find(t => t.id === id)
    if (!task || task.runs.some(r => r.status === 'running')) return
    const runId = crypto.randomUUID()
    const target = structuredClone(task.target)
    set(s => ({ tasks: s.tasks.map(t => t.id === id ? { ...t, runs: [{ id: runId, startedAt: new Date().toISOString(), status: 'running', target, projectName: projectName(t.projectId), output: '' }, ...t.runs] } : t) }))
    pending.set(id, setTimeout(() => {
      set(s => ({ tasks: s.tasks.map(t => t.id === id ? { ...t, runs: t.runs.map(r => r.id === runId ? { ...r, status: fail ? 'failed' : 'success', output: fail ? '演示失败：项目数据源暂时不可用。请检查输入后重新模拟运行；任务的定时启用状态未改变。' : sampleOutput(target) } : r) } : t) }))
      pending.delete(id)
    }, 1600))
  },
  reset: () => { pending.forEach(clearTimeout); pending.clear(); set({ tasks: sampleTasks() }) },
}))
