import { type UseQueryResult, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeftIcon,
  ChevronRightIcon,
  FileIcon,
  FileTextIcon,
  FolderIcon,
  GitCompareArrowsIcon,
  PanelRightIcon,
  RefreshCwIcon,
  XIcon,
} from 'lucide-react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import { HighlightedCode } from '@/components/agent-thread/code'
import { Button } from '@/components/ui/button'
import { CodeBlock, CodeBlockContent, CodeBlockLine } from '@/components/ui/code-block'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { codeOf } from '@/lib/api-client'
import { subtleScrollbar } from '@/lib/scrollbar'
import { cn } from '@/lib/utils'
import type { WorkspaceChange, WorkspaceChanges, WorkspaceEntry } from '@/lib/workspace-api/generated'
import { parsePatch } from './patch'
import { languageOf, relativePath, type WorkspaceReader, type WorkspaceSource, workspaceReader } from './source'

type Tab = 'changes' | 'files'
type View = { kind: 'diff' | 'file'; path: string }

/** A request from outside the panel to show one file's diff; a new nonce repeats the same path. */
export interface OpenRequest {
  path: string
  nonce: number
}

/**
 * A side panel that shows an Agent's checkout read-only, as desktop's review pane does: what
 * changed against the commit it started from, and every file. The page bumps `revision` whenever
 * the Agent may have written (a tool call finished, a turn or node ended), and the panel reads
 * the checkout again.
 */
export function WorkspacePanel({
  source,
  revision,
  request,
  onClose,
}: {
  source: WorkspaceSource
  revision: string
  request?: OpenRequest
  onClose: () => void
}) {
  const reader = useMemo(() => workspaceReader(source), [source])
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<Tab>('changes')
  const [view, setView] = useState<View | null>(null)
  const changes = useQuery({ queryKey: [...reader.key, 'changes'], queryFn: reader.changes, retry: false })

  useEffect(() => {
    void queryClient.invalidateQueries({ queryKey: reader.key })
  }, [queryClient, reader, revision])

  const [handled, setHandled] = useState<number | undefined>(undefined)
  if (request && request.nonce !== handled) {
    setHandled(request.nonce)
    setTab('changes')
    setView({ kind: 'diff', path: relativePath(request.path, changes.data?.files) })
  }

  return (
    <aside aria-label="工作区" className="flex h-full min-h-0 w-full flex-col border-l bg-background">
      <header className="flex h-12 shrink-0 items-center gap-1.5 border-b px-3">
        <div role="group" aria-label="工作区视图" className="inline-flex rounded-lg border p-0.5">
          {(['changes', 'files'] as const).map((t) => (
            <Button
              key={t}
              size="sm"
              variant={tab === t && !view ? 'secondary' : 'ghost'}
              className="h-7 gap-1.5 px-2.5 text-xs"
              aria-pressed={tab === t && !view}
              onClick={() => {
                setTab(t)
                setView(null)
              }}
            >
              {t === 'changes' ? <GitCompareArrowsIcon className="size-3.5" /> : <FolderIcon className="size-3.5" />}
              {t === 'changes' ? '变更' : '文件'}
              {t === 'changes' && changes.data && changes.data.files.length > 0 && (
                <span className="font-mono text-[10px] text-muted-foreground tabular-nums">{changes.data.files.length}</span>
              )}
            </Button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-0.5">
          <Button size="icon-sm" variant="ghost" aria-label="刷新" onClick={() => void queryClient.invalidateQueries({ queryKey: reader.key })}>
            <RefreshCwIcon />
          </Button>
          <Button size="icon-sm" variant="ghost" aria-label="关闭工作区" onClick={onClose}>
            <XIcon />
          </Button>
        </div>
      </header>
      <div className={cn('min-h-0 flex-1 overflow-y-auto', subtleScrollbar)}>
        {view ? (
          <FileView
            reader={reader}
            view={view}
            status={changes.data?.files.find((c) => c.path === view.path)?.status}
            onBack={() => setView(null)}
            onSwitch={setView}
          />
        ) : tab === 'changes' ? (
          <ChangesList query={changes} onOpen={(path) => setView({ kind: 'diff', path })} />
        ) : (
          <div role="tree" aria-label="工作区文件" className="flex flex-col p-2 text-sm">
            <Directory reader={reader} path="" depth={0} onOpen={(path) => setView({ kind: 'file', path })} />
          </div>
        )}
      </div>
    </aside>
  )
}

/** The header button that shows or hides a page's workspace panel. */
export function WorkspaceToggle({ open, toggle }: { open: boolean; toggle: () => void }) {
  return (
    <Button size="sm" variant={open ? 'secondary' : 'ghost'} className="h-7 gap-1.5 px-2 text-xs" aria-pressed={open} onClick={toggle}>
      <PanelRightIcon className="size-3.5" />
      工作区
    </Button>
  )
}

/** What a failed read means to the person, by the server's error code. */
function failureText(error: unknown): string {
  switch (codeOf(error)) {
    case 'workspace_not_ready':
      return 'Agent 还没有开始工作，工作区在第一轮开始后才会准备好。'
    case 'workspace_removed':
      return '这个工作区已被清理，无法再查看。'
    case 'binary_file':
      return '这是二进制文件，无法预览。'
    case 'too_large':
      return '内容太大，无法预览。'
    case 'path_not_found':
      return '文件不存在，可能已被删除。'
    case 'invalid_path':
      return '这个路径不在工作区内。'
    default:
      return '读取失败。'
  }
}

function Message({ children, error, onRetry }: { children: ReactNode; error?: boolean; onRetry?: () => void }) {
  return (
    <div role={error ? 'alert' : 'status'} className="flex flex-col items-center gap-3 px-6 py-10 text-center text-sm text-muted-foreground">
      <p>{children}</p>
      {onRetry && (
        <Button size="sm" variant="outline" onClick={onRetry}>
          重试
        </Button>
      )}
    </div>
  )
}

/** Codes that describe the workspace, not a failure worth retrying. */
const settled = new Set(['workspace_not_ready', 'workspace_removed', 'binary_file', 'too_large', 'path_not_found', 'invalid_path'])

function Failure({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const code = codeOf(error)
  return (
    <Message error={!code || !settled.has(code)} onRetry={code && settled.has(code) ? undefined : onRetry}>
      {failureText(error)}
    </Message>
  )
}

const statusMark: Record<WorkspaceChange['status'], { letter: string; label: string; className: string }> = {
  added: { letter: 'A', label: '新增', className: 'text-success' },
  modified: { letter: 'M', label: '修改', className: 'text-warning' },
  deleted: { letter: 'D', label: '删除', className: 'text-destructive' },
}

function ChangesList({
  query,
  onOpen,
}: {
  query: UseQueryResult<WorkspaceChanges>
  onOpen: (path: string) => void
}) {
  if (query.isPending) return <Message>正在读取变更…</Message>
  if (query.isError) return <Failure error={query.error} onRetry={() => void query.refetch()} />
  const { files, truncated } = query.data
  if (files.length === 0) return <Message>还没有文件改动。</Message>
  const added = files.reduce((n, c) => n + c.additions, 0)
  const removed = files.reduce((n, c) => n + c.deletions, 0)
  return (
    <div className="flex flex-col py-1">
      <p className="px-3 py-1.5 text-xs text-muted-foreground">
        相对初始版本改动了 {files.length} 个文件 <span className="text-success">+{added}</span> <span className="text-destructive">−{removed}</span>
      </p>
      <ul aria-label="改动的文件" className="flex flex-col">
        {files.map((c) => {
          const name = c.path.split('/').at(-1)
          const dir = c.path.slice(0, -(name?.length ?? 0)).replace(/\/$/, '')
          const mark = statusMark[c.status]
          return (
            <li key={c.path}>
              <button
                type="button"
                className="flex w-full min-w-0 items-center gap-2 px-3 py-1.5 text-left outline-none hover:bg-muted focus-visible:bg-muted"
                title={c.path}
                onClick={() => onOpen(c.path)}
              >
                <span className={cn('w-3 shrink-0 font-mono text-[11px] font-semibold', mark.className)} aria-label={mark.label}>
                  {mark.letter}
                </span>
                <span className={cn('shrink-0 font-mono text-xs', c.status === 'deleted' && 'line-through')}>{name}</span>
                {dir && <span className="min-w-0 truncate font-mono text-[11px] text-muted-foreground">{dir}</span>}
                <span className="ml-auto shrink-0 font-mono text-[11px] tabular-nums">
                  {c.binary ? (
                    <span className="text-muted-foreground">二进制</span>
                  ) : (
                    <>
                      <span className="text-success">+{c.additions}</span> <span className="text-destructive">−{c.deletions}</span>
                    </>
                  )}
                </span>
              </button>
            </li>
          )
        })}
      </ul>
      {truncated && <p className="px-3 py-2 text-xs text-muted-foreground">改动太多，只列出了前 {files.length} 个文件。</p>}
    </div>
  )
}

/** One directory level, read when it is first shown; folders below load as they open. */
function Directory({ reader, path, depth, onOpen }: { reader: WorkspaceReader; path: string; depth: number; onOpen: (path: string) => void }) {
  const query = useQuery({ queryKey: [...reader.key, 'entries', path], queryFn: () => reader.entries(path), retry: false })
  const indent = { paddingLeft: `${depth * 14 + 4}px` }
  if (query.isPending) {
    return (
      <p className="py-1 text-xs text-muted-foreground" style={indent}>
        正在读取…
      </p>
    )
  }
  if (query.isError) {
    return depth === 0 ? (
      <Failure error={query.error} onRetry={() => void query.refetch()} />
    ) : (
      <p className="py-1 text-xs text-destructive" style={indent}>
        {failureText(query.error)}
      </p>
    )
  }
  if (depth === 0 && query.data.entries.length === 0) return <Message>工作区是空的。</Message>
  return (
    <>
      {query.data.entries.map((e) => (
        <EntryRow key={e.path} entry={e} reader={reader} depth={depth} onOpen={onOpen} />
      ))}
      {query.data.truncated && (
        <p className="py-1 text-xs text-muted-foreground" style={indent}>
          目录太大，只列出了一部分。
        </p>
      )}
    </>
  )
}

function EntryRow({ entry, reader, depth, onOpen }: { entry: WorkspaceEntry; reader: WorkspaceReader; depth: number; onOpen: (path: string) => void }) {
  const indent = { paddingLeft: `${depth * 14 + 4}px` }
  if (entry.kind === 'file') {
    return (
      <button
        type="button"
        role="treeitem"
        className="flex w-full min-w-0 items-center gap-1.5 rounded py-0.5 pr-1 text-left outline-none hover:bg-muted focus-visible:bg-muted"
        style={{ paddingLeft: `${depth * 14 + 22}px` }}
        onClick={() => onOpen(entry.path)}
      >
        <FileIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate font-mono text-xs">{entry.name}</span>
      </button>
    )
  }
  return (
    <Collapsible>
      <CollapsibleTrigger
        role="treeitem"
        className="group/folder flex w-full min-w-0 items-center gap-1.5 rounded py-0.5 pr-1 text-left outline-none hover:bg-muted focus-visible:bg-muted"
        style={indent}
      >
        <ChevronRightIcon className="size-3.5 shrink-0 transition-transform group-aria-expanded/folder:rotate-90" />
        <FolderIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate font-mono text-xs">{entry.name}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Directory reader={reader} path={entry.path} depth={depth + 1} onOpen={onOpen} />
      </CollapsibleContent>
    </Collapsible>
  )
}

/** One file, as its diff against the initial version or as its current content. */
function FileView({
  reader,
  view,
  status,
  onBack,
  onSwitch,
}: {
  reader: WorkspaceReader
  view: View
  status?: WorkspaceChange['status']
  onBack: () => void
  onSwitch: (view: View) => void
}) {
  return (
    <div className="flex flex-col gap-2 p-3">
      <div className="flex min-w-0 items-center gap-1.5">
        <Button size="icon-sm" variant="ghost" aria-label="返回列表" onClick={onBack}>
          <ArrowLeftIcon />
        </Button>
        <span className="min-w-0 flex-1 truncate font-mono text-xs" title={view.path}>
          {view.path}
        </span>
        {view.kind === 'diff' && status !== 'deleted' && (
          <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => onSwitch({ kind: 'file', path: view.path })}>
            <FileTextIcon />
            查看文件
          </Button>
        )}
        {view.kind === 'file' && (
          <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => onSwitch({ kind: 'diff', path: view.path })}>
            <GitCompareArrowsIcon />
            查看改动
          </Button>
        )}
      </div>
      {view.kind === 'diff' ? <DiffContent reader={reader} path={view.path} /> : <FileContent reader={reader} path={view.path} />}
    </div>
  )
}

function DiffContent({ reader, path }: { reader: WorkspaceReader; path: string }) {
  const query = useQuery({ queryKey: [...reader.key, 'diff', path], queryFn: () => reader.diff(path), retry: false })
  const parsed = useMemo(() => (query.data ? parsePatch(query.data.patch) : undefined), [query.data])
  if (query.isPending) return <Message>正在读取改动…</Message>
  if (query.isError) return <Failure error={query.error} onRetry={() => void query.refetch()} />
  if (parsed!.binary) return <Message>二进制文件有改动，无法预览。</Message>
  if (parsed!.lines.length === 0) return <Message>这个文件和初始版本相同。</Message>
  return (
    <CodeBlock size="sm">
      <CodeBlockContent aria-label={`${path} 的改动`}>
        {parsed!.lines.map((l, i) =>
          l.op === '@' ? (
            <CodeBlockLine key={i} className="text-muted-foreground">
              {l.text}
            </CodeBlockLine>
          ) : (
            <CodeBlockLine key={i} variant={l.op === '+' ? 'added' : l.op === '-' ? 'removed' : 'default'}>
              {l.text}
            </CodeBlockLine>
          ),
        )}
      </CodeBlockContent>
    </CodeBlock>
  )
}

function FileContent({ reader, path }: { reader: WorkspaceReader; path: string }) {
  const query = useQuery({ queryKey: [...reader.key, 'file', path], queryFn: () => reader.file(path), retry: false })
  if (query.isPending) return <Message>正在读取文件…</Message>
  if (query.isError) return <Failure error={query.error} onRetry={() => void query.refetch()} />
  if (query.data.content === '') return <Message>空文件。</Message>
  return <HighlightedCode code={query.data.content} language={languageOf(path)} title={path.split('/').at(-1)} />
}
