package storage

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cyblster/soundmate/internal/i18n"
)

func connectTestDB(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("SOUNDMATE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOUNDMATE_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := Connect(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	t.Cleanup(db.Close)
	for _, stmt := range []string{"DROP TABLE IF EXISTS history", "DROP TABLE IF EXISTS guild", "DROP TABLE IF EXISTS schema_migrations"} {
		if _, err := db.pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	return db
}

func TestPostgresMigrateIsIdempotent(t *testing.T) {
	db := connectTestDB(t)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate() error: %v", err)
	}
	var applied int
	if err := db.pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	migrations, _ := Migrations()
	if applied != len(migrations) {
		t.Fatalf("applied migrations = %d, want %d", applied, len(migrations))
	}
}

func TestPostgresGuildLifecycle(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	g := Guild{GuildID: "123456789012345678", ChannelID: "2", PlayerMessageID: "3", QueueMessageID: "4", Locale: i18n.Russian, SlowMode: 2 * time.Minute}
	if err := db.UpsertGuild(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.ChannelID, g.QueueMessageID, g.Locale, g.SlowMode = "20", "", "", 0
	if err := db.UpsertGuild(ctx, g); err != nil {
		t.Fatal(err)
	}
	guilds, err := db.ListGuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(guilds) != 1 || guilds[0] != g {
		t.Fatalf("ListGuilds() = %+v, want [%+v]", guilds, g)
	}
	if err := db.DeleteGuild(ctx, g.GuildID); err != nil {
		t.Fatal(err)
	}
	if guilds, _ := db.ListGuilds(ctx); len(guilds) != 0 {
		t.Fatalf("guild was not deleted: %+v", guilds)
	}
	if err := db.UpsertGuild(ctx, Guild{GuildID: "bad"}); err == nil {
		t.Fatal("invalid ids must be rejected")
	}
}

func TestPostgresHistoryRetentionAndCascade(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	for _, guildID := range []string{"1", "2"} {
		if err := db.UpsertGuild(ctx, Guild{GuildID: guildID, ChannelID: "2", PlayerMessageID: "3", QueueMessageID: "4"}); err != nil {
			t.Fatal(err)
		}
	}
	const inserted = HistorySize + 5
	for n := range inserted {
		title := strconv.Itoa(n)
		if err := db.AddHistory(ctx, "1", HistoryEntry{Author: "x", Title: title, URI: "https://example.com/" + title}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddHistory(ctx, "2", HistoryEntry{Author: "y", Title: "other", URI: "https://example.com/other"}); err != nil {
		t.Fatal(err)
	}
	var stored int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM history WHERE guild_id = 1").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != HistorySize {
		t.Fatalf("stored rows = %d, want %d", stored, HistorySize)
	}
	entries, err := db.ListHistory(ctx, "1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != HistorySize || entries[0].Title != strconv.Itoa(inserted-1) || entries[HistorySize-1].Title != strconv.Itoa(inserted-HistorySize) {
		t.Fatalf("history must hold the newest %d entries, newest first, got %d entries", HistorySize, len(entries))
	}
	for _, e := range entries {
		if e.Added.IsZero() {
			t.Error("added timestamp must be set")
		}
	}
	if limited, _ := db.ListHistory(ctx, "1", 1); len(limited) != 1 || limited[0].Title != strconv.Itoa(inserted-1) {
		t.Fatalf("ListHistory(limit 1) = %+v", limited)
	}
	if other, _ := db.ListHistory(ctx, "2", 100); len(other) != 1 || other[0].Title != "other" {
		t.Fatalf("retention must not affect other guilds, got %+v", other)
	}
	if err := db.AddHistory(ctx, "999", HistoryEntry{Author: "x", Title: "orphan", URI: "u"}); err != nil {
		t.Fatalf("history for unknown guild must be ignored, got %v", err)
	}
	if err := db.DeleteGuild(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	if left, _ := db.ListHistory(ctx, "1", 10); len(left) != 0 {
		t.Fatalf("history must be deleted with the guild, got %d entries", len(left))
	}
}

func TestPostgresMigratesLegacySchema(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	legacy := []string{
		"DROP TABLE IF EXISTS history", "DROP TABLE IF EXISTS guild", "DROP TABLE IF EXISTS schema_migrations",
		`CREATE TABLE guild (id SERIAL PRIMARY KEY, added TIMESTAMP DEFAULT now(), updated TIMESTAMP,
			guild_id BIGINT NOT NULL, channel_id BIGINT NOT NULL, player_message_id BIGINT NOT NULL, queue_message_id BIGINT NOT NULL)`,
		"CREATE UNIQUE INDEX ix_guild_guild_id ON guild (guild_id)",
		"INSERT INTO guild (guild_id, channel_id, player_message_id, queue_message_id) VALUES (10, 20, 30, 40)",
		`CREATE TABLE history (id SERIAL PRIMARY KEY, added TIMESTAMP DEFAULT now(), updated TIMESTAMP, guild_id BIGINT NOT NULL,
			author TEXT NOT NULL, title TEXT NOT NULL, uri TEXT NOT NULL,
			CONSTRAINT fk_history_guild_id_guild FOREIGN KEY (guild_id) REFERENCES guild (guild_id) ON UPDATE CASCADE ON DELETE CASCADE)`,
		"CREATE INDEX ix_history_guild_id ON history (guild_id)",
		"INSERT INTO history (guild_id, author, title, uri) SELECT 10, 'x', n::text, 'u' FROM generate_series(1, 50) AS n",
	}
	for _, stmt := range legacy {
		if _, err := db.pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() on legacy schema error: %v", err)
	}
	guilds, err := db.ListGuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(guilds) != 1 || guilds[0].GuildID != "10" || guilds[0].Locale != "" || guilds[0].SlowMode != time.Minute {
		t.Fatalf("legacy guild was not preserved: %+v", guilds)
	}
	entries, err := db.ListHistory(ctx, "10", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != HistorySize || entries[0].Title != "50" || entries[HistorySize-1].Title != "31" {
		t.Fatalf("legacy history must be trimmed to the newest %d entries, got %d", HistorySize, len(entries))
	}
}
