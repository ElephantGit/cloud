-- v2-server's whole schema. The server is temporary, so it applies this idempotently at
-- startup instead of keeping migrations.
CREATE TABLE IF NOT EXISTS users (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	source       text NOT NULL,
	subject      text NOT NULL,
	display_name text NOT NULL,
	status       text NOT NULL DEFAULT 'active',
	version      bigint NOT NULL DEFAULT 1,
	created_at   timestamptz NOT NULL DEFAULT now(),
	deleted_at   timestamptz,
	UNIQUE (source, subject)
);

CREATE TABLE IF NOT EXISTS tenants (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name       text NOT NULL UNIQUE,
	status     text NOT NULL DEFAULT 'active',
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
	tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
	user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	role      text NOT NULL,
	PRIMARY KEY (tenant_id, user_id)
);

-- Only the SHA-256 of a session token is stored.
CREATE TABLE IF NOT EXISTS v2_schedule_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 user_id uuid NOT NULL REFERENCES users(id),
 name text NOT NULL,
 project_id text NOT NULL DEFAULT '',
 target jsonb NOT NULL,
 rule jsonb NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS v2_schedule_configs_owner ON v2_schedule_configs(tenant_id,user_id);

CREATE TABLE IF NOT EXISTS v2_schedule_occurrences (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 schedule_id uuid NOT NULL REFERENCES v2_schedule_configs(id),
 kind text NOT NULL CHECK(kind IN ('scheduled','manual')),
 trigger_key text NOT NULL,
 scheduled_at timestamptz NOT NULL,
 config_version bigint NOT NULL CHECK(config_version>0),
 snapshot jsonb NOT NULL,
 status text NOT NULL DEFAULT 'recorded' CHECK(status='recorded'),
 recorded_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(schedule_id,kind,trigger_key)
);

CREATE TABLE IF NOT EXISTS sessions (
	token_hash bytea PRIMARY KEY,
	user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	expires_at timestamptz NOT NULL
);
