import { childSessionOf } from './conversation'
import type { ContentItem, MessageItem, PlanItem, ThoughtItem, ToolItem, Turn, TurnItem } from './types'

/** A delegated agent run, with its own rows and possibly its own children. */
export interface SubagentGroup {
  kind: 'subagent'
  id: string
  title?: string
  rows: Row[]
  /** Whether the child is still producing output. */
  live: boolean
}

/** Consecutive read and search calls, shown as one exploration step. */
export interface ExploreGroup {
  kind: 'explore'
  id: string
  tools: ToolItem[]
}

export type ActivityEntry =
  | ThoughtItem
  | PlanItem
  | (ToolItem & { subagent?: SubagentGroup })
  | ExploreGroup
  | SubagentGroup

/** One block in a turn: the agent's text, an inline attachment, or a run of work between texts. */
export type Row =
  | MessageItem
  | ContentItem
  | { kind: 'activity'; id: string; entries: ActivityEntry[] }

const exploring = new Set(['read', 'search', 'fetch'])

/**
 * Arranges a turn's items for display. Items a child session produced become a subagent group,
 * placed at the parent tool call that started it when the provider links one, otherwise where
 * the child first spoke. Everything between two runs of agent text is one activity block.
 * Resource links are left out: they are rendered once per turn as sources.
 */
export function buildRows(turn: Turn): Row[] {
  const groups = collectGroups(turn.items)
  return rowsFor(undefined, turn.items, groups, turn.status === 'streaming')
}

/** The resource links a turn cited, in the order the agent sent them. */
export function turnSources(turn: Turn) {
  return turn.items.flatMap((i) => (i.kind === 'content' && i.block.type === 'resource_link' ? [i.block] : []))
}

/** A file one turn changed, with line counts taken from the diffs its tool calls reported. */
export interface FileChange {
  path: string
  added: number
  removed: number
  created: boolean
}

/** Every file the turn's diffs touched; a later diff of the same file adds to the earlier one. */
export function turnFileChanges(turn: Turn): FileChange[] {
  const byPath = new Map<string, FileChange>()
  for (const item of turn.items) {
    if (item.kind !== 'tool') continue
    for (const c of item.content) {
      if (c.type !== 'diff') continue
      const { added, removed } = countLines(c.oldText ?? '', c.newText)
      const prev = byPath.get(c.path)
      byPath.set(c.path, {
        path: c.path,
        added: (prev?.added ?? 0) + added,
        removed: (prev?.removed ?? 0) + removed,
        created: prev?.created ?? c.oldText == null,
      })
    }
  }
  return [...byPath.values()]
}

/** Lines only in `next` and lines only in `prev`, counted as multisets; enough for a summary. */
export function countLines(prev: string, next: string): { added: number; removed: number } {
  const split = (s: string) => (s === '' ? [] : s.replace(/\n$/, '').split('\n'))
  const counts = new Map<string, number>()
  for (const line of split(prev)) counts.set(line, (counts.get(line) ?? 0) + 1)
  let added = 0
  for (const line of split(next)) {
    const n = counts.get(line) ?? 0
    if (n > 0) counts.set(line, n - 1)
    else added++
  }
  const removed = [...counts.values()].reduce((a, b) => a + b, 0)
  return { added, removed }
}

interface GroupInfo {
  id: string
  parentId: string
  title?: string
  /** The tool call in the parent that started this child, when the provider links one. */
  linkedTool?: string
}

function collectGroups(items: TurnItem[]): Map<string, GroupInfo> {
  const groups = new Map<string, GroupInfo>()
  for (const item of items) {
    const child = childSessionOf(item.meta)
    if (!child) continue
    const g = groups.get(child.id) ?? { id: child.id, parentId: child.parentId }
    g.title = child.title ?? g.title
    groups.set(child.id, g)
  }
  for (const item of items) {
    const linked = item.kind === 'tool' ? delegatedSession(item) : undefined
    const g = linked && groups.get(linked)
    if (g) g.linkedTool = item.id
  }
  return groups
}

/** OpenCode names the delegated child in `rawOutput.metadata.sessionID`; nothing else links them. */
function delegatedSession(tool: ToolItem): string | undefined {
  const output = tool.rawOutput as { metadata?: { sessionID?: unknown } } | undefined
  const id = output?.metadata?.sessionID
  return typeof id === 'string' ? id : undefined
}

/** The group an item's source sits under, as seen from `context` (undefined is the parent). */
function sourceIn(item: TurnItem, context: string | undefined, groups: Map<string, GroupInfo>): string | undefined | null {
  let id = childSessionOf(item.meta)?.id
  // Walk up from the item's own session until the next step up is the context we are drawing.
  while (id !== undefined) {
    if (id === context) return id
    const g = groups.get(id)!
    const parent = groups.has(g.parentId) ? g.parentId : undefined
    if (parent === context) return id
    id = parent
  }
  return context === undefined ? undefined : null
}

function rowsFor(context: string | undefined, items: TurnItem[], groups: Map<string, GroupInfo>, live: boolean): Row[] {
  const entries: (TurnItem | SubagentGroup)[] = []
  const placed = new Set<string>()
  const groupFor = (id: string): SubagentGroup => ({
    kind: 'subagent',
    id,
    title: groups.get(id)!.title,
    rows: rowsFor(id, items, groups, live),
    live,
  })

  for (const item of items) {
    const source = sourceIn(item, context, groups)
    if (source === null) continue
    if (source === context || source === undefined) {
      entries.push(item)
      continue
    }
    if (placed.has(source)) continue
    placed.add(source)
    // A linked child is drawn inside its tool call instead.
    if (!groups.get(source)!.linkedTool) entries.push(groupFor(source))
  }

  const rows: Row[] = []
  let activity: ActivityEntry[] = []
  const flush = () => {
    if (activity.length) rows.push({ kind: 'activity', id: `activity-${rows.length}`, entries: clusterExploration(activity) })
    activity = []
  }
  for (const entry of entries) {
    if (entry.kind === 'message') {
      flush()
      rows.push(entry)
    } else if (entry.kind === 'content') {
      if (entry.block.type === 'resource_link') continue
      flush()
      rows.push(entry)
    } else if (entry.kind === 'tool') {
      const child = [...groups.values()].find((g) => g.linkedTool === entry.id)
      activity.push(child ? { ...entry, subagent: groupFor(child.id) } : entry)
    } else {
      activity.push(entry)
    }
  }
  flush()
  return rows
}

function clusterExploration(entries: ActivityEntry[]): ActivityEntry[] {
  const out: ActivityEntry[] = []
  let run: ToolItem[] = []
  const flush = () => {
    if (run.length >= 2) out.push({ kind: 'explore', id: `explore-${run[0].id}`, tools: run })
    else out.push(...run)
    run = []
  }
  for (const entry of entries) {
    if (entry.kind === 'tool' && !entry.subagent && entry.toolKind && exploring.has(entry.toolKind)) {
      run.push(entry)
      continue
    }
    flush()
    out.push(entry)
  }
  flush()
  return out
}

/** A directory in the changed-files tree. */
export interface Folder {
  name: string
  path: string
  folders: Folder[]
  files: FileChange[]
}

/** Builds the directory tree of the changed files, collapsing single-child folder chains. */
export function fileTree(changes: FileChange[]): Folder {
  const root: Folder = { name: '', path: '', folders: [], files: [] }
  for (const c of changes) {
    const parts = c.path.split('/')
    let dir = root
    for (const part of parts.slice(0, -1)) {
      let next = dir.folders.find((f) => f.name === part)
      if (!next) {
        next = { name: part, path: dir.path ? `${dir.path}/${part}` : part, folders: [], files: [] }
        dir.folders.push(next)
      }
      dir = next
    }
    dir.files.push(c)
  }
  const squash = (f: Folder): Folder => {
    const folders = f.folders.map(squash)
    if (f.name && f.files.length === 0 && folders.length === 1) {
      const only = folders[0]
      return { ...only, name: `${f.name}/${only.name}` }
    }
    return { ...f, folders }
  }
  return squash(root)
}
