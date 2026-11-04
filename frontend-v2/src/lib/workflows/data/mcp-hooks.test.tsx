import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { UserContext } from '@/app/auth/user'
import { demoUser, fakeBackend } from '@/test/backend'
import { http } from '@/lib/api-client'
import { useDemoPluginSelections } from '@/features/settings/plugins/demo-state'
import { useInstalledPlugins } from './catalog-hooks'
import { createContractsClient, createHttpWorkflowContracts } from './contracts-client'
import { ContractsClientProvider } from './contracts-client-provider'

const original = http.defaults.adapter
afterEach(() => { http.defaults.adapter = original; useDemoPluginSelections.getState().reset() })

it('updates mounted MCP pickers on Settings installation and removal without stale cache', async () => {
  fakeBackend((config) => ({ status: 200, data: { items: config.url === '/api/v1/tenants/t/spaces' ? [{ id: 's' }] : [] } }))
  const client = createContractsClient(createHttpWorkflowContracts('t'), 't')
  const query = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const wrapper = ({ children }: { children: ReactNode }) => <UserContext.Provider value={demoUser}>
    <QueryClientProvider client={query}><ContractsClientProvider client={client}>{children}</ContractsClientProvider></QueryClientProvider>
  </UserContext.Provider>
  const { result } = renderHook(useInstalledPlugins, { wrapper })
  await waitFor(() => expect(result.current.data).toEqual([]))
  act(() => useDemoPluginSelections.getState().setSelected(`${demoUser.id}:t:s`, 'demo/sample-mcp', true))
  await waitFor(() => expect(result.current.data?.map((p) => p.id)).toEqual(['demo/sample-mcp']))
  act(() => useDemoPluginSelections.getState().setSelected(`${demoUser.id}:t:s`, 'demo/sample-mcp', false))
  await waitFor(() => expect(result.current.data).toEqual([]))
})
