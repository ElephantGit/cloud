import {
  getApiV1TenantsTidSpaces,
  getApiV1TenantsTidSpacesSpaceIdPlugins,
} from '@/api/spaces/spaces'
import type { InstalledPlugin, Skill } from '../contracts'

/** Definitions are tenant-wide; execution rechecks the chosen project's space and receipt. */
export async function workflowPluginCatalog(tenantId: string) {
  const spaces = await getApiV1TenantsTidSpaces(tenantId)
  const lists = await Promise.all(spaces.items.map((space) =>
    getApiV1TenantsTidSpacesSpaceIdPlugins(tenantId, space.id)))
  const installed = lists.flatMap((list) => list.items).filter((item) =>
    item.desiredState === 'installed' && item.observedState === 'installed')
  const unique = new Map(installed.map((item) => [`${item.sourceNamespace}/${item.identifier}`, item]))
  const skills: Skill[] = []
  const plugins: InstalledPlugin[] = []
  for (const [id, item] of unique) {
    if (item.kind === 'skill') {
      skills.push({ id, namespace: item.sourceNamespace, name: id,
        description: item.identifier, source: { kind: 'plugin', pluginId: id }, availability: 'available' })
    }
    if (item.kind === 'mcp') {
      plugins.push({ id, namespace: item.sourceNamespace, name: item.identifier,
        displayName: item.identifier, version: item.observedVersion ?? item.desiredVersion,
        description: '', homepage: null, license: null, logo: null, kind: 'mcp',
        installationValidity: { validity: 'valid' },
        configuration: { state: 'unavailable', errorCode: 'mcp_node_unsupported' }, runtime: 'stopped' })
    }
  }
  return { skills, plugins }
}
