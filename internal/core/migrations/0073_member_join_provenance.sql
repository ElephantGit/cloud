ALTER TABLE tenant_invitations ADD COLUMN note text NOT NULL DEFAULT '' CHECK(length(note)<=200);
ALTER TABLE tenant_join_links ADD COLUMN note text NOT NULL DEFAULT '' CHECK(length(note)<=200);
ALTER TABLE tenant_invitations ADD CONSTRAINT invitation_tenant_identity UNIQUE(id,tenant_id);
ALTER TABLE tenant_join_requests ADD CONSTRAINT join_request_tenant_identity UNIQUE(id,tenant_id);

-- Existing redemptions do not prove a membership transition. Leave historical
-- memberships without provenance rather than inventing an inviter or join time.
CREATE TABLE membership_join_events (
 id uuid PRIMARY KEY,
 tenant_id uuid NOT NULL,
 user_id uuid NOT NULL,
 method text NOT NULL CHECK(method IN ('workspace_created','invitation','application','directory')),
 actor_user_id uuid NOT NULL,
 invitation_id uuid,
 request_id uuid,
 joined_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,user_id) REFERENCES tenant_memberships(tenant_id,user_id),
 FOREIGN KEY(tenant_id,actor_user_id) REFERENCES tenant_memberships(tenant_id,user_id),
 FOREIGN KEY(invitation_id,tenant_id) REFERENCES tenant_invitations(id,tenant_id),
 FOREIGN KEY(request_id,tenant_id) REFERENCES tenant_join_requests(id,tenant_id),
 CHECK((method='invitation')=(invitation_id IS NOT NULL)),
 CHECK((method='application')=(request_id IS NOT NULL))
);
CREATE INDEX membership_join_history ON membership_join_events(tenant_id,user_id,joined_at DESC,id);
