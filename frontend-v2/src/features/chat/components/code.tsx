import { CodeBlock, CodeBlockActions, CodeBlockContent, CodeBlockCopyButton, CodeBlockHeader, CodeBlockLine, CodeBlockTitle } from '@/components/ui/code-block'
import { useHighlightedCode } from '@/hooks/use-highlighted-code'
import { diffLines } from '../model/diff'

/** A fenced code block with syntax highlighting and a copy button. */
export function HighlightedCode({ code, language, title }: { code: string; language: string; title?: string }) {
  const lines = useHighlightedCode(code, language)
  return (
    <CodeBlock>
      {title || language ? (
        <CodeBlockHeader>
          <CodeBlockTitle>{title ?? language}</CodeBlockTitle>
          <CodeBlockActions>
            <CodeBlockCopyButton value={code} />
          </CodeBlockActions>
        </CodeBlockHeader>
      ) : (
        <CodeBlockActions>
          <CodeBlockCopyButton value={code} />
        </CodeBlockActions>
      )}
      <CodeBlockContent>
        {lines.map((line, i) => (
          <CodeBlockLine key={i}>{line}</CodeBlockLine>
        ))}
      </CodeBlockContent>
    </CodeBlock>
  )
}

/** Plain monospace output such as a command's stdout, small enough to sit inside a trace line. */
export function OutputBlock({ text, error }: { text: string; error?: boolean }) {
  return (
    <CodeBlock size="sm" variant={error ? 'destructive' : 'default'}>
      <CodeBlockContent wrap className="max-h-64">
        {text}
      </CodeBlockContent>
    </CodeBlock>
  )
}

/** A file diff as added and removed lines, from ACP's old and new text. */
export function DiffBlock({ path, oldText, newText }: { path: string; oldText?: string | null; newText: string }) {
  const lines = diffLines(oldText ?? '', newText)
  return (
    <CodeBlock size="sm">
      <CodeBlockHeader>
        <CodeBlockTitle>{path}</CodeBlockTitle>
      </CodeBlockHeader>
      <CodeBlockContent className="max-h-80">
        {lines.map((l, i) => (
          <CodeBlockLine key={i} variant={l.op === '+' ? 'added' : l.op === '-' ? 'removed' : 'default'}>
            {l.text}
          </CodeBlockLine>
        ))}
      </CodeBlockContent>
    </CodeBlock>
  )
}
