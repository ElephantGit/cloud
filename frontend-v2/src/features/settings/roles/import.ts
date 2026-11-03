import type { Role, RoleDraft, Snapshot } from '../data/types'
import { saveRole } from '../data/resource-actions'

export const MAX_ROLE_MARKDOWN_BYTES = 1024 * 1024

export interface RoleImportPreview {
  fileName: string
  draft: RoleDraft
  existing: Role | null
}

export type RoleImportDecision = 'skip' | 'overwrite'

/** Imports desktop's Agent Markdown shape: YAML front matter plus Markdown body. */
export function parseRoleMarkdown(markdown: string): RoleDraft {
  if (new TextEncoder().encode(markdown).length > MAX_ROLE_MARKDOWN_BYTES)
    throw new Error('Markdown 文件不能超过 1 MiB。')
  const normalized = markdown.replace(/\r\n?/g, '\n')
  const lines = normalized.split('\n')
  if (lines[0]?.trim() !== '---')
    throw new Error('Markdown 必须以 YAML front matter 开头。')
  const end = lines.findIndex(
    (line, index) => index > 0 && line.trim() === '---',
  )
  if (end < 1) throw new Error('YAML front matter 缺少结束分隔线。')
  const fields = parseFrontMatter(lines.slice(1, end))
  const name = (fields.name ?? '').trim()
  const description = (fields.description ?? '').trim()
  if (!name) throw new Error('front matter 缺少非空的 name。')
  if (!description)
    throw new Error('front matter 缺少非空的 description。')
  validateRoleFields(name, description)
  const content = lines
    .slice(end + 1)
    .join('\n')
    .replace(/^\n/, '')
    .trimEnd()
  if (content.length > 100000)
    throw new Error('角色内容不能超过 100,000 个字符。')
  return { name, description, content }
}

export function previewRoleImport(
  markdown: string,
  fileName: string,
  roles: readonly Role[],
): RoleImportPreview {
  const draft = parseRoleMarkdown(markdown)
  const conflicting =
    roles.find(
      (role) => role.name.toLowerCase() === draft.name.toLowerCase(),
    ) ?? null
  return {
    fileName,
    draft,
    existing: conflicting === null ? null : { ...conflicting },
  }
}

/**
 * Applies one preview. A conflicting role must still be the same row the user saw, so another
 * edit cannot be overwritten silently between preview and confirmation.
 */
export function applyRoleImport(
  data: Snapshot,
  preview: RoleImportPreview,
  decision: RoleImportDecision | null,
) {
  if (preview.existing) {
    const current = data.roles.find((role) => role.id === preview.existing?.id)
    if (!current || current.updatedAt !== preview.existing.updatedAt)
      throw new Error('目标角色已发生变化，请重新确认处理方式。')
    if (decision === 'skip') return
    if (decision !== 'overwrite')
      throw new Error('请选择跳过或覆盖同名角色。')
    saveRole(data, preview.draft, current.id)
    return
  }
  if (
    data.roles.some(
      (role) =>
        role.name.toLowerCase() === preview.draft.name.trim().toLowerCase(),
    )
  )
    throw new Error('角色列表已发生变化，请重新导入。')
  saveRole(data, preview.draft)
}

function validateRoleFields(name: string, description: string) {
  if (!name || name.length > 80 || /[\r\n/\\]/.test(name))
    throw new Error('标题需为 1–80 个字符，不能包含换行或路径分隔符。')
  if (description.length > 500)
    throw new Error('描述最多 500 个字符。')
}

function parseFrontMatter(lines: string[]): Record<string, string> {
  const fields: Record<string, string> = {}
  let index = 0
  while (index < lines.length) {
    const line = lines[index] ?? ''
    if (line.trim() === '' || line.trimStart().startsWith('#')) {
      index += 1
      continue
    }
    const match = /^([A-Za-z][A-Za-z0-9_-]*):(.*)$/.exec(line)
    if (!match) throw new Error('YAML front matter 只能包含单行键值对。')
    const key = match[1]!
    const raw = match[2]!.trim()
    if (key in fields) throw new Error(`front matter 重复定义 ${key}。`)
    if (raw === '|' || raw === '>' || raw === '|-' || raw === '>-') {
      const block = collectBlock(lines, index + 1)
      if (key === 'name' || key === 'description')
        fields[key] =
          raw.startsWith('>') ? foldBlock(block.lines) : block.lines.join('\n')
      index = block.next
      continue
    }
    if (key === 'name' || key === 'description') fields[key] = scalar(raw)
    index += 1
  }
  return fields
}

function collectBlock(lines: string[], start: number) {
  const collected: string[] = []
  let index = start
  while (index < lines.length) {
    const line = lines[index] ?? ''
    if (line.trim() !== '' && !/^\s/.test(line)) break
    collected.push(line)
    index += 1
  }
  while (collected.length > 0 && collected.at(-1)!.trim() === '')
    collected.pop()
  const first = collected.find((line) => line.trim() !== '')
  const indent = first?.match(/^\s*/)?.[0] ?? ''
  return {
    lines: collected.map((line) =>
      line.startsWith(indent) ? line.slice(indent.length) : line.trimStart(),
    ),
    next: index,
  }
}

function foldBlock(lines: string[]): string {
  let result = ''
  for (const line of lines) {
    if (line.trim() === '') {
      result = result.trimEnd() + '\n'
    } else {
      result += (result === '' || result.endsWith('\n') ? '' : ' ') + line
    }
  }
  return result.trim()
}

function scalar(raw: string): string {
  if (raw.startsWith('"')) {
    if (!raw.endsWith('"')) throw new Error('双引号值缺少结束引号。')
    try {
      const parsed: unknown = JSON.parse(raw)
      if (typeof parsed !== 'string')
        throw new Error('引号值必须是字符串。')
      return parsed
    } catch {
      throw new Error('双引号值不是有效的 JSON 字符串。')
    }
  }
  if (raw.startsWith("'")) {
    if (!raw.endsWith("'")) throw new Error('单引号值缺少结束引号。')
    return raw.slice(1, -1).replaceAll("''", "'")
  }
  return raw.replace(/\s+#.*$/, '').trim()
}
