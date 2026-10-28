import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { SidebarProvider } from '@/components/ui/sidebar'
import { renderRoutes, renderWithProviders } from '@/test/render'
import { server } from '@/test/msw-server'
import { UnavailablePage } from './unavailable-page'

describe('UnavailablePage', () => {
  it('explains availability without issuing demo requests or accepting work', () => {
    const requests: string[] = []
    server.use(
      http.all('*/workspaces/:slug/*', ({ request }) => {
        requests.push(request.url)
        return HttpResponse.json([])
      }),
    )
    renderWithProviders(
      <UnavailablePage
        title="示例入口"
        description="请使用已经开放的任务入口。"
        actions={[{ label: '打开任务', to: '/w/real-workspace/issues' }]}
      />,
    )
    expect(screen.getByRole('heading', { name: '示例入口暂不可用' })).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(requests).toEqual([])
  })

  it('navigates with the native link keyboard behavior', async () => {
    const user = userEvent.setup()
    const { router } = renderRoutes(
      [
        {
          path: '/w/team/squads',
          element: (
            <SidebarProvider>
              <UnavailablePage
                title="小队"
                description="查看真实成员。"
                actions={[{ label: '查看成员', to: '/w/team/settings/members' }]}
              />
            </SidebarProvider>
          ),
        },
        { path: '/w/team/settings/members', element: <h1>空间成员</h1> },
      ],
      '/w/team/squads',
    )
    const link = screen.getByRole('link', { name: '查看成员' })
    expect(link).not.toHaveAttribute('role', 'button')
    link.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('heading', { name: '空间成员' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/w/team/settings/members')
  })
})
