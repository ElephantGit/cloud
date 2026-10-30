import { useState } from 'react'
import { Bot, Pencil, UserRound } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { SquadDetail } from './api/generated'
import { activeMembers, endedMembers, formatTime, leaderName } from './squad-model'

/**
 * Read-only view of one squad with its maintainer actions. Membership changes belong to the
 * member operations; this view only edits the squad's own text and archives it.
 */
export function SquadDetailView({ squad, canMaintain, archiving, error, onEdit, onArchive }: {
  squad: SquadDetail
  canMaintain: boolean
  archiving: boolean
  /** Error from the last archive attempt. */
  error: string
  onEdit: () => void
  onArchive: () => void
}) {
  const [confirming, setConfirming] = useState(false)
  const live = activeMembers(squad)
  const ended = endedMembers(squad)
  const leader = leaderName(squad)
  const archived = squad.archivedAt !== null
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
              {member.spaceAgentId !== null && member.spaceAgentId === squad.leaderAgentId && <Badge variant="outline">leader</Badge>}
            </span>
            <span className="mc-member-role">{member.role || '未填写职责'}</span>
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
        : confirming
          ? <>
            <span className="mc-confirm">归档后不能恢复，全部有效编组会结束。</span>
            <Button type="button" variant="outline" size="sm" onClick={() => setConfirming(false)} disabled={archiving}>取消</Button>
            <Button type="button" variant="destructive" size="sm" onClick={onArchive} disabled={archiving}>{archiving ? '归档中…' : '确认归档'}</Button>
          </>
          : <>
            {!canMaintain && <span className="mc-muted">只有本队人类维护者或工作区管理员可以修改。</span>}
            <Button type="button" variant="ghost" size="sm" onClick={() => setConfirming(true)} disabled={!canMaintain}>归档</Button>
            <Button type="button" variant="outline" size="sm" onClick={onEdit} disabled={!canMaintain}><Pencil size={14} />编辑</Button>
          </>}
    </footer>
  </div>
}
