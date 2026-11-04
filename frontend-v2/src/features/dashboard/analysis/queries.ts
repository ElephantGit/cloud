import { useGetApiV1MeTenants } from '@/api/me/me'
import { useUser } from '@/app/auth/user'
import { statusOf } from '@/lib/api-client'

export function useAnalysisScope() {
  const user = useUser()
  const tenants = useGetApiV1MeTenants()
  return {
    user: user.id,
    tenant: tenants.data?.items[0]?.id ?? '',
    pending: tenants.isPending,
    error: tenants.isError,
  }
}

/** NotFound also hides revoked private resources; cached success must not remain visible. */
export function accessFailure(error: unknown) {
  return [401, 403, 404].includes(statusOf(error) ?? 0)
}
export function failureMessage(error: unknown) {
  return accessFailure(error)
    ? '会话不可访问，请确认登录、成员关系及来源任务。'
    : '读取失败，请刷新重试。'
}

export const sessionStatus: Record<string, string> = {
  running: '运行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
  interrupted: '已中断',
}
