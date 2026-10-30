import { requireAdmin } from './resource-actions'
import {
  DEMO_TENANT,
  type DemoLink,
  type DemoUser,
  type Snapshot,
} from './types'

export function updateMember(
  data: Snapshot,
  actor: DemoUser,
  userId: string,
  change: 'admin' | 'member' | 'disabled',
) {
  requireAdmin(data, actor.id)
  const member = data.members.find((m) => m.userId === userId)
  if (!member || member.status !== 'active')
    throw new Error('只能修改有效成员；停用成员需要重新申请加入。')
  if (
    member.role === 'admin' &&
    change !== 'admin' &&
    !data.members.some(
      (m) => m.userId !== userId && m.role === 'admin' && m.status === 'active',
    )
  )
    throw new Error('不能停用或降级最后一名有效管理员。')
  if (change === 'disabled') member.status = 'disabled'
  else member.role = change
  member.version++
}
export const linkState = (link: DemoLink, now = Date.now()) =>
  link.revokedAt
    ? '已撤销'
    : 'consumedAt' in link && link.consumedAt
      ? '已使用'
      : Date.parse(link.expiresAt) <= now
        ? '已过期'
        : '有效'

export function createLink(
  data: Snapshot,
  actor: DemoUser,
  kind: DemoLink['kind'],
  note: string,
) {
  requireAdmin(data, actor.id)
  if (note.length > 200) throw new Error('备注最多 200 字。')
  const id = crypto.randomUUID()
  data.links.unshift({
    id,
    kind,
    note: note.trim(),
    tenantId: DEMO_TENANT,
    createdBy: actor.id,
    createdByName: actor.displayName,
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 7 * 86400000).toISOString(),
    version: 1,
  })
  return id
}
export function revokeLink(data: Snapshot, actor: DemoUser, id: string) {
  requireAdmin(data, actor.id)
  const link = data.links.find((l) => l.id === id)
  if (!link || linkState(link) !== '有效')
    throw new Error('此链接已失效，请刷新列表。')
  link.revokedAt = new Date().toISOString()
  link.version++
}
export function decideRequest(
  data: Snapshot,
  actor: DemoUser,
  id: string,
  approve: boolean,
) {
  requireAdmin(data, actor.id)
  const request = data.requests.find((r) => r.id === id)
  if (!request || request.status !== 'pending')
    throw new Error('该申请已处理，请刷新列表。')
  const now = new Date().toISOString()
  Object.assign(request, {
    status: approve ? 'approved' : 'rejected',
    decidedAt: now,
    decidedBy: actor.id,
    decidedByName: actor.displayName,
    version: request.version + 1,
  })
  if (!approve) return
  const existing = data.members.find((m) => m.userId === request.userId)
  if (existing?.status === 'active') throw new Error('该用户已经是有效成员。')
  const fields = {
    userId: request.userId,
    displayName: request.displayName ?? '演示申请者',
    tenantId: DEMO_TENANT,
    role: 'member',
    status: 'active',
    joinMethod: 'application' as const,
    joinedAt: now,
    actorDisplayName: actor.displayName,
  }
  if (existing)
    Object.assign(existing, fields, { version: existing.version + 1 })
  else data.members.push({ id: crypto.randomUUID(), ...fields, version: 1 })
  data.history.unshift({
    id: crypto.randomUUID(),
    tenantId: DEMO_TENANT,
    userId: request.userId,
    method: 'application',
    joinedAt: now,
    actorUserId: actor.id,
    actorDisplayName: actor.displayName,
    requestId: request.id,
    linkNote: request.linkNote,
  })
}
