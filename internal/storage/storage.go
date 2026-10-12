package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cyblster/soundmate/internal/i18n"
)

const HistorySize = 20

type Guild struct {
	GuildID         string
	ChannelID       string
	PlayerMessageID string
	QueueMessageID  string
	Locale          i18n.Locale
	SlowMode        time.Duration
}

type HistoryEntry struct {
	Author string
	Title  string
	URI    string
	Added  time.Time
}

type GuildRepository interface {
	UpsertGuild(ctx context.Context, guild Guild) error
	ListGuilds(ctx context.Context) ([]Guild, error)
	DeleteGuild(ctx context.Context, guildID string) error
}

type HistoryRepository interface {
	AddHistory(ctx context.Context, guildID string, entry HistoryEntry) error
	ListHistory(ctx context.Context, guildID string, limit int) ([]HistoryEntry, error)
}

func ParseID(id string) (int64, error) {
	v, err := strconv.ParseInt(id, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid snowflake %q", id)
	}
	return v, nil
}

func FormatID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func parseIDs(ids ...string) ([]int64, error) {
	result := make([]int64, len(ids))
	for i, id := range ids {
		v, err := ParseID(id)
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}
