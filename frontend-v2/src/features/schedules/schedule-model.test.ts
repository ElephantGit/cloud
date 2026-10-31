import { describe, expect, it } from 'vitest'
import { emptyDraft, nextTimes, targetFromDraft, validateDraft } from './schedule-model'

describe('schedule rules', () => {
  it('uses task timezone, skips an elapsed time, and rolls the year over', () => {
    const rule = emptyDraft().rule
    expect(nextTimes(rule, new Date('2026-12-31T01:00:00Z')).map(d => d.toISOString())).toEqual(['2027-01-01T01:00:00.000Z', '2027-01-02T01:00:00.000Z', '2027-01-03T01:00:00.000Z'])
  })
  it('skips weekends and supports several selected weekdays', () => {
    const rule = { ...emptyDraft().rule, kind: 'weekdays' as const }
    expect(nextTimes(rule, new Date('2026-10-30T02:00:00Z'))[0]?.toISOString()).toBe('2026-11-02T01:00:00.000Z')
    expect(nextTimes({ ...rule, kind: 'weekly', days: [1, 5] }, new Date('2026-10-30T02:00:00Z')).map(d => d.toISOString())).toEqual(['2026-11-02T01:00:00.000Z', '2026-11-06T01:00:00.000Z', '2026-11-09T01:00:00.000Z'])
  })
  it('skips a nonexistent DST wall time instead of displaying a shifted hour', () => {
    const rule = { ...emptyDraft().rule, time: '02:30', timezone: 'America/New_York' }
    expect(nextTimes(rule, new Date('2026-03-08T00:00:00Z'))[0]?.toISOString()).toBe('2026-03-09T06:30:00.000Z')
  })
  it('anchors hourly recurrence to creation, not each render', () => {
    const rule = { ...emptyDraft().rule, kind: 'hours' as const, hours: 6 }
    expect(nextTimes(rule, new Date('2026-10-29T08:30:00Z'), new Date('2026-10-29T01:00:00Z')).map(d => d.toISOString())).toEqual(['2026-10-29T13:00:00.000Z', '2026-10-29T19:00:00.000Z', '2026-10-30T01:00:00.000Z'])
  })
})
describe('execution target boundaries',()=>{
 it('builds only conversation requests and rejects legacy references without interpreting their parameters',()=>{
  const draft={...emptyDraft(),name:'简报',prompt:'汇总变化'}
  expect(validateDraft(draft)).toEqual({})
  expect(targetFromDraft(draft)).toEqual({kind:'chat',prompt:'汇总变化',agent:'',model:''})
  for(const kind of ['workflow','task'] as const){
   expect(validateDraft({...draft,kind})).toHaveProperty('target')
   expect(()=>targetFromDraft({...draft,kind})).toThrow('引用目标只读')
  }
 })
})
