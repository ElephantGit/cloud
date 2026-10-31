import { Users } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const collaboration: Feature = {
  path: 'collaboration',
  title: '协作',
  icon: Users,
  page: () => import('./page'),
}
