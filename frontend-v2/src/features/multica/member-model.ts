import type { MemberListItem } from '@/api/generated.schemas'
import type { SquadDetail } from './api/generated'
import { activeMembers } from './squad-model'

/**
 * Workspace members who may still join a squad: active accounts with no live membership in it.
 * A member who left keeps their history but can be added again, which is why ended rows are not
 * considered. Agents are not part of the workspace roster and never appear here.
 */
export function candidatesFor(squad: SquadDetail, roster: readonly MemberListItem[]): MemberListItem[] {
  const joined = new Set(activeMembers(squad).flatMap(member => (member.userId === null ? [] : [member.userId])))
  return roster.filter(member => member.status === 'active' && !joined.has(member.userId))
}

/** Filters candidates by display name; an empty query leaves the list unchanged. */
export function matchingMembers(roster: readonly MemberListItem[], query: string): MemberListItem[] {
  const needle = query.trim().toLowerCase()
  return needle ? roster.filter(member => member.displayName.toLowerCase().includes(needle)) : [...roster]
}
