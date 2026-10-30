import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { ThemeProvider } from 'next-themes'
import { UserContext } from '@/app/auth/user'
import { demoUser } from '@/test/backend'

export function renderSettingsPage(element: ReactNode, path = '/settings') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createMemoryRouter([{ path: '*', element }], {
    initialEntries: [path],
  })
  const result = render(
    <ThemeProvider
      attribute="class"
      storageKey="ora-v2-theme"
      defaultTheme="light"
      scriptProps={{ type: 'text/plain' }}
    >
      <QueryClientProvider client={client}>
        <UserContext.Provider value={demoUser}>
          <RouterProvider router={router} />
        </UserContext.Provider>
      </QueryClientProvider>
    </ThemeProvider>,
  )
  return { ...result, client, router }
}
