/** A short Chinese duration: "0.4 秒", "12 秒", "2 分 5 秒". */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${(Math.max(ms, 0) / 1000).toFixed(1)} 秒`
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s} 秒`
  return `${Math.floor(s / 60)} 分 ${s % 60} 秒`
}

/** Token counts the way a header can fit them: 860, 12.3k, 1.2M. */
export function formatTokens(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`
  return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`
}

/** A file size: 2.1 KB, 180 KB, 1.4 MB. */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
