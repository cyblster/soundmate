CREATE TABLE IF NOT EXISTS guild (
    id                SERIAL PRIMARY KEY,
    added             TIMESTAMP DEFAULT now(),
    updated           TIMESTAMP,
    guild_id          BIGINT NOT NULL,
    channel_id        BIGINT NOT NULL,
    player_message_id BIGINT NOT NULL,
    queue_message_id  BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ix_guild_guild_id ON guild (guild_id);

CREATE TABLE IF NOT EXISTS history (
    id       SERIAL PRIMARY KEY,
    added    TIMESTAMP DEFAULT now(),
    updated  TIMESTAMP,
    guild_id BIGINT NOT NULL,
    author   TEXT NOT NULL,
    title    TEXT NOT NULL,
    uri      TEXT NOT NULL,
    CONSTRAINT fk_history_guild_id_guild FOREIGN KEY (guild_id)
        REFERENCES guild (guild_id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_history_guild_id ON history (guild_id);
