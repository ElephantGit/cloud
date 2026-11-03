import { useEffect, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { isAgentRunning } from './task-views'
import { listPendingMentions, listTaskDecompositions, listTasks, type PendingMention } from './tasks-api/generated'

/** The caller's workspace: v2 signs everyone into one team, so it is the first membership. */
export function useWorkspace() {
  const tenants = useGetApiV1MeTenants()
  const current = tenants.data?.items[0]
  return { tenantId: current?.id ?? '', role: current?.role, isLoading: tenants.isLoading, isError: tenants.isError }
}

/**
 * The workspace's tasks, shared by the sidebar counts and the list. There is no push channel, so
 * the list is read again every few seconds while an Agent runs, and less often otherwise, which is
 * how one member sees another's task move without reloading.
 */
export function useTaskList(tenantId: string) {
  return useQuery({
    queryKey: ['tasks', tenantId],
    queryFn: () => listTasks(tenantId),
    enabled: !!tenantId,
    refetchInterval: query => (query.state.data?.items.some(isAgentRunning) ? 3000 : 15000),
  })
}

/**
 * The caller's mentions still waiting on their reply, by task. With no notification channel, this
 * is how an @ reaches a person: the task joins 待我处理 and its card says who is waiting.
 */
export function usePendingMentions(tenantId: string): ReadonlyMap<string, PendingMention> {
  const query = useQuery({
    queryKey: ['task-mentions', tenantId],
    queryFn: () => listPendingMentions(tenantId),
    enabled: !!tenantId,
    refetchInterval: 15000,
  })
  const items = query.data?.items
  return useMemo(() => new Map((items ?? []).map(m => [m.taskId, m])), [items])
}

/**
 * The task's decompositions, newest first. Nothing pushes a run's end, so the list is read again
 * every two seconds while one runs; when it settles the team's task list is read again too, which
 * is how the new subtasks reach the board.
 */
export function useDecompositions(tenantId: string, taskId: string) {
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['task-decompositions', tenantId, taskId],
    queryFn: () => listTaskDecompositions(tenantId, taskId),
    enabled: !!tenantId && !!taskId,
    refetchInterval: q => (q.state.data?.items[0]?.status === 'running' ? 2000 : false),
  })
  const latest = query.data?.items[0]
  const settled = latest ? `${latest.id}:${latest.status}` : ''
  useEffect(() => {
    if (settled) void client.invalidateQueries({ queryKey: ['tasks', tenantId] })
  }, [settled, client, tenantId])
  return { latest, isError: query.isError }
}
