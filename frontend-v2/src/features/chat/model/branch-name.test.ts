import { describe, expect, it } from 'vitest'
import { branchNameFor } from './branch-name'

describe('branchNameFor', () => {
  it('names the branch after the first words of the prompt', () => {
    expect(branchNameFor('Fix the login redirect after OAuth callback', [])).toBe('ora/fix-login-redirect-after')
    expect(branchNameFor('Please add retry to the SSE stream', [])).toBe('ora/add-retry-sse-stream')
    expect(branchNameFor('修一下 v2 的 SSE 重连', [])).toBe('ora/v2-sse')
  })

  it('falls back to ora/<adjective>-<noun> when the prompt has no Latin words', () => {
    expect(branchNameFor('把对话页的标题改一下', [], () => 0)).toBe('ora/amber-brook')
    expect(branchNameFor('看看这里', [])).toMatch(/^ora\/[a-z]+-[a-z]+$/)
  })

  it('never returns a taken name, numbering it instead', () => {
    expect(branchNameFor('add tests', ['ora/add-tests'])).toBe('ora/add-tests-2')
    expect(branchNameFor('add tests', ['ora/add-tests', 'ora/add-tests-2'])).toBe('ora/add-tests-3')
  })
})
