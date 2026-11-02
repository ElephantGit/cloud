import type {
  ModelConnection,
  ModelDefault,
} from '@/api/generated.schemas'
import { seed, seedRoles } from './seed'
import {
  DEMO_SPACE,
  type Role,
  type Skill,
  type Snapshot,
} from './types'

type StoragePort = Pick<Storage, 'getItem' | 'setItem'>
/** Small local boundary: failed writes never become successful UI state. No HTTP or credentials. */
export class DemoRepository {
  readonly key: string
  private readonly storage: () => StoragePort
  constructor(
    user: { id: string },
    storage: () => StoragePort = () => window.localStorage,
  ) {
    this.storage = storage
    this.key = `ora-v2-settings-demo:v2:${encodeURIComponent(user.id)}:${DEMO_SPACE}`
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
      const initial = seed()
      this.write(initial)
      return initial
    }
    try {
      const value: unknown = JSON.parse(raw)
      if (isSnapshot(value)) return value
      if (isLegacySnapshot(value)) {
        const migrated = migrate(value)
        this.write(migrated)
        return migrated
      }
      throw new Error('invalid snapshot')
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
    const initial = seed()
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

/** One cache key shared by Settings and every catalog that reads the same local snapshot. */
export function settingsDemoQueryKey(repository: Pick<DemoRepository, 'key'>) {
  return ['settings-demo', repository.key] as const
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
    value.schema !== 4 ||
    !commonSnapshotFields(value) ||
    !Array.isArray(value.roles) ||
    !value.roles.every(isRole)
  )
    return false
  return true
}

interface LegacySnapshot {
  schema: 2 | 3
  revision: number
  connections: ModelConnection[]
  modelDefault: ModelDefault
  skills: Skill[]
}

function isLegacySnapshot(value: unknown): value is LegacySnapshot {
  if (
    !record(value) ||
    (value.schema !== 2 && value.schema !== 3) ||
    !commonSnapshotFields(value)
  )
    return false
  return true
}

function migrate(value: LegacySnapshot): Snapshot {
  return {
    schema: 4,
    revision: value.revision,
    connections: value.connections,
    modelDefault: value.modelDefault,
    skills:
      value.schema === 2
        ? value.skills.filter((skill) => skill.source.kind === 'local')
        : value.skills,
    roles: seedRoles(),
  }
}

/** Shape shared by current snapshots and the legacy snapshots that can be upgraded. */
function commonSnapshotFields(value: Record<string, unknown>): boolean {
  return (
    Number.isSafeInteger(value.revision) &&
    rows(value.skills, [
      'id',
      'name',
      'namespace',
      'description',
      'content',
      'availability',
    ]) &&
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

function isRole(value: unknown): value is Role {
  return (
    record(value) &&
    value.namespace === 'local' &&
    ['id', 'name', 'description', 'content', 'updatedAt'].every(
      (field) => typeof value[field] === 'string',
    )
  )
}
