import { useMutation } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { localizeContractError } from '@/lib/workflows/host/contract-error'
import { toast } from '@/lib/workflows/host/ui'
import { submitDemoHitl, updateDemoRunInput } from './demo-runs'

// The desktop hooks the copied Theater components call, over the in-browser demo engine. Each
// keeps desktop's name and call shape so the copied files change only their import path.

export { agentLabel, type AgentEntry, useAgentCatalog } from '@/lib/workflows/data/use-agent-catalog'
export { useAgents, useInstalledPlugins, useSkills } from '@/lib/workflows/data/catalog-hooks'

/** Shows a failed action as a toast, titled by the action when the caller names it. */
export function useContractErrorToast(): (error: unknown, title?: string) => void {
  const { t } = useTranslation()
  return useCallback(
    (error: unknown, title?: string) => {
      const message = localizeContractError(error, t)
      toast.error(title ?? message, { description: title === undefined ? undefined : message })
    },
    [t],
  )
}

/** Saves the Start node's prompt and values edited in Theater's inspector. */
export function useUpdateWorkflowRunInput() {
  return useMutation({
    mutationFn: ({ runId, input, variables }: { runId: string; input: string; variables?: Record<string, unknown> }) =>
      updateDemoRunInput(runId, input, variables),
  })
}

/** Answers a human-in-the-loop gate. */
export function useSubmitGraphWorkflowHitl() {
  return useMutation({
    mutationFn: ({ runId, requestId, payload }: { runId: string; requestId: string; payload: Record<string, unknown> }) =>
      submitDemoHitl(runId, requestId, payload),
  })
}

/** AI diagnosis needs a model; the demo has none, so the inspector never offers it. */
export function useDiagnoseWorkflowNodeFailure() {
  return useMutation({
    mutationFn: async (_input: { runId: string; nodeId: string }): Promise<void> => {
      throw new Error('演示运行不支持 AI 诊断')
    },
  })
}

interface DemoSelection {
  selection: { workflowRunId: string | null }
}

/**
 * Desktop's inspector reads the selected run only to offer AI diagnosis. Reporting none keeps
 * that button hidden in the demo.
 */
export function useWorkspaceSelectionStore<T>(selector: (state: DemoSelection) => T): T {
  return selector({ selection: { workflowRunId: null } })
}

/** The run's file changes would open the review pane, which the chat page does not have. */
export function useTaskChangesNavigation(): { openDiff: (path: string) => void } | null {
  return null
}

/** Shows an Agent's absolute worktree path relative to the worktree, as desktop's diff list does. */
export function displayPath(path: string): string {
  const managedWorktreePath = path.match(/(?:^|[\\/])worktrees[\\/][^\\/]+[\\/](.+)$/)?.[1]
  return managedWorktreePath?.replaceAll('\\', '/') ?? path
}
