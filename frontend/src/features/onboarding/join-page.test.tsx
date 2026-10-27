import { screen, waitFor } from '@testing-library/react'
import { useQueryClient } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { JoinContinue, JoinPage } from '@/features/onboarding/join-page'
import { SESSION_QUERY_KEY } from '@/features/auth/session'
import { PENDING_JOIN_PATH_KEY } from '@/lib/paths'
import {
  installSignedInSession,
  TEST_SPACE_ID,
  TEST_TENANT_ID,
  TEST_USER_ID,
} from '@/test/cloud-handlers'
import { renderRoutes } from '@/test/render'
import { server } from '@/test/msw-server'

const TOKEN = 'a'.repeat(43)

function renderJoin(kind: 'invite' | 'apply', token = TOKEN) {
  return renderRoutes(
    [
      { path: '/join/invite/:token', element: <JoinPage kind="invite" /> },
      { path: '/join/apply/:token', element: <JoinPage kind="apply" /> },
      { path: '/onboarding', element: <div>Onboarding screen</div> },
      { path: '/login', element: <div>Login screen</div> },
      { path: '/w/:workspaceSlug/issues', element: <div>Issues screen</div> },
    ],
    `/join/${kind}/${token}`,
  )
}

function space() {
  return {
    id: TEST_SPACE_ID,
    tenantId: TEST_TENANT_ID,
    name: 'Team',
    slug: 'team',
    role: 'member',
    description: '',
    createdBy: TEST_USER_ID,
    version: 1,
    createdAt: '2026-09-20T10:00:00Z',
    updatedAt: '2026-09-20T10:00:00Z',
    archivedAt: null,
  }
}

describe('JoinPage', () => {
  beforeEach(() =>
    server.use(
      http.post('/api/v1/join/links/preview', async ({ request }) => {
        const body = await request.json()
        return HttpResponse.json({
          kind: typeof body === 'object' && body && 'kind' in body ? body['kind'] : 'invite',
          state: 'active',
          name: 'Team',
          slug: 'team',
        })
      }),
    ),
  )
  it('restores a private link after external login from browser tab storage', async () => {
    installSignedInSession()
    sessionStorage.setItem(PENDING_JOIN_PATH_KEY, `/join/invite/${TOKEN}`)
    server.use(
      http.get('/api/v1/me/spaces', () => HttpResponse.json({ items: [], nextCursor: '' })),
    )
    renderRoutes(
      [
        { path: '/join/continue', element: <JoinContinue /> },
        { path: '/join/invite/:token', element: <JoinPage kind="invite" /> },
      ],
      '/join/continue',
    )
    expect(await screen.findByRole('button', { name: '确认加入' })).toBeInTheDocument()
    sessionStorage.removeItem(PENDING_JOIN_PATH_KEY)
  })

  it('returns to onboarding when no private link survived login', async () => {
    sessionStorage.removeItem(PENDING_JOIN_PATH_KEY)
    renderRoutes(
      [
        { path: '/join/continue', element: <JoinContinue /> },
        { path: '/onboarding', element: <div>Onboarding screen</div> },
      ],
      '/join/continue',
    )
    expect(await screen.findByText('Onboarding screen')).toBeInTheDocument()
  })

  it('redeems a single-use invitation and enters the joined tenant', async () => {
    installSignedInSession()
    let joined = false
    let submitted: unknown
    server.use(
      http.get('/api/v1/me/spaces', () =>
        HttpResponse.json({ items: joined ? [space()] : [], nextCursor: '' }),
      ),
      http.post('/api/v1/join/invitations/redeem', async ({ request }) => {
        submitted = await request.json()
        joined = true
        return HttpResponse.json({
          tenantId: TEST_TENANT_ID,
          userId: TEST_USER_ID,
          name: 'Team',
          role: 'member',
          status: 'active',
          version: 1,
        })
      }),
    )
    const user = userEvent.setup()
    renderJoin('invite')
    await user.click(await screen.findByRole('button', { name: '确认加入' }))
    await waitFor(() => expect(submitted).toEqual({ token: TOKEN }))
    await user.click(await screen.findByRole('button', { name: '进入工作区' }))
    expect(await screen.findByText('Issues screen')).toBeInTheDocument()
  })

  it('submits an application without granting immediate space access', async () => {
    installSignedInSession()
    server.use(
      http.get('/api/v1/me/spaces', () => HttpResponse.json({ items: [], nextCursor: '' })),
      http.post('/api/v1/join/requests', async () => {
        await delay(50)
        return HttpResponse.json(
          {
            id: 'r',
            tenantId: TEST_TENANT_ID,
            userId: TEST_USER_ID,
            linkId: 'l',
            status: 'pending',
            version: 1,
          },
          { status: 201 },
        )
      }),
    )
    const user = userEvent.setup()
    renderJoin('apply')
    await user.click(await screen.findByRole('button', { name: '提交申请' }))
    expect(screen.getByRole('button', { name: '正在处理…' })).toBeDisabled()
    expect(await screen.findByText('申请已提交，等待管理员审批')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '进入工作区' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '查看我的工作区' }))
    expect(await screen.findByText('Onboarding screen')).toBeInTheDocument()
  })

  it('rejects malformed links and reports revoked invitations', async () => {
    installSignedInSession()
    server.use(
      http.get('/api/v1/me/spaces', () => HttpResponse.json({ items: [], nextCursor: '' })),
    )
    renderJoin('invite', 'short')
    expect(await screen.findByText('这条链接暂时无法使用')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认加入' })).not.toBeInTheDocument()
  })

  it('keeps a revoked invitation visible as an actionable fault', async () => {
    installSignedInSession()
    server.use(
      http.get('/api/v1/me/spaces', () => HttpResponse.json({ items: [], nextCursor: '' })),
      http.post('/api/v1/join/invitations/redeem', () =>
        HttpResponse.json(
          { code: 'join_link_unavailable', params: { reason: 'revoked' }, requestId: 'r' },
          { status: 404 },
        ),
      ),
    )
    const user = userEvent.setup()
    renderJoin('invite')
    await user.click(await screen.findByRole('button', { name: '确认加入' }))
    expect(await screen.findByText('这条链接已被管理员撤销')).toBeInTheDocument()
  })
})

describe('Join link inspection', () => {
  it('clears successful admission when the signed-in account changes', async () => {
    let switched = false
    server.use(
      http.get('/api/v1/me', () =>
        HttpResponse.json({
          id: switched ? '99999999-9999-4999-8999-999999999999' : TEST_USER_ID,
          displayName: 'Test person',
          status: 'active',
          version: 1,
          createdAt: '2026-10-27T00:00:00Z',
        }),
      ),
      http.post('/api/v1/join/links/preview', () =>
        HttpResponse.json({
          kind: 'invite',
          state: 'active',
          name: 'Identity test',
          slug: 'identity-test',
        }),
      ),
      http.post('/api/v1/join/invitations/redeem', () =>
        HttpResponse.json({
          userId: TEST_USER_ID,
          name: 'Identity test',
          tenantId: TEST_TENANT_ID,
          role: 'member',
          status: 'active',
          version: 1,
        }),
      ),
    )
    function IdentityRefresh() {
      const client = useQueryClient()
      return (
        <button
          onClick={() => {
            switched = true
            void client.invalidateQueries({ queryKey: SESSION_QUERY_KEY })
          }}
        >
          Switch test account
        </button>
      )
    }
    renderRoutes(
      [
        {
          path: '/join/invite/:token',
          element: (
            <>
              <IdentityRefresh />
              <JoinPage kind="invite" />
            </>
          ),
        },
      ],
      `/join/invite/${TOKEN}`,
    )
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: '确认加入' }))
    await waitFor(() => expect(screen.getByText('你已加入此工作区')).toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: 'Switch test account' }))
    expect(await screen.findByRole('button', { name: '确认加入' })).toBeInTheDocument()
    expect(screen.queryByText('你已加入此工作区')).not.toBeInTheDocument()
  })

  it('retries the session probe when link inspection succeeded but sign-in status failed', async () => {
    let attempts = 0
    server.use(
      http.get('/api/v1/me', () => {
        attempts += 1
        return HttpResponse.json(
          { code: attempts === 1 ? 'upstream_unavailable' : 'unauthenticated' },
          { status: attempts === 1 ? 503 : 401 },
        )
      }),
      http.post('/api/v1/join/links/preview', () =>
        HttpResponse.json({ kind: 'invite', state: 'active' }),
      ),
    )
    renderJoin('invite')
    expect(await screen.findByText('暂时无法确认登录状态，请重试')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('button', { name: '登录后确认加入' })).toBeInTheDocument()
    expect(attempts).toBe(2)
  })
  it('does not carry a successful submission into a different bearer route', async () => {
    installSignedInSession()
    server.use(
      http.post('/api/v1/join/links/preview', () =>
        HttpResponse.json({ kind: 'invite', state: 'active', name: 'Team', slug: 'team' }),
      ),
      http.post('/api/v1/join/invitations/redeem', () =>
        HttpResponse.json({
          tenantId: TEST_TENANT_ID,
          userId: TEST_USER_ID,
          name: 'Team',
          role: 'member',
          status: 'active',
          version: 1,
        }),
      ),
    )
    const { router } = renderJoin('invite')
    await userEvent.setup().click(await screen.findByRole('button', { name: '确认加入' }))
    expect(await screen.findByText('你已加入此工作区')).toBeInTheDocument()
    await router.navigate(`/join/invite/${'B'.repeat(43)}`)
    expect(await screen.findByRole('button', { name: '确认加入' })).toBeInTheDocument()
    expect(screen.queryByText('你已加入此工作区')).not.toBeInTheDocument()
  })
  it.each([
    ['used', '这条邀请链接已被使用'],
    ['expired', '这条链接已过期'],
    ['revoked', '这条链接已被管理员撤销'],
    ['unavailable', '这条链接暂时无法使用'],
  ])('explains %s before requiring sign-in', async (state, title) => {
    server.use(
      http.post('/api/v1/join/links/preview', () => HttpResponse.json({ kind: 'invite', state })),
    )
    renderJoin('invite')
    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '登录后确认加入' })).not.toBeInTheDocument()
    expect(screen.queryByText('join_link_unavailable')).not.toBeInTheDocument()
  })

  it('keeps the bearer token out of login returnTo', async () => {
    server.use(
      http.post('/api/v1/join/links/preview', () =>
        HttpResponse.json({ kind: 'invite', state: 'active' }),
      ),
    )
    const { router } = renderJoin('invite')
    await userEvent.setup().click(await screen.findByRole('button', { name: '登录后确认加入' }))
    expect(await screen.findByText('Login screen')).toBeInTheDocument()
    expect(router.state.location.search).toContain(encodeURIComponent('/join/continue'))
    expect(router.state.location.search).not.toContain(TOKEN)
    expect(sessionStorage.getItem(PENDING_JOIN_PATH_KEY)).toBe(`/join/invite/${TOKEN}`)
    sessionStorage.removeItem(PENDING_JOIN_PATH_KEY)
  })

  it('lets a current member enter through a used link without submitting it', async () => {
    installSignedInSession()
    server.use(
      http.post('/api/v1/join/links/preview', () =>
        HttpResponse.json({
          kind: 'invite',
          state: 'used',
          alreadyMember: true,
          name: 'Team',
          slug: 'team',
        }),
      ),
    )
    renderJoin('invite')
    expect(await screen.findByText('你已加入此工作区')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认加入' })).not.toBeInTheDocument()
    await userEvent.setup().click(await screen.findByRole('button', { name: '进入工作区' }))
    expect(await screen.findByText('Issues screen')).toBeInTheDocument()
  })

  it('distinguishes service failure and retries instead of calling a link expired', async () => {
    let calls = 0
    server.use(
      http.post('/api/v1/join/links/preview', () => {
        calls += 1
        if (calls === 1) return HttpResponse.json({ code: 'upstream_unavailable' }, { status: 503 })
        return HttpResponse.json({ kind: 'invite', state: 'active' })
      }),
    )
    renderJoin('invite')
    await userEvent.setup().click(await screen.findByRole('button', { name: '重试' }))
    expect(await screen.findByRole('heading', { name: '邀请有效' })).toBeInTheDocument()
    expect(screen.queryByText('这条链接已过期')).not.toBeInTheDocument()
  })
})
