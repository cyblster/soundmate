DROP INDEX IF EXISTS ix_history_guild_id;

CREATE OR REPLACE FUNCTION history_keep_latest() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM history
    WHERE guild_id = NEW.guild_id AND id < (
        SELECT id FROM history WHERE guild_id = NEW.guild_id ORDER BY id DESC OFFSET 19 LIMIT 1
    );
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS history_keep_latest ON history;

CREATE TRIGGER history_keep_latest
    AFTER INSERT ON history
    FOR EACH ROW EXECUTE FUNCTION history_keep_latest();

DELETE FROM history h
USING (
    SELECT id, row_number() OVER (PARTITION BY guild_id ORDER BY id DESC) AS rank FROM history
) ranked
WHERE h.id = ranked.id AND ranked.rank > 20;
