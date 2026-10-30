import { useWorkflowLibrary, useWorkflowVersions, useWorkflowVersionSnapshot } from '@/lib/workflows/data/workflows'
import { parseWorkflowGraph } from '@/lib/workflows/runtime'
import { formatWorkflowVariableValue, resolveWorkflowInputFieldType, workflowVariableValueExample } from '@/lib/workflows/mock'
import type { Catalog, Draft } from './schedule-model'

/** Loads only the chosen published graph; drafts are never schedule targets. */
export function useScheduleWorkflow(draft: Draft, data: Catalog) {
  const library = useWorkflowLibrary()
  const versions = useWorkflowVersions(draft.kind === 'workflow' ? draft.workflowId : null)
  const snapshot = useWorkflowVersionSnapshot(draft.kind === 'workflow' ? draft.workflowId : null, draft.snapshot || null)
  let graph
  let graphError = false
  if (snapshot.data) {
    try { graph = parseWorkflowGraph(snapshot.data.graph) } catch { graphError = true }
  }
  const start = graph?.nodes.find(n => n.data.kind === 'start')
  const fields = (start?.data.inputVariables ?? []).map(variable => ({
    key: variable.name,
    name: variable.displayName?.trim() || variable.name,
    placeholder: workflowVariableValueExample(variable.valueType),
    required: variable.required === true,
    variable,
  }))
  const workflows = (library.data ?? []).map(w => ({
    id: w.id, name: w.name, description: w.publishedVersion ? '已发布工作流' : '尚未发布',
    versions: w.id === draft.workflowId ? (versions.data ?? []).map(v => v.version) : w.publishedVersion ? [w.publishedVersion] : [],
    fields: w.id === draft.workflowId ? fields : [],
  }))
  const inputs = Object.fromEntries(fields.map(f => [f.key, draft.inputs[f.key] ?? (
    resolveWorkflowInputFieldType(f.variable) === 'checkbox' && f.variable.value === undefined
      ? 'false' : formatWorkflowVariableValue(f.variable.value, f.variable.valueType)
  )]))
  const error = library.isError ? '工作流库加载失败，请重试。'
    : versions.isError || snapshot.isError ? '工作流或发布版本已不可用，请重新选择。'
    : graphError || (snapshot.data && !start) ? '发布版本无法读取开始节点，请重新选择。' : undefined
  const loading = library.isLoading || (!!draft.workflowId && (versions.isLoading || (!!draft.snapshot && snapshot.isLoading)))
  return { data: { ...data, workflows }, inputs, error, loading }
}
