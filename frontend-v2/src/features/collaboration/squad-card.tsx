import type { SquadSummary } from './api/generated'
import { formatTime } from './squad-model'

/**
 * One squad in the list. Counts are the server's live counts, which exclude ended memberships;
 * the leader's name needs the member list, so the card only says whether a leader is set.
 */
export function SquadCard({ squad, onOpen }: { squad: SquadSummary; onOpen: () => void }) {
  return <button type="button" className="mc-card" aria-label={`查看${squad.name}`} onClick={onOpen}>
    <span className="mc-card-head">
      <strong>{squad.name}</strong>
      <span className={`mc-leader ${squad.leaderAgentId ? 'set' : ''}`}>{squad.leaderAgentId ? '已设 leader' : '未设 leader'}</span>
    </span>
    <span className="mc-card-description">{squad.description || '还没有简介。'}</span>
    <span className="mc-card-counts">
      <span><i className="mc-dot human" />人类 <b>{squad.humanCount}</b></span>
      <span><i className="mc-dot agent" />Agent <b>{squad.agentCount}</b></span>
      <span><i className="mc-dot maintainer" />维护者 <b>{squad.maintainerCount}</b></span>
    </span>
    <small className="mc-card-time">更新于 {formatTime(squad.updatedAt)}</small>
  </button>
}
