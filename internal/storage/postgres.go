package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/retry"
)

const migrationLockID = 7_302_514_601

type Postgres struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func Connect(ctx context.Context, dsn string, log *slog.Logger) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	cfg.MaxConns = 4
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	policy := retry.Policy{Attempts: 10, Delay: time.Second, MaxDelay: 10 * time.Second}
	err = policy.Do(ctx, func(ctx context.Context) error {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return pool.Ping(pingCtx)
	}, func(attempt int, wait time.Duration, err error) {
		log.Warn("postgres is not reachable yet, retrying", "attempt", attempt, "retry_in", wait, "error", err)
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	return &Postgres{pool: pool, log: log}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Migrate(ctx context.Context) error {
	migrations, err := Migrations()
	if err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied := 0
	for _, m := range migrations {
		done, err := p.applyMigration(ctx, m)
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", m.Version, err)
		}
		if done {
			applied++
			p.log.Info("database migration applied", "version", m.Version)
		}
	}
	p.log.Info("database schema is up to date", "applied", applied, "total", len(migrations))
	return nil
}

func (p *Postgres) applyMigration(ctx context.Context, m Migration) (bool, error) {
	applied := false
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(migrationLockID)); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, m.Version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.Version); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (p *Postgres) UpsertGuild(ctx context.Context, g Guild) error {
	ids, err := parseIDs(g.GuildID, g.ChannelID, g.PlayerMessageID)
	if err != nil {
		return err
	}
	var queueID *int64
	if g.QueueMessageID != "" {
		id, err := ParseID(g.QueueMessageID)
		if err != nil {
			return err
		}
		queueID = &id
	}
	var locale *string
	if g.Locale != "" {
		l := string(g.Locale)
		locale = &l
	}
	_, err = p.pool.Exec(ctx, `
		INSERT INTO guild (guild_id, channel_id, player_message_id, queue_message_id, locale, slow_mode_seconds)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (guild_id) DO UPDATE SET
			channel_id = EXCLUDED.channel_id,
			player_message_id = EXCLUDED.player_message_id,
			queue_message_id = EXCLUDED.queue_message_id,
			locale = EXCLUDED.locale,
			slow_mode_seconds = EXCLUDED.slow_mode_seconds,
			updated = now()`,
		ids[0], ids[1], ids[2], queueID, locale, int(g.SlowMode/time.Second))
	if err != nil {
		return fmt.Errorf("upsert guild: %w", err)
	}
	return nil
}

func (p *Postgres) ListGuilds(ctx context.Context) ([]Guild, error) {
	rows, err := p.pool.Query(ctx, `SELECT guild_id, channel_id, player_message_id, queue_message_id, locale, slow_mode_seconds FROM guild`)
	if err != nil {
		return nil, fmt.Errorf("list guilds: %w", err)
	}
	guilds, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Guild, error) {
		var guildID, channelID, playerID int64
		var queueID *int64
		var locale *string
		var slowMode int
		if err := row.Scan(&guildID, &channelID, &playerID, &queueID, &locale, &slowMode); err != nil {
			return Guild{}, err
		}
		g := Guild{
			GuildID:         FormatID(guildID),
			ChannelID:       FormatID(channelID),
			PlayerMessageID: FormatID(playerID),
			SlowMode:        time.Duration(slowMode) * time.Second,
		}
		if queueID != nil {
			g.QueueMessageID = FormatID(*queueID)
		}
		if locale != nil {
			if l, err := i18n.Parse(*locale); err == nil {
				g.Locale = l
			}
		}
		return g, nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan guilds: %w", err)
	}
	return guilds, nil
}

func (p *Postgres) DeleteGuild(ctx context.Context, guildID string) error {
	id, err := ParseID(guildID)
	if err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx, `DELETE FROM guild WHERE guild_id = $1`, id); err != nil {
		return fmt.Errorf("delete guild: %w", err)
	}
	return nil
}

func (p *Postgres) AddHistory(ctx context.Context, guildID string, e HistoryEntry) error {
	id, err := ParseID(guildID)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO history (guild_id, author, title, uri) VALUES ($1, $2, $3, $4)`, id, e.Author, e.Title, e.URI)
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23503" {
			return nil
		}
		return fmt.Errorf("add history: %w", err)
	}
	return nil
}

func (p *Postgres) ListHistory(ctx context.Context, guildID string, limit int) ([]HistoryEntry, error) {
	id, err := ParseID(guildID)
	if err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		SELECT author, title, uri, COALESCE(added, now()::timestamp)
		FROM history WHERE guild_id = $1
		ORDER BY id DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (HistoryEntry, error) {
		var e HistoryEntry
		err := row.Scan(&e.Author, &e.Title, &e.URI, &e.Added)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan history: %w", err)
	}
	return entries, nil
}
