import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it } from 'vitest'
import { demoUser, fakeBackend } from '@/test/backend'
import { App } from './app'
import { features } from './features'
import { routes } from './routes'

const renderAt = (path: string) => {
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  render(<App router={router} />)
  return router
}

describe('app shell, signed in', () => {
  beforeEach(() => {
    fakeBackend((c) => (c.url === '/api/v1/me' ? { status: 200, data: demoUser } : undefined))
  })

  it('opens the first page from the root', async () => {
    const router = renderAt('/')
    // The first visit loads the page's chunk, which can take a moment under jsdom.
    expect(await screen.findByRole('heading', { level: 1, name: features[0].title }, { timeout: 5000 })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe(`/${features[0].path}`)
  })

  it('switches pages from the icon rail', async () => {
    const router = renderAt('/')
    await screen.findByRole('heading', { level: 1, name: features[0].title })
    for (const f of features) {
      await userEvent.click(screen.getByRole('link', { name: f.title }))
      // Each page's chunk loads on its first visit; the workflow editor's is the largest.
      expect(await screen.findByRole('heading', { level: 1, name: f.title }, { timeout: 5000 })).toBeInTheDocument()
      expect(router.state.location.pathname).toBe(`/${f.path}`)
    }
  })

  it('sends unknown paths home', async () => {
    const router = renderAt('/nope')
    await screen.findByRole('heading', { level: 1, name: features[0].title })
    expect(router.state.location.pathname).toBe(`/${features[0].path}`)
  })

  it('shows who is signed in', async () => {
    renderAt('/')
    expect(await screen.findByRole('img', { name: '演示用户' })).toHaveTextContent('演')
  })
})

describe('session gate', () => {
  it('shows the login screen on 401, with one button per provider', async () => {
    fakeBackend((c) => {
      if (c.url === '/api/v1/me') return { status: 401, data: { code: 'unauthenticated', params: {}, requestId: 'x' } }
      if (c.url === '/auth/providers') return { status: 200, data: { providers: ['dev'], default: 'dev' } }
      return undefined
    })
    renderAt('/workflows')
    expect(await screen.findByRole('heading', { name: '登录 Ora' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '开发者登录' })).toBeInTheDocument()
  })

  it('tells a disabled user on 403', async () => {
    fakeBackend((c) => (c.url === '/api/v1/me' ? { status: 403, data: {} } : undefined))
    renderAt('/')
    expect(await screen.findByText('账号已停用，请联系管理员。')).toBeInTheDocument()
  })

  it('signs out back to the login screen', async () => {
    let signedIn = true
    const seen = fakeBackend((c) => {
      if (c.url === '/api/v1/me') return signedIn ? { status: 200, data: demoUser } : { status: 401, data: {} }
      if (c.url === '/auth/logout') return (signedIn = false), { status: 204 }
      if (c.url === '/auth/providers') return { status: 200, data: { providers: ['dev'] } }
      return undefined
    })
    renderAt('/')
    await userEvent.click(await screen.findByRole('button', { name: '退出登录' }))
    expect(await screen.findByRole('heading', { name: '登录 Ora' })).toBeInTheDocument()
    expect(seen).toContain('POST /auth/logout')
  })
})
