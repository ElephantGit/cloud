import { chat } from '@/features/chat'
import { dashboard } from '@/features/dashboard'
import { multica } from '@/features/multica'
import { schedules } from '@/features/schedules'
import { workflows } from '@/features/workflows'
import type { Feature } from './feature'

/** Navigation order. Adding a page is one directory under `features/` plus one entry here. */
export const features: Feature[] = [chat, multica, workflows, schedules, dashboard]
