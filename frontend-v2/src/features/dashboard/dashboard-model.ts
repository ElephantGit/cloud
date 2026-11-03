import type { DailyDone, TeamMemberProgress } from '@/lib/demo/dashboard'

/**
 * Share of a member's week already waiting for acceptance or finished, 0–1; a quiet week is 0.
 * The denominator is every task the member holds, so a row's bar matches its own numbers.
 */
export const completionRate = (member: TeamMemberProgress): number => {
  const cleared = member.review + member.doneThisWeek
  const total = member.running + cleared
  return total === 0 ? 0 : cleared / total
}

/** A whole percentage for display, rounded half up. */
export const percent = (rate: number): string => `${Math.round(rate * 100)}%`

/** Human and agent headcounts and live work, for the header line above the member rows. */
export const teamTotals = (members: readonly TeamMemberProgress[]): { humans: number; agents: number; running: number } => ({
  humans: members.filter(member => member.kind === 'human').length,
  agents: members.filter(member => member.kind === 'agent').length,
  running: members.reduce((total, member) => total + member.running, 0),
})

/** The largest daily completion count, at least 1 so bar heights never divide by zero. */
export const trendMax = (trend: readonly DailyDone[]): number => Math.max(1, ...trend.map(day => day.done))

/** How tall a day's bar is relative to the busiest day, 0–1. */
export const barHeight = (day: DailyDone, max: number): number => day.done / max
