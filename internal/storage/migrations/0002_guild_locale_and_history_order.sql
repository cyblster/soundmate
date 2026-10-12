ALTER TABLE guild ADD COLUMN IF NOT EXISTS locale TEXT;

CREATE INDEX IF NOT EXISTS ix_history_guild_id_id ON history (guild_id, id DESC);
