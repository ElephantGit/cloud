import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { App } from './app'
import { features } from './features'
import { routes } from './routes'

const renderAt = (path: string) => {
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  render(<App router={router} />)
  return router
}

describe('app shell', () => {
  it('opens the first page from the root', async () => {
    const router = renderAt('/')
    expect(await screen.findByRole('heading', { level: 1, name: features[0].title })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe(`/${features[0].path}`)
  })

  it('switches pages from the icon rail', async () => {
    const router = renderAt('/')
    for (const f of features) {
      await userEvent.click(screen.getByRole('link', { name: f.title }))
      expect(await screen.findByRole('heading', { level: 1, name: f.title })).toBeInTheDocument()
      expect(router.state.location.pathname).toBe(`/${f.path}`)
    }
  })

  it('sends unknown paths home', async () => {
    const router = renderAt('/nope')
    await screen.findByRole('heading', { level: 1, name: features[0].title })
    expect(router.state.location.pathname).toBe(`/${features[0].path}`)
  })
})
