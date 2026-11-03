import { Users } from 'lucide-react'
import { redirect } from 'react-router'
import type { Feature } from '@/app/feature'

export const collaboration: Feature = {
  path: 'collaboration',
  title: '协作',
  icon: Users,
  page: () => import('./page'),
  children: [
    { index: true, lazy: async () => ({ Component: (await import('./tasks-page')).default }) },
    { path: 'tasks/:taskId', lazy: async () => ({ Component: (await import('./task-page')).default }) },
    { path: 'squads', lazy: async () => ({ Component: (await import('./squads-page')).default }) },
    { path: '*', loader: () => redirect('/collaboration') },
  ],
}
