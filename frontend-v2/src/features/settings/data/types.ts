import type {
  Invitation,
  JoinLink,
  JoinRequest,
  MemberJoinEvent,
  MemberListItem,
  ModelConnection,
  ModelDefault,
  PluginCatalogEntry,
  SpacePlugin,
} from '@/api/generated.schemas'

export const DEMO_TENANT = 'settings-demo-tenant'
export const DEMO_SPACE = 'settings-demo-space'
export interface DemoUser {
  id: string
  displayName: string
}
export type SkillSource =
  { kind: 'local' } | { kind: 'plugin'; pluginId: string }
/** A document resource, independent from the package that may supply it. */
export interface Skill {
  id: string
  namespace: string
  name: string
  description: string
  content: string
  source: SkillSource
  availability: 'available' | 'unavailable'
}
export type DemoLink = (Invitation | JoinLink) & { kind: 'invite' | 'apply' }
export interface Snapshot {
  schema: 1
  revision: number
  members: MemberListItem[]
  history: MemberJoinEvent[]
  links: DemoLink[]
  requests: JoinRequest[]
  connections: ModelConnection[]
  modelDefault: ModelDefault
  catalog: PluginCatalogEntry[]
  installations: SpacePlugin[]
  skills: Skill[]
}
export type ConnectionDraft = Pick<
  ModelConnection,
  'name' | 'protocol' | 'authMode' | 'baseUrl' | 'enabled' | 'models'
>
export type SkillDraft = Pick<Skill, 'name' | 'description' | 'content'>
/** Catalog row ids and selection row ids are unrelated; this pair identifies the package. */
export const pluginKey = (plugin: {
  sourceNamespace: string
  identifier: string
}) => `${plugin.sourceNamespace}/${plugin.identifier}`
