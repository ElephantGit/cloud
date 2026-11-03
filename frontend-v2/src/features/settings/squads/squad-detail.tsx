import { useState } from 'react'
import { Bot, Crown, Pencil, ShieldCheck, UserPlus, UserRound, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { SquadDetail, SquadMember } from '@/lib/squads-api/generated'
import { activeMembers, endedMembers, formatTime, isLeader, leaderName, ownMembership } from './squad-model'

/**
 * Read-only view of one squad with its maintainer actions: edit the squad's own text, add an
 * existing workspace member, toggle a human's maintainer mark, set or clear the leader (an in-squad
 * active Agent), remove a live member, or archive. A member always sees an exit action on their own
 * record; the server skips the maintainer check for self-exit. Removing ends only the squad
 * membership; the workspace membership is untouched.
 */
export function SquadDetailView({ squad, userId, canMaintain, archiving, removing, updating, error, onEdit, onArchive, onAddMembers, onRemove, onToggleMaintainer, onSetLeader }: {
  squad: SquadDetail
  userId: string
  canMaintain: boolean
  archiving: boolean
  /** Membership id currently being removed or exited, or '' when none. */
  removing: string
  /** Membership id whose maintainer or leader mark is being written, or '' when none. */
  updating: string
  /** Error from the last archive or membership attempt. */
  error: string
  onEdit: () => void
  onArchive: () => void
  onAddMembers: () => void
  onRemove: (member: SquadMember) => void
  onToggleMaintainer: (member: SquadMember) => void
  onSetLeader: (member: SquadMember) => void
}) {
  const [confirmingArchive, setConfirmingArchive] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState('')
  const live = activeMembers(squad)
  const ended = endedMembers(squad)
  const leader = leaderName(squad)
  const archived = squad.archivedAt !== null
  const own = ownMembership(squad, userId)

  const removeActions = (member: SquadMember) => {
    const isOwn = member.id === own?.id
    if (confirmingRemove === member.id) {
      return <>
        <Button type="button" variant="ghost" size="sm" onClick={() => setConfirmingRemove('')} disabled={removing === member.id}><X size={14} />取消</Button>
        <Button type="button" variant="destructive" size="sm" onClick={() => { setConfirmingRemove(''); onRemove(member) }} disabled={removing === member.id}>{removing === member.id ? '处理中…' : isOwn ? '确认退出' : '确认移除'}</Button>
      </>
    }
    return <Button type="button" variant="ghost" size="sm" aria-label={isOwn ? `退出${member.displayName}` : `移除${member.displayName}`} onClick={() => setConfirmingRemove(member.id)}>{isOwn ? '退出小队' : <><X size={14} />移除</>}</Button>
  }

  return <div className="mc-detail">
    <div className="mc-detail-scroll">
      {archived && <p className="mc-archived">这个小队已归档，成员编组已结束，只保留历史。</p>}
      <p className="mc-description">{squad.description || '还没有简介。'}</p>

      <section className="mc-section" aria-label="协作说明">
        <h3>协作说明</h3>
        <p>{squad.charter || '还没有协作说明。'}</p>
      </section>

      <div className="mc-facts">
        <span><i className="mc-dot human" />人类 <b>{squad.humanCount}</b></span>
        <span><i className="mc-dot agent" />Agent <b>{squad.agentCount}</b></span>
        <span><i className="mc-dot maintainer" />维护者 <b>{squad.maintainerCount}</b></span>
        <span>leader：{leader ?? (squad.leaderAgentId ? 'Agent 已不在本队' : '未设置')}</span>
      </div>

      <section className="mc-section" aria-label="成员">
        <h3>成员 <Badge variant="secondary">{live.length}</Badge></h3>
        {live.length === 0
          ? <p className="mc-empty-line">没有有效成员。</p>
          : <ul className="mc-members">{live.map(member => <li key={member.id}>
            <span className={`mc-member-icon ${member.type}`} aria-hidden="true">{member.type === 'human' ? <UserRound size={15} /> : <Bot size={15} />}</span>
            <span className="mc-member-name">
              {member.displayName}
              <em>{member.type === 'human' ? '人类' : 'Agent'}</em>
              {member.maintainer && <Badge variant="outline">维护者</Badge>}
              {isLeader(squad, member) && <Badge variant="outline">leader</Badge>}
            </span>
            <span className="mc-member-role">{member.role || '未填写职责'}</span>
            {!archived && canMaintain && member.type === 'human' && (
              <Button type="button" variant="ghost" size="sm" disabled={updating === member.id} onClick={() => onToggleMaintainer(member)}>{member.maintainer ? <><ShieldCheck size={14} />取消维护者</> : '设为维护者'}</Button>
            )}
            {!archived && canMaintain && member.type === 'agent' && (
              <Button type="button" variant="ghost" size="sm" disabled={updating === member.id} onClick={() => onSetLeader(member)}>{isLeader(squad, member) ? '取消 leader' : <><Crown size={14} />设为 leader</>}</Button>
            )}
            {!archived && (member.id === own?.id || canMaintain) && removeActions(member)}
          </li>)}</ul>}
      </section>

      {ended.length > 0 && <details className="mc-history">
        <summary>已退出 {ended.length}</summary>
        <ul>{ended.map(member => <li key={member.id}>
          <span>{member.displayName}<em>{member.type === 'human' ? '人类' : 'Agent'}</em></span>
          <small>{formatTime(member.joinedAt)} 至 {formatTime(member.endedAt!)}</small>
        </li>)}</ul>
      </details>}

      <p className="mc-note">小队成员关系不授予私人模型、Git 凭据或运行时权限，也不触发 Agent 执行或自动委派。</p>
      {error && <p role="alert" className="mc-error">{error}</p>}
    </div>

    <footer className="mc-detail-footer">
      {archived
        ? <span className="mc-muted">已归档 · 只读</span>
        : confirmingArchive
          ? <>
            <span className="mc-confirm">归档后不能恢复，全部有效编组会结束。</span>
            <Button type="button" variant="outline" size="sm" onClick={() => setConfirmingArchive(false)} disabled={archiving}>取消</Button>
            <Button type="button" variant="destructive" size="sm" onClick={onArchive} disabled={archiving}>{archiving ? '归档中…' : '确认归档'}</Button>
          </>
          : <>
            {!canMaintain && <span className="mc-muted">只有本队人类维护者或工作区管理员可以修改。</span>}
            <Button type="button" variant="ghost" size="sm" onClick={() => setConfirmingArchive(true)} disabled={!canMaintain}>归档</Button>
            <Button type="button" variant="ghost" size="sm" onClick={onAddMembers} disabled={!canMaintain}><UserPlus size={14} />添加成员</Button>
            <Button type="button" variant="outline" size="sm" onClick={onEdit} disabled={!canMaintain}><Pencil size={14} />编辑</Button>
          </>}
    </footer>
  </div>
}