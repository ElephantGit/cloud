import { afterEach, expect, it } from 'vitest'
import { http } from '@/lib/api-client'
import { fakeBackend } from '@/test/backend'
import { workflowPluginCatalog } from './plugin-catalog'

const original = http.defaults.adapter
afterEach(() => { http.defaults.adapter = original })

it('uses confirmed server installations and keeps imported package identities', async () => {
  fakeBackend((config) => config.url === '/api/v1/tenants/t/spaces'
    ? { status: 200, data: { items: [{ id: 'space' }] } }
    : { status: 200, data: { items: [
      { sourceNamespace: 'official', identifier: 'skills', kind: 'skill', desiredState: 'installed', observedState: 'installed' },
      { sourceNamespace: 'custom', identifier: 'pending', kind: 'skill', desiredState: 'installed', observedState: 'pending' },
      { sourceNamespace: 'official', identifier: 'removed', kind: 'mcp', desiredState: 'removed', observedState: 'installed' },
    ] } })
  const result = await workflowPluginCatalog('t')
  expect(result.skills.map((skill) => skill.name)).toEqual(['official/skills'])
  expect(result.plugins).toEqual([])
})

it('does not substitute demo plugins on a catalog failure', async () => {
  fakeBackend(() => ({ status: 503 }))
  await expect(workflowPluginCatalog('t')).rejects.toThrow()
})
