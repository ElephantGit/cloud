import type * as acp from '@agentclientprotocol/sdk'
import { memo } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { cn } from '@/lib/utils'
import { HighlightedCode } from './code'
import { hostname, linkCitations } from './links'

type Source = Extract<acp.ContentBlock, { type: 'resource_link' }>

/**
 * Agent text as Markdown (GitHub flavour). Fenced code is highlighted; `[n]` outside code becomes
 * a citation of the turn's n-th source when there is one.
 */
export const Markdown = memo(function Markdown({ text, sources = [] }: { text: string; sources?: Source[] }) {
  const components: Components = {
    a: ({ href, children }) => {
      const cite = href?.match(/^#cite-(\d+)$/)
      const source = cite ? sources[Number(cite[1]) - 1] : undefined
      if (source) return <Citation n={Number(cite![1])} source={source} />
      return (
        <a href={href} target="_blank" rel="noreferrer" className="font-medium underline underline-offset-4">
          {children}
        </a>
      )
    },
    pre: ({ children }) => <>{children}</>,
    code: ({ className, children }) => {
      const language = className?.match(/language-(\S+)/)?.[1]
      const code = String(children)
      if (!language && !code.includes('\n')) {
        return <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">{children}</code>
      }
      return <HighlightedCode code={code} language={language ?? 'text'} />
    },
    table: ({ children }) => (
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm">{children}</table>
      </div>
    ),
    th: ({ children }) => <th className="border-b px-2 py-1.5 text-left font-medium">{children}</th>,
    td: ({ children }) => <td className="border-b px-2 py-1.5">{children}</td>,
  }
  return (
    <div
      className={cn(
        'flex min-w-0 flex-col gap-3 text-sm leading-relaxed',
        '[&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:font-semibold',
        '[&_ol]:list-decimal [&_ol]:pl-5 [&_ul]:list-disc [&_ul]:pl-5 [&_li]:my-0.5',
        '[&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground',
      )}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {sources.length ? linkCitations(text) : text}
      </ReactMarkdown>
    </div>
  )
})

function Citation({ n, source }: { n: number; source: Source }) {
  const host = hostname(source.uri)
  return (
    <HoverCard>
      <HoverCardTrigger
        delay={10}
        closeDelay={100}
        render={<Button variant="secondary" size="xs" className="mx-0.5 h-5 px-1.5 align-baseline text-[11px]" />}
        aria-label={`来源 ${n}：${source.title ?? source.name}`}
      >
        {host}
      </HoverCardTrigger>
      <HoverCardContent className="flex w-64 flex-col gap-1 text-sm">
        <div className="font-medium">{source.title ?? source.name}</div>
        {source.description && <p className="text-muted-foreground">{source.description}</p>}
        <a href={source.uri} target="_blank" rel="noreferrer" className="text-xs text-muted-foreground underline-offset-4 hover:underline">
          {host}
        </a>
      </HoverCardContent>
    </HoverCard>
  )
}
