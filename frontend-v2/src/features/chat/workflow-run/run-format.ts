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
