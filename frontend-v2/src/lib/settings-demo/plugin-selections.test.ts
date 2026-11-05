import { beforeEach, expect, it } from 'vitest'
import { useDemoPluginSelections } from './plugin-selections'

beforeEach(() => {
  useDemoPluginSelections.getState().reset()
  localStorage.clear()
})

it('keeps browser-local MCP and Agent installs scoped and persisted', () => {
  const { setSelected } = useDemoPluginSelections.getState()

  setSelected('owner:tenant:space-a', 'demo/sample-mcp', true)
  setSelected('owner:tenant:space-a', 'official/ora-space.opencode', true)
  setSelected('owner:tenant:space-b', 'demo/sample-mcp', true)

  expect(useDemoPluginSelections.getState().byScope).toEqual({
    'owner:tenant:space-a': [
      'demo/sample-mcp',
      'official/ora-space.opencode',
    ],
    'owner:tenant:space-b': ['demo/sample-mcp'],
  })
  expect(localStorage.getItem('ora-v2-plugin-demo:v1')).toContain(
    'tenant:space-a',
  )

  setSelected('owner:tenant:space-a', 'demo/sample-mcp', false)

  expect(
    useDemoPluginSelections.getState().byScope['owner:tenant:space-a'],
  ).toEqual(['official/ora-space.opencode'])
  expect(
    useDemoPluginSelections.getState().byScope['owner:tenant:space-b'],
  ).toEqual(['demo/sample-mcp'])
})
