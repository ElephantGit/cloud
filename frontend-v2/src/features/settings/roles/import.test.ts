import { describe, expect, it } from 'vitest'
import { seed } from '@/lib/settings-demo/seed'
import {
  MAX_ROLE_MARKDOWN_BYTES,
  applyRoleImport,
  parseRoleMarkdown,
  previewRoleImport,
} from './import'

const markdown = `---
name: 发布检查
description: "发布前核对版本和说明"
---

# 发布检查

核对版本和发布说明。`

describe('role Markdown import', () => {
  it('parses desktop-compatible front matter and keeps the body', () => {
    expect(parseRoleMarkdown(markdown)).toEqual({
      name: '发布检查',
      description: '发布前核对版本和说明',
      content: '# 发布检查\n\n核对版本和发布说明。',
    })
  })

  it('supports single quotes and folded descriptions', () => {
    expect(
      parseRoleMarkdown(`---
name: 'Reviewer'
description: >
  检查正确性、
  风险和测试覆盖
---
正文`),
    ).toEqual({
      name: 'Reviewer',
      description: '检查正确性、 风险和测试覆盖',
      content: '正文',
    })
  })

  it.each([
    ['missing delimiter', 'name: x\ndescription: y'],
    ['missing closing delimiter', '---\nname: x\ndescription: y'],
    ['missing name', '---\ndescription: y\n---\nbody'],
    ['missing description', '---\nname: x\n---\nbody'],
    ['too large', `---\nname: x\ndescription: y\n---\n${'x'.repeat(MAX_ROLE_MARKDOWN_BYTES)}`],
  ])('rejects %s', (_name, source) => {
    expect(() => parseRoleMarkdown(source)).toThrow()
  })
})

describe('role import conflicts', () => {
  it('detects a case-insensitive conflict and overwrites only the previewed row', () => {
    const data = seed()
    const preview = previewRoleImport(
      `---
name: 代码审查
description: 新版说明
---
新版内容`,
      'reviewer.md',
      data.roles,
    )
    expect(preview.existing?.id).toBe('role-reviewer')
    expect(() => applyRoleImport(data, preview, null)).toThrow('跳过或覆盖')
    applyRoleImport(data, preview, 'overwrite')
    expect(data.roles.find((role) => role.id === 'role-reviewer')).toMatchObject(
      {
        description: '新版说明',
        content: '新版内容',
      },
    )
  })

  it('skips without changing the row and rejects a stale conflict', () => {
    const data = seed()
    const preview = previewRoleImport(markdown.replace('发布检查', '代码审查'), 'x.md', data.roles)
    const before = data.roles.find((role) => role.id === 'role-reviewer')!
    applyRoleImport(data, preview, 'skip')
    expect(data.roles.find((role) => role.id === 'role-reviewer')).toBe(before)
    before.updatedAt = '2026-01-01T00:00:00.000Z'
    expect(() => applyRoleImport(data, preview, 'overwrite')).toThrow(
      '已发生变化',
    )
  })
})
