import { createMockWorkflowNode } from '@/lib/workflows/mock/node-factory'
import type { GraphWorkflowRun, WorkflowDefinition } from '@/lib/workflows/runtime'
import { demoMockEnabled } from './flags'

// Replaced by an installed cdase workflow and real execution. This module contains data only;
// the normal workflow picker, RunPane, Overview and Theater own every visible control.
export const cdaseInput = '评审退款申请接口：识别重复提交和退款失败的风险，给出预防措施与验证清单。'
export const cdaseWorkflow = { id: 'demo-cdase-sfmea', name: 'cdase · SFMEA 评审', version: 'v1', snapshotId: 'demo-cdase-sfmea-v1' }
export const cdaseReport = [
  '退款申请接口 · SFMEA 评审报告',
  '结论：建议补齐风险控制措施后实现。以下是评审建议，尚未整改或执行测试。',
  '',
  '1. 重复提交导致重复退款',
  '原因：请求重试缺少业务幂等约束；影响：重复资金支出。',
  '措施：以订单和退款单标识建立幂等约束，重复请求返回原结果。',
  '待验证：并发提交同一退款单，确认只产生一次退款。',
  '',
  '2. 渠道超时后状态不明确',
  '原因：将超时直接当成失败；影响：重试造成重复处理或退款遗漏。',
  '措施：保留处理中状态，通过渠道查询对账后再推进状态。',
  '待验证：模拟渠道超时和延迟回执，核对最终状态与资金记录。',
  '',
  '本次仅输出文本评审结论，未修改仓库文件。',
].join('\n')

const titles = ['评审输入', '理解需求与边界', '识别失效模式', '评估风险与措施', '输出评审结论']
const outputs = [cdaseInput, '范围：退款提交、渠道响应、失败恢复。检查目标：重复支出与退款遗漏。', '发现两项失效风险：重复提交导致重复退款；渠道超时后状态不明确。', '建议增加退款单幂等约束、渠道查询对账，并补齐并发和超时回执验证。', cdaseReport]

/** The same frozen definition model the real workflow run views consume. */
export function cdaseDefinition(): WorkflowDefinition {
  const nodes = titles.map((title, i) => {
    const node = createMockWorkflowNode({ kind: i === 0 ? 'start' : i === titles.length - 1 ? 'output' : 'agent', sequence: i, position: { x: i * 330, y: 100 }, locale: 'zh-CN' })
    node.id = `cdase-${i}`
    node.data.title = title
    if (i === 0) node.data.input = cdaseInput
    if (node.data.kind === 'agent') node.data.instruction = outputs[i]
    return node
  })
  return { id: cdaseWorkflow.id, name: cdaseWorkflow.name, description: '使用团队 SFMEA 方法评审退款接口的研发风险。', updatedAt: '2026-11-06T00:00:00Z', viewport: { x: 0, y: 0, zoom: 1 }, nodes, edges: nodes.slice(1).map((node, i) => ({ id: `cdase-edge-${i}`, source: nodes[i]!.id, target: node.id, type: 'workflow' })) }
}

interface Entry { tenantId: string; run: GraphWorkflowRun; startedAt?: number }
const entries = new Map<string, Entry>()

/** Only reserved demo run IDs opt out of server requests and SSE. */
export const isCdaseRun = (id: string): boolean => demoMockEnabled() && id.startsWith('demo-cdase-run-')

/** Creates isolated mock data through the production run dialog. Nothing is persisted remotely. */
export function createCdaseRun(tenantId: string, projectId: string, name: string, now = Date.now()): string {
  if (!demoMockEnabled()) throw new Error('演示未启用')
  const id = `demo-cdase-run-${crypto.randomUUID()}`
  const definition = cdaseDefinition()
  entries.set(id, { tenantId, run: { id, projectId, definitionId: cdaseWorkflow.id, snapshotId: cdaseWorkflow.snapshotId, definitionSnapshot: definition, name: name.trim() || cdaseWorkflow.name, status: 'pending', nodeStates: Object.fromEntries(definition.nodes.map(n => [n.id, { status: 'idle' }])), openHitls: [], createdAt: new Date(now).toISOString(), updatedAt: new Date(now).toISOString() } })
  return id
}

/** Reads a tenant-scoped fixture and advances its deterministic states through ordinary polling. */
export function readCdaseRun(tenantId: string, id: string, now = Date.now()): GraphWorkflowRun {
  const entry = entries.get(id)
  if (!isCdaseRun(id) || !entry || entry.tenantId !== tenantId) throw new Error('找不到此演示运行')
  const run = structuredClone(entry.run)
  if (entry.startedAt !== undefined && run.status === 'running') {
    const current = Math.min(5, Math.floor((now - entry.startedAt) / 1800))
    for (const [i, node] of run.definitionSnapshot.nodes.entries()) {
      run.nodeStates[node.id] = { status: i < current ? 'succeeded' : i === current ? 'running' : 'idle', ...(i < current ? { output: { summary: titles[i]!, detail: outputs[i]! } } : {}) }
    }
    if (current === 5) { run.status = 'succeeded'; run.finalOutput = cdaseReport; run.finishedAt = new Date(now).toISOString() }
    run.updatedAt = new Date(now).toISOString()
  }
  return run
}

/** Returns only this tenant/project's local runs alongside the server-backed project history. */
export function cdaseProjectRuns(tenantId: string, projectId: string) {
  if (!demoMockEnabled()) return []
  return [...entries.values()].filter(e => e.tenantId === tenantId && e.run.projectId === projectId).map(e => readCdaseRun(tenantId, e.run.id))
}

/** Edits only the supported example, so fixed review conclusions never answer another task. */
export function updateCdaseInput(tenantId: string, id: string, input: string): void {
  const run = readCdaseRun(tenantId, id)
  if (run.status !== 'pending') throw new Error('运行已开始')
  if (input.trim() !== cdaseInput) throw new Error('此演示仅支持预置退款评审任务')
  const entry = entries.get(id)!
  entry.run.kickoffInput = input
  entry.run.definitionSnapshot.nodes[0]!.data.input = input
}

/** Starts mock progression without invoking an Agent or changing the UI implementation. */
export function startCdaseRun(tenantId: string, id: string, input: string, now = Date.now()): void {
  updateCdaseInput(tenantId, id, input)
  const entry = entries.get(id)!
  entry.startedAt = now
  entry.run.status = 'running'
}

/** Mirrors cancel/delete semantics for the local fixture. */
export function endCdaseRun(tenantId: string, id: string, remove: boolean): void {
  const run = readCdaseRun(tenantId, id)
  if (remove) { if (run.status === 'running') throw new Error('运行尚未结束'); entries.delete(id) }
  else { if (run.status !== 'pending' && run.status !== 'running') throw new Error('运行已结束'); entries.get(id)!.run.status = 'cancelled' }
}

/** The real workspace panel also receives data only: this text-only review changes no files. */
export function cdaseWorkspaceReader(tenantId: string, id: string) {
  if (!isCdaseRun(id)) return undefined
  const missingFile = async (): Promise<never> => { throw new Error('本次文本评审没有仓库文件') }
  return {
    key: ['workspace', 'run', tenantId, id],
    entries: async (path: string) => { readCdaseRun(tenantId, id); return { path, entries: [], truncated: false } },
    changes: async () => { readCdaseRun(tenantId, id); return { baseCommit: '', files: [], truncated: false } },
    file: missingFile,
    diff: missingFile,
  }
}
