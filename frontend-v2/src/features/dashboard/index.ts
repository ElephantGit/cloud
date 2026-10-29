import { LayoutDashboard } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const dashboard: Feature = {
  path: 'dashboard',
  title: '仪表盘',
  icon: LayoutDashboard,
  page: () => import('./page'),
}
