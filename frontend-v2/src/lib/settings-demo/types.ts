import type {
  ModelConnection,
  ModelDefault,
} from '@/api/generated.schemas'

export const DEMO_SPACE = 'settings-demo-space'
/** The signed-in account as the demo pages need it; only the real role decides administration. */
export interface DemoUser {
  id: string
  displayName: string
  role: 'owner' | 'member'
}
/**
 * A configurable role, mirroring desktop's local Agent definition. The name is what workflow
 * graphs store, so changing it deliberately leaves existing graph references untouched.
 */
export interface Role {
  id: string
  namespace: 'local'
  name: string
  description: string
  content: string
  updatedAt: string
}
/**
 * Browser-local demo data. It holds no people: the roster is real (`GET /tenants/{tid}/members`),
 * and the only role that reaches these pages is the signed-in one.
 */
export interface Snapshot {
  schema: 5
  revision: number
  connections: ModelConnection[]
  modelDefault: ModelDefault
  roles: Role[]
}
export type ConnectionDraft = Pick<
  ModelConnection,
  'name' | 'protocol' | 'authMode' | 'baseUrl' | 'enabled' | 'models'
>
export type RoleDraft = Pick<Role, 'name' | 'description' | 'content'>
