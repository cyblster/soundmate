package bot

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"

	"github.com/cyblster/soundmate/internal/format"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/storage"
)

func (b *Bot) enqueue(guildID, channelID string, tracks []music.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	player := b.players.Get(guildID)
	start := player.Enqueue(tracks...)
	if start != nil {
		b.cancelIdleLeave(guildID)
		b.play(guildID, *start)
		if err := b.join(guildID, channelID); err != nil {
			b.destroyPlayer(guildID)
			player.Reset()
			return fmt.Errorf("join voice channel: %w", err)
		}
	}
	b.log.Info("tracks added to queue",
		"guild", b.guildName(guildID),
		"count", len(tracks),
		"first", tracks[0].Author+" - "+tracks[0].Title,
		"requester", tracks[0].Requester,
	)
	b.scheduleRender(guildID)
	return nil
}

func (b *Bot) play(guildID string, track music.Track) {
	ctx, cancel := b.timeout()
	err := b.link.Player(toSnowflake(guildID)).Update(ctx, lavalink.WithEncodedTrack(track.Encoded))
	cancel()
	if err != nil {
		b.log.Error("failed to start track", "guild", b.guildName(guildID), "title", track.Title, "error", err)
		b.advance(guildID)
	}
}

func (b *Bot) advance(guildID string) {
	b.votes.Clear(guildID)
	next := b.players.Get(guildID).Advance()
	if next == nil {
		b.log.Info("queue finished", "guild", b.guildName(guildID), "leave_in", b.idleTimeout)
		b.stop(guildID)
		b.scheduleRender(guildID)
		b.scheduleIdleLeave(guildID)
		return
	}
	b.play(guildID, *next)
}

func (b *Bot) stop(guildID string) {
	player := b.link.ExistingPlayer(toSnowflake(guildID))
	if player == nil {
		return
	}
	ctx, cancel := b.timeout()
	defer cancel()
	if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
		b.log.Warn("failed to stop playback", "guild", b.guildName(guildID), "error", err)
	}
}

func (b *Bot) skip(guildID string) {
	current := b.players.Get(guildID).Current()
	if current == nil {
		return
	}
	b.log.Info("track skipped", "guild", b.guildName(guildID), "title", current.Title)
	b.advance(guildID)
}

func (b *Bot) onTrackStart(_ disgolink.Player, e lavalink.TrackStartEvent) {
	guildID := e.GuildID().String()
	current := b.players.Get(guildID).Current()
	if current == nil {
		return
	}
	b.log.Info("playing track",
		"guild", b.guildName(guildID),
		"author", current.Author,
		"title", current.Title,
		"uri", current.URI,
		"requester", current.Requester,
	)
	b.scheduleRender(guildID)
	if _, ok := b.guilds.Get(guildID); !ok {
		return
	}
	go func(t music.Track) {
		ctx, cancel := b.timeout()
		defer cancel()
		entry := storage.HistoryEntry{Author: t.Author, Title: t.Title, URI: t.URI}
		if err := b.store.AddHistory(ctx, guildID, entry); err != nil {
			b.log.Warn("failed to save history", "guild", b.guildName(guildID), "error", err)
		}
	}(*current)
}

func (b *Bot) onTrackEnd(_ disgolink.Player, e lavalink.TrackEndEvent) {
	guildID := e.GuildID().String()
	b.log.Debug("track ended", "guild", b.guildName(guildID), "title", e.Track.Info.Title, "reason", e.Reason)
	if !e.Reason.MayStartNext() || !b.players.Get(guildID).Active() {
		return
	}
	b.advance(guildID)
}

func (b *Bot) onTrackException(_ disgolink.Player, e lavalink.TrackExceptionEvent) {
	guildID := e.GuildID().String()
	b.log.Warn("track playback failed",
		"guild", b.guildName(guildID),
		"title", e.Track.Info.Title,
		"severity", e.Exception.Severity,
		"message", e.Exception.Message,
		"cause", e.Exception.Cause,
	)
	m := i18n.Get(b.guildLocale(guildID))
	b.notify(guildID, fmt.Sprintf(m.TrackFailed, format.EscapeMarkdown(format.Truncate(e.Track.Info.Title, 100))), noticeTTL)
}

func (b *Bot) onTrackStuck(_ disgolink.Player, e lavalink.TrackStuckEvent) {
	guildID := e.GuildID().String()
	b.log.Warn("track is stuck, skipping", "guild", b.guildName(guildID), "title", e.Track.Info.Title, "threshold", e.Threshold)
	b.advance(guildID)
}

func (b *Bot) onWebSocketClosed(_ disgolink.Player, e lavalink.WebSocketClosedEvent) {
	b.log.Warn("voice connection closed", "guild", b.guildName(e.GuildID().String()), "code", e.Code, "reason", e.Reason, "by_remote", e.ByRemote)
}

func (b *Bot) loadTracks(ctx context.Context, identifier string) (*lavalink.LoadResult, error) {
	node := b.link.BestNode()
	if node == nil {
		return nil, fmt.Errorf("no lavalink node available")
	}
	return node.LoadTracks(ctx, identifier)
}
