/** Formats an ISO timestamp as local H:MM:SS for run node timing, as desktop's `lib/format` does. */
export function formatRunClock(iso: string, locale?: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat(locale, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

/** Formats an epoch-ms timestamp as a short relative label, e.g. "2h ago". */

/** Formats a duration as "42s", "3m 05s" or "1h 02m", as desktop's `lib/format` does. */
export function formatElapsedDuration(durationMs: number | undefined): string | null {
  if (durationMs === undefined || !Number.isFinite(durationMs) || durationMs < 0) return null;
  const seconds = Math.floor(durationMs / 1_000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds.toString().padStart(2, "0")}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${(minutes % 60).toString().padStart(2, "0")}m`;
}
