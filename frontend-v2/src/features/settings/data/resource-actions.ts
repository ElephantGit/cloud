import { pluginSkill } from './seed'
import {
  DEMO_SPACE,
  DEMO_TENANT,
  pluginKey,
  type ConnectionDraft,
  type SkillDraft,
  type Snapshot,
} from './types'

export function requireAdmin(data: Snapshot, userId: string) {
  if (
    !data.members.some(
      (m) => m.userId === userId && m.role === 'admin' && m.status === 'active',
    )
  )
    throw new Error('需要演示工作空间管理员权限。')
}
export function setPluginInstalled(
  data: Snapshot,
  userId: string,
  key: string,
  installed: boolean,
) {
  requireAdmin(data, userId)
  const entry = data.catalog.find((p) => pluginKey(p) === key)
  if (!entry || entry.kind === 'agent' || entry.kind === 'pack')
    throw new Error('此插件暂不支持安装。')
  const now = new Date().toISOString()
  let selection = data.installations.find((p) => pluginKey(p) === key)
  if (!selection) {
    selection = {
      id: crypto.randomUUID(),
      spaceId: DEMO_SPACE,
      tenantId: DEMO_TENANT,
      sourceNamespace: entry.sourceNamespace,
      identifier: entry.identifier,
      kind: entry.kind,
      desiredState: 'removed',
      desiredVersion: entry.version,
      observedState: 'removed',
      observedVersion: null,
      installError: null,
      version: 0,
      createdAt: now,
      updatedAt: now,
    }
    data.installations.push(selection)
  }
  Object.assign(selection, {
    desiredState: installed ? 'installed' : 'removed',
    desiredVersion: entry.version,
    observedState: installed ? 'installed' : 'removed',
    observedVersion: installed ? entry.version : null,
    installError: null,
    updatedAt: now,
    version: selection.version + 1,
  })
  if (entry.kind === 'skill') {
    const supplied = data.skills.filter(
      (s) => s.source.kind === 'plugin' && s.source.pluginId === key,
    )
    if (installed && supplied.length === 0) data.skills.push(pluginSkill(key))
    supplied.forEach((s) => {
      s.availability = installed ? 'available' : 'unavailable'
    })
  }
}

export function saveSkill(data: Snapshot, draft: SkillDraft, id?: string) {
  const current = id ? data.skills.find((s) => s.id === id) : undefined
  if (id && !current) throw new Error('技能已不存在，请刷新列表。')
  if (current?.source.kind === 'plugin')
    throw new Error('插件提供的技能只读，请管理来源插件。')
  const name = draft.name.trim()
  if (!name || name.length > 80 || /[\r\n/\\]/.test(name))
    throw new Error('名称需为 1–80 个字符，不能包含换行或路径分隔符。')
  if (
    data.skills.some(
      (s) =>
        s.id !== id &&
        s.namespace === 'local' &&
        s.name.toLowerCase() === name.toLowerCase(),
    )
  )
    throw new Error('已有同名自建技能，请使用其他名称。')
  if (draft.description.length > 500 || draft.content.length > 100000)
    throw new Error('简介最多 500 字，正文最多 100,000 字。')
  const fields = {
    name,
    description: draft.description.trim(),
    content: draft.content,
  }
  if (current) Object.assign(current, fields)
  else
    data.skills.unshift({
      id: crypto.randomUUID(),
      namespace: 'local',
      source: { kind: 'local' },
      availability: 'available',
      ...fields,
    })
}
export function deleteSkill(data: Snapshot, id: string) {
  const skill = data.skills.find((s) => s.id === id)
  if (!skill || skill.source.kind !== 'local')
    throw new Error('只能删除自建技能。')
  data.skills = data.skills.filter((s) => s.id !== id)
}

export function validateConnection(draft: ConnectionDraft): string | undefined {
  if (!draft.name.trim() || draft.name.length > 120)
    return '连接名称需为 1–120 个字符。'
  if (!['openai-completions', 'anthropic-messages'].includes(draft.protocol))
    return '请选择支持的协议。'
  if (
    !['bearer', 'x-api-key'].includes(draft.authMode) ||
    (draft.protocol === 'openai-completions' && draft.authMode !== 'bearer')
  )
    return '此协议不支持所选认证方式。'
  try {
    const url = new URL(draft.baseUrl)
    if (
      url.protocol !== 'https:' ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      url.hostname === 'localhost' ||
      !url.hostname.includes('.') ||
      /^[\d.]+$/.test(url.hostname) ||
      url.hostname.startsWith('[')
    )
      return '服务地址需为公共 HTTPS 地址，不能包含账号、查询参数或片段。'
  } catch {
    return '请输入有效的 HTTPS 服务地址。'
  }
  if (draft.models.length === 0 || draft.models.length > 100)
    return '连接需包含 1–100 个模型。'
  const ids = new Set<string>()
  for (const model of draft.models) {
    if (
      !model.id.trim() ||
      /[\r\n]/.test(model.id) ||
      ids.has(model.id.trim()) ||
      !model.name.trim()
    )
      return '每个模型需要名称和不重复的标识。'
    ids.add(model.id.trim())
    if (
      !Number.isSafeInteger(model.contextWindow) ||
      !Number.isSafeInteger(model.maxTokens) ||
      model.maxTokens <= 0 ||
      model.contextWindow < model.maxTokens
    )
      return '输出上限需为正整数，且不能超过上下文上限。'
  }
}
export function saveConnection(
  data: Snapshot,
  draft: ConnectionDraft,
  id?: string,
  expectedVersion?: number,
) {
  const error = validateConnection(draft)
  if (error) throw new Error(error)
  const current = id ? data.connections.find((c) => c.id === id) : undefined
  if (id && !current) throw new Error('连接已不存在，请刷新列表。')
  if (
    current &&
    expectedVersion !== undefined &&
    current.version !== expectedVersion
  )
    throw new Error(
      '连接已在别处修改，草稿已保留。请关闭编辑器，核对最新连接后重新编辑。',
    )
  const fields = {
    ...draft,
    name: draft.name.trim(),
    baseUrl: draft.baseUrl.trim(),
    models: draft.models.map((m) => ({
      ...m,
      id: m.id.trim(),
      name: m.name.trim(),
    })),
    updatedAt: new Date().toISOString(),
  }
  if (current) Object.assign(current, fields, { version: current.version + 1 })
  else
    data.connections.push({
      ...fields,
      id: crypto.randomUUID(),
      version: 1,
      createdAt: fields.updatedAt,
      credentialConfigured: false,
    })
}
export function connectionReady(data: Snapshot) {
  const { connectionId, modelId } = data.modelDefault
  return data.connections.some(
    (c) =>
      c.id === connectionId &&
      c.enabled &&
      c.credentialConfigured &&
      c.models.some((m) => m.id === modelId),
  )
}
export function setModelDefault(
  data: Snapshot,
  connectionId: string,
  modelId: string,
) {
  if (
    !data.connections.some(
      (c) =>
        c.id === connectionId &&
        c.enabled &&
        c.credentialConfigured &&
        c.models.some((m) => m.id === modelId),
    )
  )
    throw new Error('请选择已启用且配置了演示凭据的连接与模型。')
  data.modelDefault = {
    connectionId,
    modelId,
    version: data.modelDefault.version + 1,
  }
}
export function changeConnection(
  data: Snapshot,
  id: string,
  action: 'toggle' | 'credential' | 'delete',
) {
  const connection = data.connections.find((c) => c.id === id)
  if (!connection) throw new Error('连接已不存在。')
  if (action === 'delete')
    data.connections = data.connections.filter((c) => c.id !== id)
  else {
    if (action === 'toggle') connection.enabled = !connection.enabled
    else connection.credentialConfigured = !connection.credentialConfigured
    connection.version++
    connection.updatedAt = new Date().toISOString()
  }
}
