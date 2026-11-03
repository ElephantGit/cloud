import { describe, expect, it } from 'vitest'
import type { Conversation, Turn } from '@/components/agent-thread/model/types'
import { parsePatch } from './patch'
import { conversationRevision } from './revision'
import { languageOf, relativePath } from './source'

describe('parsePatch', () => {
  it('drops the file header and keeps hunks line by line', () => {
    const patch = [
      'diff --git a/a.ts b/a.ts',
      'index 1..2 100644',
      '--- a/a.ts',
      '+++ b/a.ts',
      '@@ -1,2 +1,2 @@',
      ' keep',
      '-old',
      '+new',
      '\\ No newline at end of file',
      '',
    ].join('\n')
    expect(parsePatch(patch)).toEqual({
      binary: false,
      lines: [
        { op: '@', text: '@@ -1,2 +1,2 @@' },
        { op: ' ', text: 'keep' },
        { op: '-', text: 'old' },
        { op: '+', text: 'new' },
      ],
    })
  })

  it('reports a binary change and an unchanged file', () => {
    expect(parsePatch('diff --git a/x b/x\nnew file mode 100644\nBinary files /dev/null and b/x differ\n')).toEqual({ binary: true, lines: [] })
    expect(parsePatch('')).toEqual({ binary: false, lines: [] })
  })
})

describe('relativePath', () => {
  const changes = [
    { path: 'src/app.ts', status: 'modified' as const, additions: 1, deletions: 0, binary: false },
    { path: 'app.ts', status: 'added' as const, additions: 1, deletions: 0, binary: false },
  ]
  it('matches an absolute Agent path to the longest changed path it ends with', () => {
    expect(relativePath('/data/nodes/abc/w/r/c-1/src/app.ts', changes)).toBe('src/app.ts')
    expect(relativePath('src/app.ts', changes)).toBe('src/app.ts')
  })
  it('falls back to the checkout layout, then to the path itself', () => {
    expect(relativePath('/data/nodes/abc/w/r/c-1/docs/new.md', changes)).toBe('docs/new.md')
    expect(relativePath('/other/place.md')).toBe('other/place.md')
  })
})

it('picks a highlighter language by extension', () => {
  expect(languageOf('src/a.tsx')).toBe('tsx')
  expect(languageOf('README.md')).toBe('markdown')
  expect(languageOf('Makefile')).toBe('')
})

describe('conversationRevision', () => {
  const turn = (items: Turn['items'], status: Turn['status'] = 'streaming'): Turn => ({
    id: 't1',
    prompt: { text: '改一下', attachments: [], plugins: [] },
    items,
    status,
    startedAt: 0,
  })
  const tool = (toolKind: string, status: 'pending' | 'completed') =>
    ({ kind: 'tool', id: toolKind + status, title: toolKind, toolKind, status, content: [], locations: [], startedAt: 0 }) as Turn['items'][number]
  const conversation = (turns: Turn[]) => ({ turns, markers: [], permissions: [] }) as unknown as Conversation

  it('changes when a writing tool finishes or the turn ends, not on reads', () => {
    const start = conversationRevision(conversation([turn([tool('edit', 'pending')])]))
    expect(conversationRevision(conversation([turn([tool('edit', 'pending'), tool('read', 'completed')])]))).toBe(start)
    const edited = conversationRevision(conversation([turn([tool('edit', 'completed')])]))
    expect(edited).not.toBe(start)
    expect(conversationRevision(conversation([turn([tool('edit', 'completed')], 'completed')]))).not.toBe(edited)
  })
})
