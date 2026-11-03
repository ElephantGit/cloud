/** Shared browser key that wakes other tabs when the signed-in identity changes. */
export const AUTH_IDENTITY_KEY = 'ora-browser-session-user'

/**
 * Publishes the current identity only when it changes. Other tabs receive a `storage` event and
 * revalidate their session; the current tab does not need that event.
 */
export function rememberAuthIdentity(userId: string) {
  try {
    if (localStorage.getItem(AUTH_IDENTITY_KEY) === userId) return
    localStorage.setItem(AUTH_IDENTITY_KEY, userId)
  } catch {
    // Storage can be blocked. The session still works in this tab without cross-tab refresh.
  }
}

/** Removes the shared identity after a successful sign-out so other tabs revalidate too. */
export function forgetAuthIdentity() {
  try {
    localStorage.removeItem(AUTH_IDENTITY_KEY)
  } catch {
    // See rememberAuthIdentity.
  }
}

/** Subscribes to identity changes made by another tab; returns the unsubscribe function. */
export function subscribeAuthIdentity(onChange: () => void): () => void {
  const listener = (event: StorageEvent) => {
    if (event.key === AUTH_IDENTITY_KEY) onChange()
  }
  window.addEventListener('storage', listener)
  return () => window.removeEventListener('storage', listener)
}
