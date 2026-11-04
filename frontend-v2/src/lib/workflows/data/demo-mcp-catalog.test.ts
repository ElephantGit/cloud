import { afterEach, expect, it } from 'vitest'
import { http } from '@/lib/api-client'
import { fakeBackend } from '@/test/backend'
import { demoMcpEntry, demoHookEntry } from '@/features/settings/plugins/demo'
import { workflowDemoMcps } from './demo-mcp-catalog'

const original = http.defaults.adapter
afterEach(() => { http.defaults.adapter = original })

it('uses existing Settings selections across authorized spaces, deduplicates and excludes other scopes/kinds', async () => {
  const seen = fakeBackend((config) => config.url === '/api/v1/tenants/t/spaces'
    ? { status: 200, data: { items: [{ id: 'a' }, { id: 'b' }] } }
    : { status: 200, data: { items: [demoMcpEntry, demoHookEntry] } })
  const plugins = await workflowDemoMcps('t', 'u', {
    'u:t:a': ['demo/sample-mcp', 'demo/sample-hooks', 'missing'],
    'u:t:b': ['demo/sample-mcp'],
    'other:t:a': ['private/mcp'], 'u:other:a': ['private/mcp'], 'u:t:inaccessible': ['private/mcp'],
  })
  expect(plugins.map(({ id, displayName, runtime }) => ({ id, displayName, runtime }))).toEqual([
    { id: 'demo/sample-mcp', displayName: '示例 MCP 工具（演示）', runtime: 'stopped' },
  ])
  expect(seen).not.toContain('GET /api/v1/tenants/t/spaces/inaccessible/plugins/catalog')
  expect(await workflowDemoMcps('t', 'someone-else', { 'u:t:a': ['demo/sample-mcp'] })).toEqual([])
})

it('reads real marketplace MCP metadata and removes uninstalled choices', async () => {
  fakeBackend((config) => config.url === '/api/v1/tenants/t/spaces'
    ? { status: 200, data: { items: [{ id: 'a' }] } }
    : { status: 200, data: { items: [{ ...demoMcpEntry, sourceNamespace: 'official', identifier: 'tavily', title: 'Tavily' }] } })
  expect((await workflowDemoMcps('t', 'u', { 'u:t:a': ['official/tavily'] }))[0]).toMatchObject({
    id: 'official/tavily', displayName: 'Tavily（演示）',
  })
  expect(await workflowDemoMcps('t', 'u', { 'u:t:a': [] })).toEqual([])
})

it('does not invent installed MCPs from empty selections or a failed catalog', async () => {
  fakeBackend((config) => config.url === '/api/v1/tenants/t/spaces'
    ? { status: 200, data: { items: [{ id: 'a' }] } }
    : { status: 503 })
  expect(await workflowDemoMcps('t', 'u', {})).toEqual([])
  await expect(workflowDemoMcps('t', 'u', { 'u:t:a': ['demo/sample-mcp'] })).rejects.toThrow()
})
