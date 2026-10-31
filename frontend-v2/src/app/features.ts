import { chat } from '@/features/chat'
import { dashboard } from '@/features/dashboard'
import { collaboration } from '@/features/collaboration'
import { schedules } from '@/features/schedules'
import { workflows } from '@/features/workflows'
import { settings } from '@/features/settings'
import type { Feature } from './feature'

/** Navigation order. Adding a page is one directory under `features/` plus one entry here. */
export const features: Feature[] = [
  chat,
  collaboration,
  workflows,
  schedules,
  dashboard,
  settings,
]
