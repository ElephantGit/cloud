const adjectives = ['amber', 'brisk', 'calm', 'clear', 'deft', 'eager', 'gentle', 'keen', 'lucid', 'mellow', 'nimble', 'quiet', 'steady', 'sunny', 'swift', 'tidy']
/** Words that say nothing about the change, left out of a branch name. */
const filler = new Set(['a', 'an', 'and', 'are', 'can', 'could', 'for', 'from', 'in', 'into', 'is', 'it', 'of', 'on', 'or', 'please', 'the', 'this', 'that', 'to', 'we', 'with', 'you'])
const nouns = ['brook', 'canyon', 'cedar', 'comet', 'delta', 'ember', 'falcon', 'harbor', 'island', 'maple', 'meadow', 'otter', 'pine', 'river', 'summit', 'willow']

/**
 * The branch for a worktree nobody named, taken from the chat's first prompt as agent tools do
 * once a chat starts: `ora/` and the prompt's first few meaningful Latin words
 * (`ora/fix-login-redirect`).
 * A prompt without any, such as one written in Chinese, gets `ora/<adjective>-<noun>` instead.
 * The name never repeats one in `taken`; a numeric suffix settles a collision.
 *
 * The agent will name the branch itself once v2-server runs one; this stands in until then.
 */
export function branchNameFor(prompt: string, taken: Iterable<string>, random: () => number = Math.random): string {
  const used = new Set(taken)
  const words = (prompt.toLowerCase().match(/[a-z0-9]+/g) ?? []).filter((w) => !filler.has(w)).slice(0, 4)
  const pick = (list: string[]) => list[Math.floor(random() * list.length)]
  const base = words.length ? `ora/${words.join('-')}` : `ora/${pick(adjectives)}-${pick(nouns)}`
  if (!used.has(base)) return base
  let n = 2
  while (used.has(`${base}-${n}`)) n++
  return `${base}-${n}`
}
