import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter } from 'react-router'
import { App } from './app/app'
import { routes } from './app/routes'
import './index.css'

// This is a client-only app: apply the saved theme before the first React paint. next-themes
// then owns preference changes and system/storage subscriptions; its SSR script stays inert.
try {
  const preference = localStorage.getItem('ora-v2-theme')
  const dark =
    preference === 'dark' ||
    (preference === 'system' &&
      matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
} catch {
  // Browsers that block storage can still apply a session-only preference through the provider.
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App router={createBrowserRouter(routes)} />
  </StrictMode>,
)
