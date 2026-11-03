import { describe, expect, it } from 'vitest'
import { demoMocks } from './registry'

// Every module in this directory except the switch, the register itself and tests.
const infrastructure = new Set(['flags.ts', 'registry.ts'])
const modules = Object.keys(import.meta.glob('./*.{ts,tsx}'))
  .map((path) => path.slice(2))
  .filter((file) => !infrastructure.has(file) && !/\.test\.tsx?$/.test(file))

describe('demo mock register', () => {
  it('lists every mock module exactly once', () => {
    expect(demoMocks.map((mock) => mock.module).sort()).toEqual([...modules].sort())
  })

  it('names the backend that replaces each mock', () => {
    for (const mock of demoMocks) {
      expect(mock.covers.trim(), mock.module).not.toBe('')
      expect(mock.replacedBy.trim(), mock.module).not.toBe('')
    }
  })
})
