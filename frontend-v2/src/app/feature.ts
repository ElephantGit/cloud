import type { LucideIcon } from 'lucide-react'
import type { ComponentType } from 'react'
import type { RouteObject } from 'react-router'

/**
 * One page in the left navigation. Each feature directory exports one of these from its
 * `index.ts`; `features.ts` lists them in navigation order.
 */
export interface Feature {
  /** URL segment, also the route id: `/chat`. */
  path: string
  /** Navigation label and page title, in Chinese. */
  title: string
  icon: LucideIcon
  /** Loaded on first visit, so features are code-split and build independently. */
  page: () => Promise<{ default: ComponentType }>
  /** Optional subpages rendered in the feature's Outlet. */
  children?: RouteObject[]
}
