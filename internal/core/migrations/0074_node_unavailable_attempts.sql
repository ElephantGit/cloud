ALTER TABLE operations ADD COLUMN IF NOT EXISTS node_unavailable_attempts bigint NOT NULL DEFAULT 0
    CHECK (node_unavailable_attempts >= 0);
