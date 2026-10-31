import { Client } from 'pg'

// The development login always signs in as the same demo account, so a workspace can never hold
// a second human through the UI alone. E2E adds that precondition straight to the database the
// v2-server uses, the same way the Go tests seed members.
const database = process.env.E2E_DATABASE_URL ?? 'postgres://ora:ora-local@127.0.0.1:55432/ora_v2_e2e?sslmode=disable'

/**
 * Creates (or renames) a workspace member outside the development login and joins them to the
 * demo workspace. Call it after the first browser login, which is what creates that workspace.
 */
export async function seedWorkspaceMember(displayName: string): Promise<void> {
  const client = new Client({ connectionString: database })
  await client.connect()
  try {
    // Withdraw older runs' seeded members so they do not pile up in the picker.
    await client.query(
      `DELETE FROM memberships m USING users u WHERE m.user_id = u.id AND u.source = 'e2e'`)
    await client.query(
      `INSERT INTO users (source, subject, display_name) VALUES ('e2e', $1, $1)
       ON CONFLICT (source, subject) DO UPDATE SET display_name = EXCLUDED.display_name`, [displayName])
    await client.query(
      `INSERT INTO memberships (tenant_id, user_id, role)
       SELECT t.id, u.id, 'member' FROM users u, tenants t
       WHERE u.source = 'e2e' AND u.subject = $1 AND t.name = '演示团队'
       ON CONFLICT DO NOTHING`, [displayName])
  } finally {
    await client.end()
  }
}

/** Ends one e2e member's workspace membership, leaving other runs' data alone. */
export async function dropWorkspaceMember(displayName: string): Promise<void> {
  const client = new Client({ connectionString: database })
  await client.connect()
  try {
    await client.query(
      `DELETE FROM memberships WHERE user_id IN (SELECT id FROM users WHERE source = 'e2e' AND subject = $1)`, [displayName])
  } finally {
    await client.end()
  }
}
