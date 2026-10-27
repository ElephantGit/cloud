import { joinedLinkPath, PENDING_JOIN_PATH_KEY } from '@/lib/paths'

/** Saves a private join path only in this tab; OAuth receives a tokenless return path. */
export function rememberJoinPath(path: string): boolean {
  const valid = joinedLinkPath(path)
  if (!valid) return false
  try {
    sessionStorage.setItem(PENDING_JOIN_PATH_KEY, valid)
    return true
  } catch {
    return false
  }
}

/** Reads only a validated same-origin join path after an external login. */
export function pendingJoinPath(): string | undefined {
  try {
    return joinedLinkPath(sessionStorage.getItem(PENDING_JOIN_PATH_KEY) ?? '')
  } catch {
    return undefined
  }
}

/** Drops a completed or abandoned bearer link from this tab's recovery storage. */
export function clearPendingJoin(): void {
  try {
    sessionStorage.removeItem(PENDING_JOIN_PATH_KEY)
  } catch {
    /* Storage cannot undo a committed join. */
  }
}
