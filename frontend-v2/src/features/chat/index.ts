import { MessageSquare } from 'lucide-react'
import type { Feature } from '@/app/feature'

export const chat: Feature = {
  path: 'chat',
  title: '对话',
  icon: MessageSquare,
  page: () => import('./page'),
}
