import { createContext, useContext } from 'react'

/** Lets something inside a page with a workspace panel, such as a turn's changed files, open one. */
export interface WorkspaceNavigation {
  /** Opens the panel on this file's diff; the path may be absolute, as Agents report it. */
  openDiff: (path: string) => void
}

const Context = createContext<WorkspaceNavigation | null>(null)

/** Provides the navigation of the page's workspace panel. */
export const WorkspaceNavigationProvider = Context.Provider

/** The page's workspace panel navigation, or null where the page has no panel. */
export function useWorkspaceNavigation(): WorkspaceNavigation | null {
  return useContext(Context)
}
