import { afterEach, describe, expect, it } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { UserContext } from '@/app/auth/user'
import { useSettingsData } from '@/features/settings/data/hooks'
import { demoUser } from '@/test/backend'
import { useRoleCatalog } from './role-hooks'

afterEach(() => localStorage.clear())

describe('role catalog', () => {
  it('shares the Settings snapshot cache in either order', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <UserContext.Provider value={demoUser}>{children}</UserContext.Provider>
      </QueryClientProvider>
    )
    const settings = renderHook(() => useSettingsData(), { wrapper })
    await waitFor(() => expect(settings.result.current.query.data?.revision).toBeDefined())
    settings.unmount()

    const roles = renderHook(() => useRoleCatalog(), { wrapper })
    expect(roles.result.current.data?.map((role) => role.name)).toEqual(['代码审查', '技术写作'])
    await waitFor(() => expect(roles.result.current.isFetching).toBe(false))
    roles.unmount()

    const again = renderHook(() => useSettingsData(), { wrapper })
    expect(again.result.current.query.data?.revision).toBeDefined()
    expect(again.result.current.query.data?.roles).toHaveLength(2)
  })
})
