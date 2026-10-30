import { beforeEach, describe, expect, it } from 'vitest'
import { DemoRepository } from './repository'
import {
  createLink,
  decideRequest,
  revokeLink,
  updateMember,
} from './member-actions'
import {
  changeConnection,
  connectionReady,
  deleteSkill,
  saveConnection,
  saveSkill,
  setModelDefault,
  setPluginInstalled,
  validateConnection,
} from './resource-actions'
import type { ConnectionDraft } from './types'

const user = { id: 'current', displayName: '当前用户' }
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
    await first.update(state.revision, (data) =>
      saveSkill(data, {
        name: 'custom',
        description: '',
        content: '# 私有演示',
      }),
    )
    expect(state.skills.some((s) => s.name === 'custom')).toBe(false)
    expect(
      (await repository().read()).skills.some((s) => s.name === 'custom'),
    ).toBe(true)
    expect(
      (
        await new DemoRepository(
          { id: 'other', displayName: '另一用户' },
          port,
        ).read()
      ).skills.some((s) => s.name === 'custom'),
    ).toBe(false)
  })
  it('rejects a stale write instead of losing an earlier change', async () => {
    const repo = repository()
    const state = await repo.read()
    await repo.update(state.revision, (data) =>
      saveSkill(data, { name: 'first', description: '', content: '' }),
    )
    await expect(
      repo.update(state.revision, (data) =>
        saveSkill(data, { name: 'lost', description: '', content: '' }),
      ),
    ).rejects.toThrow('其他操作')
    expect((await repo.read()).skills.map((s) => s.name)).toContain('first')
    expect((await repo.read()).skills.map((s) => s.name)).not.toContain('lost')
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
        data.skills = []
      }),
    ).rejects.toThrow('尚未保存')
    expect((await repository().read()).skills).toHaveLength(state.skills.length)
  })
  it('keeps corrupt data until an explicit reset and does not reset another user', async () => {
    const repo = repository()
    const other = new DemoRepository(
      { id: 'other', displayName: '另一用户' },
      port,
    )
    await other.read()
    values.set(repo.key, '{"schema":1,"skills":null}')
    await expect(repo.read()).rejects.toThrow('重置')
    expect(values.get(repo.key)).toBe('{"schema":1,"skills":null}')
    await repo.reset()
    expect((await repo.read()).skills.length).toBeGreaterThan(0)
    expect(values.has(other.key)).toBe(true)
  })
})

describe('plugins and document skills', () => {
  it('joins by package identity, keeps separate record ids, and restores package skills', async () => {
    const repo = repository()
    let state = await repo.read()
    const catalog = state.catalog.find((p) => p.identifier === 'writing-kit')!
    expect(
      state.installations.find((p) => p.identifier === 'writing-kit')!.id,
    ).not.toBe(catalog.id)
    state = await repo.update(state.revision, (data) =>
      setPluginInstalled(data, user.id, 'demo/writing-kit', false),
    )
    expect(
      state.skills.find((s) => s.source.kind === 'plugin')?.availability,
    ).toBe('unavailable')
    expect(
      state.skills
        .filter((s) => s.source.kind === 'local')
        .every((s) => s.availability === 'available'),
    ).toBe(true)
    state = await repo.update(state.revision, (data) =>
      setPluginInstalled(data, user.id, 'demo/writing-kit', true),
    )
    expect(state.skills.filter((s) => s.source.kind === 'plugin')).toHaveLength(
      1,
    )
    expect(
      state.skills.find((s) => s.source.kind === 'plugin')?.availability,
    ).toBe('available')
  })
  it('clears an installation failure on retry and forbids unsupported pack installation', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      setPluginInstalled(data, user.id, 'demo/browser-tools', true),
    )
    expect(
      state.installations.find((p) => p.identifier === 'browser-tools'),
    ).toMatchObject({
      desiredState: 'installed',
      observedState: 'installed',
      installError: null,
    })
    await expect(
      repo.update(state.revision, (data) =>
        setPluginInstalled(data, user.id, 'demo/starter-pack', true),
      ),
    ).rejects.toThrow('不支持')
  })
  it('supports local editing and deletion but refuses changes to package content', async () => {
    const repo = repository()
    let state = await repo.read()
    const packageSkill = state.skills.find((s) => s.source.kind === 'plugin')!
    await expect(
      repo.update(state.revision, (data) => deleteSkill(data, packageSkill.id)),
    ).rejects.toThrow('自建')
    await expect(
      repo.update(state.revision, (data) =>
        saveSkill(
          data,
          { name: 'changed', description: '', content: '' },
          packageSkill.id,
        ),
      ),
    ).rejects.toThrow('只读')
    state = await repo.update(state.revision, (data) =>
      saveSkill(
        data,
        { name: 'test-plan', description: '修改说明', content: '# 新版' },
        'local-test-plan',
      ),
    )
    expect(state.skills.find((s) => s.id === 'local-test-plan')?.content).toBe(
      '# 新版',
    )
    await expect(
      repo.update(state.revision, (data) =>
        saveSkill(data, { name: 'TEST-PLAN', description: '', content: '' }),
      ),
    ).rejects.toThrow('同名')
    state = await repo.update(state.revision, (data) =>
      deleteSkill(data, 'local-test-plan'),
    )
    expect(state.skills.some((s) => s.id === 'local-test-plan')).toBe(false)
  })
})

describe('member administration', () => {
  it('protects the final admin and prevents a readonly member from managing plugins or links', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      updateMember(data, user, 'demo-lin', 'member'),
    )
    await expect(
      repo.update(state.revision, (data) =>
        updateMember(data, user, user.id, 'disabled'),
      ),
    ).rejects.toThrow('最后')
    await expect(
      repo.update(state.revision, (data) =>
        updateMember(data, user, user.id, 'member'),
      ),
    ).rejects.toThrow('最后')
    const memberActor = { id: 'demo-chen', displayName: '普通成员' }
    await expect(
      repo.update(state.revision, (data) =>
        createLink(data, memberActor, 'invite', ''),
      ),
    ).rejects.toThrow('管理员')
    await expect(
      repo.update(state.revision, (data) =>
        setPluginInstalled(data, memberActor.id, 'demo/review-kit', true),
      ),
    ).rejects.toThrow('管理员')
  })
  it('keeps disabled members and history and refuses direct restoration', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      updateMember(data, user, 'demo-chen', 'disabled'),
    )
    expect(state.members.find((m) => m.userId === 'demo-chen')?.status).toBe(
      'disabled',
    )
    expect(state.history.some((h) => h.userId === 'demo-chen')).toBe(true)
    await expect(
      repo.update(state.revision, (data) =>
        updateMember(data, user, 'demo-chen', 'member'),
      ),
    ).rejects.toThrow('重新申请')
  })
  it('approves once, updates roster and history, and persists link revocation', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      decideRequest(data, user, 'demo-request', true),
    )
    expect(state.members.find((m) => m.userId === 'demo-xu')).toMatchObject({
      status: 'active',
      role: 'member',
    })
    expect(state.history.find((h) => h.userId === 'demo-xu')?.requestId).toBe(
      'demo-request',
    )
    await expect(
      repo.update(state.revision, (data) =>
        decideRequest(data, user, 'demo-request', true),
      ),
    ).rejects.toThrow('已处理')
    let id = ''
    state = await repo.update(state.revision, (data) => {
      id = createLink(data, user, 'apply', '新链接')
    })
    state = await repo.update(state.revision, (data) =>
      revokeLink(data, user, id),
    )
    expect(state.links.find((l) => l.id === id)?.revokedAt).toBeTruthy()
    expect(
      (await repository().read()).links.find((l) => l.id === id)?.note,
    ).toBe('新链接')
  })
  it('rejecting an application does not add a member', async () => {
    const repo = repository()
    let state = await repo.read()
    state = await repo.update(state.revision, (data) =>
      decideRequest(data, user, 'demo-request', false),
    )
    expect(state.requests[0].status).toBe('rejected')
    expect(state.members.some((m) => m.userId === 'demo-xu')).toBe(false)
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
    ).rejects.toThrow('演示凭据')
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
