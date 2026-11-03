import { describe, expect, it } from 'vitest'
import type { DailyDone, TeamMemberProgress } from '@/lib/demo/dashboard'
import { barHeight, completionRate, percent, teamTotals, trendMax } from './dashboard-model'

const member = (overrides: Partial<TeamMemberProgress>): TeamMemberProgress => ({
  id: 'member', name: '成员', kind: 'human', role: '研发', running: 0, review: 0, doneThisWeek: 0,
  ...overrides,
})

const day = (value: number): DailyDone => ({ day: '11-02', done: value })

describe('completion rate', () => {
  it('counts work waiting for acceptance as cleared but not work in progress', () => {
    expect(completionRate(member({ running: 1, review: 2, doneThisWeek: 1 }))).toBeCloseTo(0.75)
    expect(completionRate(member({ running: 3 }))).toBe(0)
    expect(completionRate(member({ doneThisWeek: 4 }))).toBe(1)
  })

  it('returns zero for a member with nothing assigned instead of dividing by zero', () => {
    expect(completionRate(member({}))).toBe(0)
  })
})

describe('display helpers', () => {
  it('rounds a rate to a whole percentage', () => {
    expect(percent(0)).toBe('0%')
    expect(percent(2 / 3)).toBe('67%')
    expect(percent(1)).toBe('100%')
  })

  it('counts people, agents and live work separately', () => {
    const members = [
      member({ id: 'a', kind: 'human', running: 2 }),
      member({ id: 'b', kind: 'human', running: 0 }),
      member({ id: 'c', kind: 'agent', running: 3 }),
    ]
    expect(teamTotals(members)).toEqual({ humans: 2, agents: 1, running: 5 })
  })
})

describe('trend bars', () => {
  it('scales bars against the busiest day', () => {
    expect(trendMax([day(2), day(5), day(0)])).toBe(5)
    expect(barHeight(day(5), 5)).toBe(1)
    expect(barHeight(day(0), 5)).toBe(0)
  })

  it('never divides by zero, even for a week with no completions', () => {
    expect(trendMax([])).toBe(1)
    expect(trendMax([day(0)])).toBe(1)
    expect(barHeight(day(0), trendMax([day(0)]))).toBe(0)
  })
})
