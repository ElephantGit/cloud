import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { beforeEach, expect, it, vi } from 'vitest'
import { UserContext } from '@/app/auth/user'
import type { User } from '@/api/generated.schemas'
import * as squadsApi from '@/lib/squads-api/generated'
import type { SquadSummary } from '@/lib/squads-api/generated'
import { collaboration } from '.'
import CollaborationLayout from './page'

vi.mock('@/api/me/me', () => ({ useGetApiV1MeTenants: () => ({ data: { items: [{ id: 'tenant', role: 'owner' }] }, isLoading: false, isError: false }) }))
vi.mock('./tasks-api/generated', () => ({
  listTasks: vi.fn().mockResolvedValue({ items: [], nextCursor: '' }),
  listPendingMentions: vi.fn().mockResolvedValue({ items: [] }),
}))
vi.mock('@/lib/squads-api/generated', () => ({ listSquads: vi.fn() }))

const me: User = { id: 'user-me', displayName: '协作空间 Owner', status: 'active', version: 1, createdAt: '2026-10-01T00:00:00Z', deletedAt: null }
const squad: SquadSummary = {
  id: 'squad-1', name: '产品小队', description: '', charter: '', leaderAgentId: null, archivedAt: null, version: 1,
  createdBy: 'user-me', createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  humanCount: 1, agentCount: 0, maintainerCount: 1,
}

beforeEach(() => vi.mocked(squadsApi.listSquads).mockResolvedValue({ items: [squad], nextCursor: '' }))

// The router renders once the lazy board route has loaded, which is slow on a busy test machine.
const routed = { timeout: 10_000 }

function show(path: string) {
  const router = createMemoryRouter([
    { path: '/collaboration', Component: CollaborationLayout, children: collaboration.children },
    { path: '/settings/squads', Component: () => <p>设置小队页</p> },
  ], { initialEntries: [path] })
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <UserContext.Provider value={me}><RouterProvider router={router} /></UserContext.Provider>
  </QueryClientProvider>)
  return router
}

it('lists squads only as task filters; managing them lives in settings', async () => {
  show('/collaboration?view=all')
  const squads = await screen.findByRole('navigation', { name: '小队' }, routed)
  expect(await within(squads).findByRole('link', { name: /产品小队/ })).toHaveAttribute('href', '/collaboration?view=all&squad=squad-1')
  expect(screen.queryByText('管理小队')).not.toBeInTheDocument()
  expect(within(squads).queryByRole('link', { name: '在设置中创建小队' })).not.toBeInTheDocument()
})

it('points to settings when the workspace has no squads yet', async () => {
  vi.mocked(squadsApi.listSquads).mockResolvedValue({ items: [], nextCursor: '' })
  show('/collaboration?view=all')
  expect(await screen.findByRole('link', { name: '在设置中创建小队' }, routed)).toHaveAttribute('href', '/settings/squads')
})

it('sends the old squads address to settings with the same squad', async () => {
  const router = show('/collaboration/squads?squad=squad-1')
  expect(await screen.findByText('设置小队页', {}, routed)).toBeInTheDocument()
  expect(router.state.location.search).toBe('?squad=squad-1')
})
