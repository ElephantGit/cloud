import { Settings } from 'lucide-react'
import { redirect } from 'react-router'
import type { Feature } from '@/app/feature'

export const settings: Feature = {
  path: 'settings',
  title: '设置',
  icon: Settings,
  page: () => import('./page'),
  children: [
    { index: true, loader: () => redirect('/settings/members') },
    {
      path: 'members',
      lazy: async () => ({
        Component: (await import('./members/page')).default,
      }),
    },
    {
      path: 'model-connections',
      lazy: async () => ({
        Component: (await import('./model-connections/page')).default,
      }),
    },
    {
      path: 'theme',
      lazy: async () => ({ Component: (await import('./theme/page')).default }),
    },
    {
      path: 'plugins',
      lazy: async () => ({
        Component: (await import('./plugins/page')).default,
      }),
    },
    {
      path: 'skills',
      lazy: async () => ({
        Component: (await import('./skills/page')).default,
      }),
    },
    { path: '*', loader: () => redirect('/settings/members') },
  ],
}
