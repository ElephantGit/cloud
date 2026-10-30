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
 status text NOT NULL DEFAULT 'recorded',
 recorded_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(schedule_id,kind,trigger_key)
);

ALTER TABLE v2_schedule_occurrences ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '';
ALTER TABLE v2_schedule_occurrences ADD COLUMN IF NOT EXISTS execution_ref text NOT NULL DEFAULT '';
ALTER TABLE v2_schedule_occurrences ADD COLUMN IF NOT EXISTS released boolean NOT NULL DEFAULT true;
ALTER TABLE v2_schedule_occurrences ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1;
ALTER TABLE v2_schedule_occurrences DROP CONSTRAINT IF EXISTS v2_schedule_occurrences_status_check;
-- Upgrade earlier draft records without dropping their identity or task occupancy.
UPDATE v2_schedule_occurrences SET status='reconciling',reason='lookup_required',version=version+1 WHERE status='admission_unknown';
UPDATE v2_schedule_occurrences SET reason='execution_busy' WHERE reason='workspace_busy';
UPDATE v2_schedule_occurrences SET reason='execution_not_started' WHERE reason='admission_not_accepted';
ALTER TABLE v2_schedule_occurrences ADD CONSTRAINT v2_schedule_occurrences_status_check CHECK(status IN ('recorded','starting','reconciling','running','cancelling','succeeded','failed','cancelled','interrupted','skipped'));
CREATE UNIQUE INDEX IF NOT EXISTS v2_schedule_active_occurrence ON v2_schedule_occurrences(schedule_id) WHERE NOT released;

CREATE TABLE IF NOT EXISTS sessions (
	token_hash bytea PRIMARY KEY,
	user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	expires_at timestamptz NOT NULL
);

-- Squads group people and Space Agents inside one workspace. Each tenant has exactly one
-- collaboration space in v2, so tenant_id is the space boundary.
CREATE TABLE IF NOT EXISTS v2_squads (
	id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	tenant_id       uuid NOT NULL REFERENCES tenants (id),
	name            text NOT NULL,
	description     text NOT NULL DEFAULT '',
	charter         text NOT NULL DEFAULT '',
	leader_agent_id uuid,
	archived_at     timestamptz,
	version         bigint NOT NULL DEFAULT 1,
	created_by      uuid NOT NULL REFERENCES users (id),
	created_at      timestamptz NOT NULL DEFAULT now(),
	updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS v2_squads_tenant ON v2_squads (tenant_id, archived_at);

-- The v2 stand-in for the shared Space Agent directory: agent members reference an active row
-- here, and no HTTP route owns this table yet. It is not a second directory API.
CREATE TABLE IF NOT EXISTS v2_space_agents (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	tenant_id  uuid NOT NULL REFERENCES tenants (id),
	name       text NOT NULL,
	status     text NOT NULL DEFAULT 'active',
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS v2_space_agents_tenant ON v2_space_agents (tenant_id);

-- Members are typed references: a human row points at a user, an agent row at a Space Agent.
-- Ending a membership keeps the row as history; rejoining inserts a new grant.
CREATE TABLE IF NOT EXISTS v2_squad_members (
	id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	squad_id       uuid NOT NULL REFERENCES v2_squads (id) ON DELETE CASCADE,
	member_type    text NOT NULL CHECK (member_type IN ('human', 'agent')),
	user_id        uuid REFERENCES users (id),
	space_agent_id uuid REFERENCES v2_space_agents (id),
	role           text NOT NULL DEFAULT '',
	maintainer     boolean NOT NULL DEFAULT false,
	joined_at      timestamptz NOT NULL DEFAULT now(),
	ended_at       timestamptz,
	CHECK ((member_type = 'human') = (user_id IS NOT NULL)),
	CHECK ((member_type = 'agent') = (space_agent_id IS NOT NULL)),
	CHECK (member_type = 'human' OR NOT maintainer)
);
-- At most one live grant per typed identity; ended grants stay as history.
CREATE UNIQUE INDEX IF NOT EXISTS v2_squad_members_active
	ON v2_squad_members (squad_id, member_type, COALESCE(user_id, space_agent_id))
	WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS v2_squad_members_squad ON v2_squad_members (squad_id);
