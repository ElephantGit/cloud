import { beforeEach, describe, expect, it } from 'vitest'
import { DemoRepository } from './repository'
import {
  changeConnection,
  connectionReady,
  deleteRole,
  saveConnection,
  saveRole,
  setModelDefault,
  validateConnection,
} from './resource-actions'
import type { ConnectionDraft } from './types'

const user = { id: 'current', displayName: '当前用户', role: 'owner' as const }
let values: Map<string, string>
const port = () => ({
  getItem: (key: string) => values.get(key) ?? null,
  setItem: (key: string, value: string) => {
    values.set(key, value)
  },
})
const repository = () => new DemoRepository(user, port)
beforeEach(() => {
  values = new Map()
})

describe('settings demo persistence', () => {
  it('persists per user, does not mutate returned snapshots, and keeps another user isolated', async () => {
    const first = repository()
    const state = await first.read()
    await first.update(state.revision, (data) => {
      data.connections[0].enabled = false
    })
    expect(state.connections[0].enabled).toBe(true)
    expect((await repository().read()).connections[0].enabled).toBe(false)
    expect(
      (
        await new DemoRepository(
          { id: 'other' },
          port,
        ).read()
      ).connections[0].enabled,
    ).toBe(true)
  })

  it('rejects a stale write instead of losing an earlier change', async () => {
    const repo = repository()
    const state = await repo.read()
    await repo.update(state.revision, (data) => {
      data.connections[0].enabled = false
    })
    await expect(
      repo.update(state.revision, (data) => {
        data.connections[0].enabled = true
      }),
    ).rejects.toThrow('其他操作')
    expect((await repo.read()).connections[0].enabled).toBe(false)
  })

  it('does not report successful storage when writing fails', async () => {
    const state = await repository().read()
    const repo = new DemoRepository(user, () => ({
      getItem: port().getItem,
      setItem: () => {
        throw new Error('quota')
      },
    }))
    await expect(
      repo.update(state.revision, (data) => {
        data.connections = []
      }),
    ).rejects.toThrow('尚未保存')
    expect((await repository().read()).connections).toHaveLength(
      state.connections.length,
    )
  })

  it('keeps corrupt data until an explicit reset and does not reset another user', async () => {
    const repo = repository()
    const other = new DemoRepository({ id: 'other' }, port)
    await other.read()
    values.set(repo.key, '{"schema":1,"settings":null}')
    await expect(repo.read()).rejects.toThrow('本地设置无法读取')
    expect(values.get(repo.key)).toBe('{"schema":1,"settings":null}')
    await repo.reset()
    expect((await repo.read()).connections.length).toBeGreaterThan(0)
    expect(values.has(other.key)).toBe(true)
  })
})

describe('browser-local settings boundary', () => {
  it('keeps only connections and roles in browser storage', async () => {
    const state = await repository().read()
    expect(state.schema).toBe(5)
    expect(state.roles.map((role) => role.name)).toEqual([
      '代码审查',
      '技术写作',
    ])
    expect(state).not.toHaveProperty('skills')
    expect(state).not.toHaveProperty('catalog')
    expect(state).not.toHaveProperty('installations')
    for (const field of ['members', 'history', 'links', 'requests'])
      expect(state).not.toHaveProperty(field)
  })

  it('upgrades schemas 2, 3 and 4 while dropping skills and retaining roles', async () => {
    const repo = repository()
    const initial = await repo.read()
    const skills = [
      {
        id: 'local-stale',
        namespace: 'local',
        name: 'stale-local',
        description: '',
        content: '',
        source: { kind: 'local' },
        availability: 'available',
      },
    ]

    values.set(
      repo.key,
      JSON.stringify({
        ...initial,
        schema: 2,
        catalog: [{ id: 'stale' }],
        installations: [{ id: 'stale-selection' }],
        skills,
      }),
    )
    let upgraded = await repo.read()
    expect(upgraded.schema).toBe(5)
    expect(upgraded.roles).toHaveLength(2)
    expect(upgraded).not.toHaveProperty('skills')
    expect(upgraded).not.toHaveProperty('catalog')
    expect(upgraded).not.toHaveProperty('installations')
    expect(JSON.parse(values.get(repo.key) ?? '{}').schema).toBe(5)

    values.set(
      repo.key,
      JSON.stringify({ ...initial, schema: 3, roles: undefined, skills }),
    )
    upgraded = await repo.read()
    expect(upgraded.schema).toBe(5)
    expect(upgraded.roles).toHaveLength(2)
    expect(upgraded).not.toHaveProperty('skills')

    const customRole = {
      id: 'custom-role',
      namespace: 'local',
      name: '自定义角色',
      description: '保留',
      content: '# 自定义',
      updatedAt: '2026-11-02T08:00:00.000Z',
    }
    values.set(
      repo.key,
      JSON.stringify({
        ...initial,
        schema: 4,
        roles: [customRole],
        skills,
      }),
    )
    upgraded = await repo.read()
    expect(upgraded.schema).toBe(5)
    expect(upgraded.roles).toEqual([customRole])
    expect(upgraded).not.toHaveProperty('skills')
  })
})

describe('roles', () => {
  it('supports local editing, deletion and isolation between users', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      saveRole(data, {
        name: '发布检查',
        description: '发布前核对版本和说明',
        content: '# 发布检查\n\n核对版本。',
      }),
    )
    const role = state.roles.find((candidate) => candidate.name === '发布检查')
    expect(role?.namespace).toBe('local')
    expect(
      (
        await new DemoRepository(
          { id: 'other' },
          port,
        ).read()
      ).roles.some((candidate) => candidate.name === '发布检查'),
    ).toBe(false)
    await expect(
      repo.update(state.revision, (data) =>
        saveRole(data, {
          name: '发布检查',
          description: '',
          content: '',
        }),
      ),
    ).rejects.toThrow('同名')
    state = await repo.update(state.revision, (data) =>
      deleteRole(data, role!.id),
    )
    expect(state.roles.some((candidate) => candidate.id === role!.id)).toBe(
      false,
    )
  })
})

describe('model metadata without credentials', () => {
  const draft: ConnectionDraft = {
    name: '连接',
    protocol: 'openai-completions',
    authMode: 'bearer',
    baseUrl: 'https://models.example.com/v1',
    enabled: true,
    models: [
      { id: 'org/model', name: '模型', contextWindow: 64000, maxTokens: 8192 },
    ],
  }
  it('preserves slash model ids and allows defaults only for ready connections', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      saveConnection(data, draft),
    )
    const connection = state.connections.at(-1)!
    expect(connection.models[0].id).toBe('org/model')
    await expect(
      repo.update(state.revision, (data) =>
        setModelDefault(data, connection.id, 'org/model'),
      ),
    ).rejects.toThrow('凭据')
    state = await repo.update(state.revision, (data) =>
      changeConnection(data, connection.id, 'credential'),
    )
    state = await repo.update(state.revision, (data) =>
      setModelDefault(data, connection.id, 'org/model'),
    )
    expect(connectionReady(state)).toBe(true)
    state = await repo.update(state.revision, (data) =>
      changeConnection(data, connection.id, 'toggle'),
    )
    expect(connectionReady(state)).toBe(false)
    state = await repo.update(state.revision, (data) =>
      changeConnection(data, connection.id, 'delete'),
    )
    expect(connectionReady(state)).toBe(false)
    expect(values.get(repo.key)).not.toMatch(/apiKey|secret|credentialValue/)
  })
  it('rejects stale connection edits while preserving latest metadata', async () => {
    const repo = repository()
    let state = await repo.read()
    const connection = state.connections[0]
    state = await repo.update(state.revision, (data) =>
      changeConnection(data, connection.id, 'toggle'),
    )
    await expect(
      repo.update(state.revision, (data) =>
        saveConnection(data, draft, connection.id, connection.version),
      ),
    ).rejects.toThrow('草稿已保留')
    expect((await repo.read()).connections[0].enabled).toBe(false)
  })
  it.each([
    { ...draft, baseUrl: 'http://example.com' },
    { ...draft, baseUrl: 'https://name:secret@example.com' },
    { ...draft, models: [] },
    { ...draft, models: [draft.models[0], draft.models[0]] },
    { ...draft, models: [{ ...draft.models[0], maxTokens: 64001 }] },
    { ...draft, authMode: 'x-api-key' as const },
  ])(
    'validates contract metadata without accessing the network: %j',
    (invalid) => {
      expect(validateConnection(invalid)).toBeTypeOf('string')
    },
  )
})
