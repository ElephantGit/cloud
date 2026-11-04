import { LayoutDashboard } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const dashboard: Feature = {
  path: 'dashboard',
  title: '仪表盘',
  icon: LayoutDashboard,
  page: () => import('./layout'),
  children: [
    { index: true, lazy: async () => ({ Component: (await import('./page')).default }) },
    { path: 'sessions', lazy: async () => ({ Component: (await import('./analysis/list')).default }) },
    { path: 'sessions/:id', lazy: async () => ({ Component: (await import('./analysis/detail')).default }) },
  ],
}
