import { useEffect, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLocation } from 'react-router'
import { useGetApiV1Me } from '@/api/me/me'
import { Button } from '@/components/ui/button'
import { statusOf } from '@/lib/api-client'
import { LoginScreen } from './login-screen'
import { rememberAuthIdentity, subscribeAuthIdentity } from './sync'
import { UserContext } from './user'

/**
 * Renders its children only for a signed-in user. `GET /api/v1/me` is the single source of truth,
 * as with Cloud's Gateway: 401 shows the login screen in place (so the current path is where the
 * login returns), 403 means the account is disabled.
 */
export function SessionGate({ children }: { children: ReactNode }) {
  const me = useGetApiV1Me({ query: { retry: false } })
  const location = useLocation()
  const queryClient = useQueryClient()
  const userId = me.data?.id

  useEffect(() => {
    if (userId) rememberAuthIdentity(userId)
  }, [userId])

  // A sign-in or sign-out in another tab changes the shared cookie. Revalidate every visible
  // query before rendering the old account's page with the new session.
  useEffect(
    () => subscribeAuthIdentity(() => void queryClient.resetQueries()),
    [queryClient],
  )

  if (me.isPending) return <Centered>加载中…</Centered>
  if (me.isError) {
    const status = statusOf(me.error)
    if (status === 401) return <LoginScreen returnTo={location.pathname + location.search} />
    if (status === 403) return <Centered>账号已停用，请联系管理员。</Centered>
    return (
      <Centered>
        <span>无法连接服务器。</span>
        <Button variant="outline" size="sm" onClick={() => me.refetch()}>重试</Button>
      </Centered>
    )
  }
  return <UserContext.Provider value={me.data}>{children}</UserContext.Provider>
}

function Centered({ children }: { children: ReactNode }) {
  return <div className="flex h-svh flex-col items-center justify-center gap-3 text-sm text-muted-foreground">{children}</div>
}
