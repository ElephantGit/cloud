import { Suspense } from 'react'
import { Outlet } from 'react-router'
import { SessionGate } from './auth/session'
import { NavRail } from './nav-rail'

/** Signed-in only. Icon rail on the left; the rest of the window belongs to the current feature page. */
export function Layout() {
  return (
    <SessionGate>
      <div className="flex h-svh">
        <NavRail />
        <main className="flex min-w-0 flex-1 flex-col overflow-auto">
          <Suspense fallback={<p className="p-4 text-sm text-muted-foreground">加载中…</p>}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </SessionGate>
  )
}
