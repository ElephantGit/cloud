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
CREATE TABLE IF NOT EXISTS sessions (
	token_hash bytea PRIMARY KEY,
	user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	expires_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    graph jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(graph) = 'object'),
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS workflows_live_name ON workflows(tenant_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS workflow_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    workflow_id uuid NOT NULL,
    version bigint NOT NULL,
    name text NOT NULL,
    graph jsonb NOT NULL CHECK (jsonb_typeof(graph) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, workflow_id) REFERENCES workflows(tenant_id, id),
    UNIQUE (workflow_id, version)
);
