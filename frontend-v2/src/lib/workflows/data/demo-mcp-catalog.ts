import { getApiV1TenantsTidSpaces, getApiV1TenantsTidSpacesSpaceIdPluginsCatalog } from '@/api/spaces/spaces'
import { withDemoCatalog } from '@/features/settings/plugins/demo'
import type { InstalledPlugin } from '../contracts'

/** Reuses Settings' user/tenant/space selections for authoring only; no MCP is started. */
export async function workflowDemoMcps(tenantId: string, userId: string, selections: Record<string, string[]>) {
  const spaces = await getApiV1TenantsTidSpaces(tenantId)
  const catalogs = await Promise.all(spaces.items.map(async (space) => {
    const selected = selections[`${userId}:${tenantId}:${space.id}`] ?? []
    if (!selected.length) return []
    const catalog = await getApiV1TenantsTidSpacesSpaceIdPluginsCatalog(tenantId, space.id)
    return withDemoCatalog(catalog.items).filter((entry) => entry.kind === 'mcp' &&
      selected.includes(`${entry.sourceNamespace}/${entry.identifier}`))
  }))
  const entries = new Map(catalogs.flat().map((entry) => [`${entry.sourceNamespace}/${entry.identifier}`, entry]))
  return Array.from(entries, ([id, entry]): InstalledPlugin => ({
    id, namespace: entry.sourceNamespace, name: entry.identifier,
    displayName: `${entry.title}（演示）`, version: entry.version,
    description: entry.description, homepage: entry.homepage, license: entry.license,
    logo: null, kind: 'mcp', installationValidity: { validity: 'valid' },
    configuration: { state: 'not_declared' }, runtime: 'stopped',
  }))
}
