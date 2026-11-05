import type { PluginCatalogEntry } from '@/api/generated.schemas'

const DEMO_INDEXED_AT = '2026-11-02T00:00:00Z'
const DEMO_SOURCE_URL = '本地'

function demoEntry(
  identifier: string,
  kind: 'mcp' | 'hook',
  title: string,
  description: string,
): PluginCatalogEntry {
  return {
    id: `demo/${identifier}`,
    sourceNamespace: 'demo',
    identifier,
    title,
    kind,
    version: '1.0.0',
    description,
    homepage: null,
    license: 'MIT',
    logo: null,
    url: null,
    sha256: null,
    targets: null,
    packMembers: null,
    readme: `# ${title}\n\n${description}`,
    marketplaceVisible: true,
    sourceUrl: DEMO_SOURCE_URL,
    indexedAt: DEMO_INDEXED_AT,
  }
}

export const demoMcpEntry = demoEntry(
  'sample-mcp',
  'mcp',
  '示例 MCP 工具',
  'MCP 插件的安装与卸载不会连接外部服务。',
)

const serverWorkflowKeys = new Set([
  'official/ora-space.requirement-development',
  'official/ora-space.requirement-decomposition',
  'official/ora-space.customer-feedback-triage',
  'official/ora-space.daily-inspection',
])

type PluginIdentity = Pick<PluginCatalogEntry, 'identifier' | 'sourceNamespace'> & {
  kind?: string | null
}

export function localWorkflow(entry: PluginIdentity) {
  return entry.kind === 'workflow' &&
    !serverWorkflowKeys.has(`${entry.sourceNamespace}/${entry.identifier}`)
}

/** Browser-local demo entries are limited to MCP; Hook demo state lives in the Space plugin API. */
export function demoPlugin(entry: PluginIdentity) {
  return localWorkflow(entry) || entry.kind === 'mcp'
}

/**
 * v2-server has no MCP executor yet. Keep one clearly labelled local example visible so the plugin
 * page can still demonstrate that lifecycle when the synced marketplace has no MCP entry. Hook
 * demo entries come from the server so their install state can be stored in PostgreSQL.
 */
export function withDemoCatalog(catalog: PluginCatalogEntry[]) {
  return [
    ...catalog,
    ...(catalog.some((entry) => entry.kind === 'mcp') ? [] : [demoMcpEntry]),
  ]
}
