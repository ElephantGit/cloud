import { describe, expect, it } from 'vitest'
import { dashboardDemo, type DashboardStat } from './dashboard'

// The recording reads this page right after the board, so a number that does not follow from the
// rows below it is a visible mistake. Keep the dataset self-consistent by construction and let a
// drift fail here instead of on camera.
const sum = (values: readonly number[]): number => values.reduce((total, value) => total + value, 0)
const tile = (label: string): DashboardStat => {
  const stat = dashboardDemo.stats.find(candidate => candidate.label === label)
  if (!stat) throw new Error(`missing stat tile: ${label}`)
  return stat
}

const { members, outputs, trend } = dashboardDemo
const humans = members.filter(member => member.kind === 'human')
const agents = members.filter(member => member.kind === 'agent')

describe('dashboard demo data', () => {
  it('takes every overview tile from the member rows', () => {
    expect(tile('进行中').value).toBe(String(sum(members.map(member => member.running))))
    expect(tile('待验收').value).toBe(String(sum(members.map(member => member.review))))
    expect(tile('本周完成').value).toBe(String(sum(members.map(member => member.doneThisWeek))))
  })

  it('closes the week exactly as the busy-day trend shows it', () => {
    expect(sum(trend.map(day => day.done))).toBe(sum(members.map(member => member.doneThisWeek)))
  })

  it('states how many people and agents are actually busy, not how many tasks they hold', () => {
    const busyHumans = humans.filter(member => member.running > 0).length
    const busyAgents = agents.filter(member => member.running > 0).length
    expect(tile('进行中').note).toBe(`${busyHumans} 人在处理，${busyAgents} 个 Agent 在运行`)
  })

  it('counts the agent work waiting for acceptance on both the tile and the output list', () => {
    const agentReview = sum(agents.map(member => member.review))
    expect(tile('待验收').note).toBe(`其中 ${agentReview} 项由 Agent 提交`)
    // The output list is the agent column of the same pending work, agent by agent.
    for (const agent of agents) {
      expect(
        outputs.filter(output => output.status === 'review' && output.agent === agent.name),
        agent.name,
      ).toHaveLength(agent.review)
    }
  })
})
