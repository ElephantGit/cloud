import { seed } from './seed'
import { DEMO_SPACE, type DemoUser, type Snapshot } from './types'

type StoragePort = Pick<Storage, 'getItem' | 'setItem'>
/** Small local boundary: failed writes never become successful UI state. No HTTP or credentials. */
export class DemoRepository {
  readonly key: string
  readonly user: DemoUser
  private readonly storage: () => StoragePort
  constructor(
    user: DemoUser,
    storage: () => StoragePort = () => window.localStorage,
  ) {
    this.user = user
    this.storage = storage
    this.key = `ora-v2-settings-demo:v1:${encodeURIComponent(user.id)}:${DEMO_SPACE}`
  }
  async read(): Promise<Snapshot> {
    return this.readNow()
  }
  private readNow(): Snapshot {
    let raw: string | null
    try {
      raw = this.storage().getItem(this.key)
    } catch {
      throw new Error('无法读取浏览器演示数据，请检查存储权限后重试。')
    }
    if (raw === null) {
      const initial = seed(this.user)
      this.write(initial)
      return initial
    }
    try {
      const value: unknown = JSON.parse(raw)
      if (!isSnapshot(value)) throw new Error('invalid snapshot')
      return value
    } catch {
      throw new Error(
        '演示数据无法读取。请重试，或通过“重置演示”恢复示例；重置会清除本机演示修改。',
      )
    }
  }
  async update(
    revision: number,
    change: (data: Snapshot) => void,
  ): Promise<Snapshot> {
    const latest = this.readNow()
    if (latest.revision !== revision)
      throw new Error('演示数据已在其他操作中更新，请核对最新数据后重试。')
    change(latest)
    latest.revision += 1
    this.write(latest)
    return latest
  }
  async reset(): Promise<Snapshot> {
    const initial = seed(this.user)
    initial.revision = Date.now()
    this.write(initial)
    return initial
  }
  private write(value: Snapshot) {
    try {
      this.storage().setItem(this.key, JSON.stringify(value))
    } catch {
      throw new Error(
        '未能保存到浏览器。请检查可用空间和存储权限，修改尚未保存。',
      )
    }
  }
}

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}
function rows(value: unknown, fields: string[]) {
  return (
    Array.isArray(value) &&
    value.every(
      (row) =>
        record(row) && fields.every((field) => typeof row[field] === 'string'),
    )
  )
}
/** Check persisted shape before exposing it to renderers; damaged data is never silently replaced. */
function isSnapshot(value: unknown): value is Snapshot {
  if (
    !record(value) ||
    value.schema !== 1 ||
    !Number.isSafeInteger(value.revision)
  )
    return false
  if (
    !rows(value.members, ['id', 'userId', 'displayName', 'role', 'status']) ||
    !rows(value.history, ['id', 'userId', 'method', 'joinedAt']) ||
    !rows(value.links, ['id', 'kind', 'expiresAt']) ||
    !rows(value.requests, ['id', 'userId', 'status']) ||
    !rows(value.catalog, [
      'id',
      'identifier',
      'sourceNamespace',
      'title',
      'kind',
      'version',
      'description',
    ]) ||
    !rows(value.installations, [
      'id',
      'identifier',
      'sourceNamespace',
      'desiredState',
      'observedState',
    ]) ||
    !rows(value.skills, [
      'id',
      'name',
      'namespace',
      'description',
      'content',
      'availability',
    ]) ||
    !rows(value.connections, ['id', 'name', 'protocol', 'authMode', 'baseUrl'])
  )
    return false
  return (
    Array.isArray(value.skills) &&
    value.skills.every(
      (s) =>
        record(s) &&
        record(s.source) &&
        (s.source.kind === 'local' ||
          (s.source.kind === 'plugin' &&
            typeof s.source.pluginId === 'string')),
    ) &&
    Array.isArray(value.connections) &&
    value.connections.every(
      (c) => record(c) && rows(c.models, ['id', 'name']),
    ) &&
    record(value.modelDefault) &&
    typeof value.modelDefault.connectionId === 'string' &&
    typeof value.modelDefault.modelId === 'string'
  )
}
