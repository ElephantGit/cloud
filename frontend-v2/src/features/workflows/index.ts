import { Workflow } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const workflows: Feature = {
  path: 'workflows',
  title: '工作流',
  icon: Workflow,
  page: () => import('./page'),
}
