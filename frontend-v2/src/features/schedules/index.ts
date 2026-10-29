import { CalendarClock } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const schedules: Feature = {
  path: 'schedules',
  title: '定时任务',
  icon: CalendarClock,
  page: () => import('./page'),
}
