/** One line of a diff: added, removed or unchanged. */
export interface DiffLine {
  op: '+' | '-' | ' '
  text: string
}

/**
 * A line diff by longest common subsequence. Tool diffs are one file at a time and small, so the
 * quadratic table is fine; past a size limit it falls back to "all removed, all added".
 */
export function diffLines(prev: string, next: string): DiffLine[] {
  const split = (s: string) => (s === '' ? [] : s.replace(/\n$/, '').split('\n'))
  const a = split(prev)
  const b = split(next)
  if (a.length * b.length > 250_000) {
    return [...a.map((text) => ({ op: '-' as const, text })), ...b.map((text) => ({ op: '+' as const, text }))]
  }
  const table = Array.from({ length: a.length + 1 }, () => new Array<number>(b.length + 1).fill(0))
  for (let i = a.length - 1; i >= 0; i--)
    for (let j = b.length - 1; j >= 0; j--)
      table[i][j] = a[i] === b[j] ? table[i + 1][j + 1] + 1 : Math.max(table[i + 1][j], table[i][j + 1])
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      out.push({ op: ' ', text: a[i] })
      i++
      j++
    } else if (table[i + 1][j] >= table[i][j + 1]) out.push({ op: '-', text: a[i++] })
    else out.push({ op: '+', text: b[j++] })
  }
  while (i < a.length) out.push({ op: '-', text: a[i++] })
  while (j < b.length) out.push({ op: '+', text: b[j++] })
  return out
}
