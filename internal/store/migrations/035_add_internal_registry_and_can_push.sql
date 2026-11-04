ALTER TABLE registries ADD COLUMN internal INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX idx_registries_internal ON registries(internal) WHERE internal = 1;

ALTER TABLE proxy_tokens ADD COLUMN can_push INTEGER NOT NULL DEFAULT 0;
