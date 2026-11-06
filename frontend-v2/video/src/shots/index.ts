import type { Shot } from '../record/shot.ts'
import { leadDecompose } from './lead-decompose.ts'
import { oraOverview } from './ora-overview.ts'
import { newcomerCdase } from './newcomer-cdase.ts'
import { dashboardUsage } from './dashboard-usage.ts'

/** Every shot the recorder knows, by name, in the order the film plays them. */
export const shots: Record<string, Shot> = {
  [oraOverview.name]: oraOverview,
  [newcomerCdase.name]: newcomerCdase,
  [leadDecompose.name]: leadDecompose,
  [dashboardUsage.name]: dashboardUsage,
}
