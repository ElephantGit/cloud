/** One displayed line of a unified diff: a hunk header, context, an addition or a removal. */
export interface PatchLine {
  op: '@' | ' ' | '+' | '-'
  text: string
}

/** What a single-file unified diff shows: its lines, or that Git judged the file binary. */
export interface ParsedPatch {
  binary: boolean
  lines: PatchLine[]
}

/**
 * Reads `git diff` output for one file. The file header (`diff --git`, `index`, `---`/`+++`, mode
 * lines) is dropped; what follows the first hunk header is shown line by line.
 */
export function parsePatch(patch: string): ParsedPatch {
  const lines: PatchLine[] = []
  let inHunk = false
  let binary = false
  for (const raw of patch.split('\n')) {
    if (raw.startsWith('@@')) {
      inHunk = true
      lines.push({ op: '@', text: raw })
      continue
    }
    if (!inHunk) {
      if (raw.startsWith('Binary files ')) binary = true
      continue
    }
    if (raw.startsWith('\\')) continue // "\ No newline at end of file"
    const op = raw[0]
    if (op === '+' || op === '-' || op === ' ') lines.push({ op, text: raw.slice(1) })
    else if (raw.startsWith('diff --git')) inHunk = false
  }
  return { binary, lines }
}
