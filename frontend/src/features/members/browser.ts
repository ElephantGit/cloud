import { useEffect, useState } from 'react'

/** Copies only after an explicit user action; callers surface clipboard failures. */
export async function copyJoinLink(value: string): Promise<void> {
  await navigator.clipboard.writeText(value)
}

/** Keeps expiry badges current while the page stays open. Server validation remains authoritative. */
export function useJoinClock() {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])
  return now
}

/** Formats an API instant consistently in the viewer's locale and local time zone. */
export function entryDate(value: string | null | undefined, locale: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '—'
    : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

/** Computes visible metadata state without ever treating a date alone as proof of validity. */
export function linkState(
  row: { expiresAt: string; consumedAt?: string | null; revokedAt?: string | null },
  now: number,
) {
  if (row.consumedAt) return 'used'
  if (row.revokedAt) return 'revoked'
  if (new Date(row.expiresAt).getTime() <= now) return 'expired'
  return 'active'
}
