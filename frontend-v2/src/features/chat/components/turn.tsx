import {
  ArrowUpRightIcon,
  ChevronRightIcon,
  CircleAlertIcon,
  CircleSlashIcon,
  FileIcon,
  FilePlusIcon,
  FolderIcon,
  FolderTreeIcon,
  GlobeIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { Bubble, BubbleContent } from '@/components/ui/bubble'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Message, MessageContent, MessageFooter } from '@/components/ui/message'
import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceDescription,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from '@/components/ui/trace'
import { buildRows, type FileChange, type Folder, fileTree, turnFileChanges, turnSources } from '../model/display'
import type { PermissionRequest, Turn } from '../model/types'
import { Rows, TurnProvider } from './activity'
import { AttachmentList, PluginList } from './attachments'
import { formatDuration, formatTokens } from './format'
import { hostname } from './links'

/** The person's prompt, right-aligned, with what they attached. */
export function UserPrompt({ turn }: { turn: Turn }) {
  const { text, attachments, plugins } = turn.prompt
  return (
    <Message align="end">
      <MessageContent>
        <AttachmentList files={attachments} className="justify-end" />
        <PluginList items={plugins} className="justify-end" />
        <Bubble align="end" variant="muted">
          <BubbleContent className="whitespace-pre-wrap">{text}</BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  )
}

/** Everything the agent did for one prompt, followed by its sources, changed files and ending. */
export function AgentTurn({
  turn,
  permissions,
  onAnswer,
}: {
  turn: Turn
  permissions: PermissionRequest[]
  onAnswer: (permissionId: string, optionId: string) => void
}) {
  const rows = useMemo(() => buildRows(turn), [turn])
  const sources = useMemo(() => turnSources(turn), [turn])
  const changes = useMemo(() => turnFileChanges(turn), [turn])
  const scope = useMemo(() => ({ status: turn.status, permissions, sources, onAnswer }), [turn.status, permissions, sources, onAnswer])
  const waitingForFirstOutput = turn.status === 'streaming' && rows.length === 0
  return (
    <TurnProvider value={scope}>
      <Message>
        <MessageContent className="gap-3">
          {waitingForFirstOutput && (
            <TraceLine status="running">
              <TraceTitle>正在准备…</TraceTitle>
            </TraceLine>
          )}
          <Rows rows={rows} />
          {sources.length > 0 && <Sources sources={sources} />}
          {changes.length > 0 && turn.status !== 'streaming' && <FileChanges changes={changes} />}
          <TurnEnding turn={turn} />
        </MessageContent>
      </Message>
    </TurnProvider>
  )
}

function TurnEnding({ turn }: { turn: Turn }) {
  if (turn.status === 'streaming') return null
  if (turn.status === 'failed') {
    return (
      <div role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
        <CircleAlertIcon className="mt-0.5 size-4 shrink-0" />
        <span>{turn.error ?? '这一轮没有完成。'}</span>
      </div>
    )
  }
  const took = turn.endedAt !== undefined ? formatDuration(turn.endedAt - turn.startedAt) : undefined
  const usage = turn.usage
  return (
    <MessageFooter className="gap-3 px-0 text-muted-foreground">
      {turn.status === 'cancelled' ? (
        <span className="inline-flex items-center gap-1">
          <CircleSlashIcon className="size-3.5" />
          已停止
        </span>
      ) : (
        took && <span>用时 {took}</span>
      )}
      {usage && (
        <span title={`输入 ${usage.inputTokens}，输出 ${usage.outputTokens}${usage.cachedReadTokens ? `，缓存命中 ${usage.cachedReadTokens}` : ''}`}>
          {formatTokens(usage.totalTokens)} tokens
        </span>
      )}
    </MessageFooter>
  )
}

function Sources({ sources }: { sources: ReturnType<typeof turnSources> }) {
  return (
    <Trace>
      <TraceCollapsible>
        <TraceCollapsibleTrigger>
          <TraceLine>
            <TraceIcon>
              <GlobeIcon />
            </TraceIcon>
            <TraceTitle>参考了 {sources.length} 个来源</TraceTitle>
          </TraceLine>
        </TraceCollapsibleTrigger>
        <TraceCollapsibleContent>
          <Trace>
            {sources.map((s, i) => (
              <a key={s.uri} href={s.uri} target="_blank" rel="noreferrer">
                <TraceLine>
                  <TraceDescription>[{i + 1}]</TraceDescription>
                  <TraceTitle>{s.title ?? s.name}</TraceTitle>
                  <TraceDescription>{hostname(s.uri)}</TraceDescription>
                  <TraceIcon>
                    <ArrowUpRightIcon />
                  </TraceIcon>
                </TraceLine>
              </a>
            ))}
          </Trace>
        </TraceCollapsibleContent>
      </TraceCollapsible>
    </Trace>
  )
}

/** The files this turn changed, as a tree with added and removed line counts. */
function FileChanges({ changes }: { changes: FileChange[] }) {
  const tree = useMemo(() => fileTree(changes), [changes])
  const added = changes.reduce((n, c) => n + c.added, 0)
  const removed = changes.reduce((n, c) => n + c.removed, 0)
  return (
    <Trace>
      <TraceCollapsible defaultOpen>
        <TraceCollapsibleTrigger>
          <TraceLine>
            <TraceIcon>
              <FolderTreeIcon />
            </TraceIcon>
            <TraceTitle>修改了 {changes.length} 个文件</TraceTitle>
            <TraceDescription>
              <span className="text-emerald-600 dark:text-emerald-400">+{added}</span> <span className="text-destructive">−{removed}</span>
            </TraceDescription>
          </TraceLine>
        </TraceCollapsibleTrigger>
        <TraceCollapsibleContent>
          <div role="tree" aria-label="改动的文件" className="flex flex-col text-sm">
            <FolderNodes folder={tree} />
          </div>
        </TraceCollapsibleContent>
      </TraceCollapsible>
    </Trace>
  )
}

function FolderNodes({ folder }: { folder: Folder }) {
  return (
    <>
      {folder.folders.map((f) => (
        <Collapsible key={f.path} defaultOpen>
          <CollapsibleTrigger className="group/folder flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left hover:bg-muted" role="treeitem">
            <ChevronRightIcon className="size-3.5 transition-transform group-aria-expanded/folder:rotate-90" />
            <FolderIcon className="size-3.5" />
            <span className="font-mono text-xs">{f.name}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="ml-4 flex flex-col">
              <FolderNodes folder={f} />
            </div>
          </CollapsibleContent>
        </Collapsible>
      ))}
      {folder.files.map((c) => (
        <div key={c.path} role="treeitem" className="flex items-center gap-1.5 px-1 py-0.5 pl-6">
          {c.created ? <FilePlusIcon className="size-3.5" /> : <FileIcon className="size-3.5" />}
          <span className="min-w-0 truncate font-mono text-xs">{c.path.split('/').at(-1)}</span>
          <span className="ml-auto font-mono text-xs tabular-nums">
            <span className="text-emerald-600 dark:text-emerald-400">+{c.added}</span> <span className="text-destructive">−{c.removed}</span>
          </span>
        </div>
      ))}
    </>
  )
}
