import { Users } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const multica: Feature = {
  path: 'multica',
  title: 'Multica',
  icon: Users,
  page: () => import('./page'),
}
