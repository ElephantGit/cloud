import { useQuery } from '@tanstack/react-query'
import { useGetApiV1MeTenants } from '@/api/me/me'
import { isAgentRunning } from './task-views'
import { listTasks } from './tasks-api/generated'

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
