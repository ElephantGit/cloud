/** Turns bare `[n]` into links Markdown can carry, leaving code spans and fences alone. */
export function linkCitations(text: string): string {
  return text
    .split(/(```[\s\S]*?(?:```|$)|`[^`\n]*`)/)
    .map((part, i) => (i % 2 ? part : part.replace(/\[(\d+)\](?![(:[])/g, '[$1](#cite-$1)')))
    .join('')
}

/** The host of a link, for compact labels; the raw text when it is not a URL. */
export function hostname(uri: string): string {
  try {
    return new URL(uri).hostname.replace(/^www\./, '')
  } catch {
    return uri
  }
}
