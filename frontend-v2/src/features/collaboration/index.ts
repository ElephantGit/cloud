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
    // Squad management moved to settings; old links keep landing on the same squad.
    { path: 'squads', loader: ({ request }) => redirect(`/settings/squads${new URL(request.url).search}`) },
    { path: '*', loader: () => redirect('/collaboration') },
  ],
}
