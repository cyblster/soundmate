package bot

import (
	"context"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) voiceChannel(guildID, userID string) string {
	if vs, err := b.session.State.VoiceState(guildID, userID); err == nil && vs != nil {
		return vs.ChannelID
	}
	return ""
}

func (b *Bot) botID() string {
	if u := b.session.State.User; u != nil {
		return u.ID
	}
	return ""
}

func (b *Bot) listeners(guildID, channelID string) int {
	b.session.State.RLock()
	defer b.session.State.RUnlock()
	for _, g := range b.session.State.Guilds {
		if g.ID == guildID {
			return CountListeners(g.VoiceStates, channelID, b.botID())
		}
	}
	return 0
}

func (b *Bot) onVoiceStateUpdate(s *discordgo.Session, e *discordgo.VoiceStateUpdate) {
	botID := b.botID()
	if e.UserID == botID {
		b.handleBotVoiceState(e)
		return
	}
	channel := b.voiceChannel(e.GuildID, botID)
	if channel == "" {
		return
	}
	if b.listeners(e.GuildID, channel) > 0 {
		b.recheckVotes(e.GuildID, channel)
		b.scheduleRender(e.GuildID)
		return
	}
	b.log.Info("voice channel is empty, leaving", "guild", b.guildName(e.GuildID))
	b.leave(e.GuildID)
}

func (b *Bot) handleBotVoiceState(e *discordgo.VoiceStateUpdate) {
	guild := toSnowflake(e.GuildID)
	if e.ChannelID == "" {
		b.forwardVoice(func(ctx context.Context) {
			if b.link.ExistingPlayer(guild) != nil {
				b.link.OnVoiceStateUpdate(ctx, guild, nil, e.SessionID)
			}
		})
		b.log.Debug("disconnected from voice channel", "guild", b.guildName(e.GuildID))
		b.reset(e.GuildID)
		return
	}
	if !b.nodeReady.Load() {
		return
	}
	channel := toSnowflake(e.ChannelID)
	b.forwardVoice(func(ctx context.Context) {
		b.link.OnVoiceStateUpdate(ctx, guild, &channel, e.SessionID)
	})
	b.log.Debug("connected to voice channel", "guild", b.guildName(e.GuildID), "channel_id", e.ChannelID)
}

func (b *Bot) onVoiceServerUpdate(_ *discordgo.Session, e *discordgo.VoiceServerUpdate) {
	if !b.nodeReady.Load() {
		return
	}
	b.forwardVoice(func(ctx context.Context) {
		b.link.OnVoiceServerUpdate(ctx, toSnowflake(e.GuildID), e.Token, e.Endpoint)
	})
}

func (b *Bot) forwardVoice(forward func(ctx context.Context)) {
	b.voiceMu.Lock()
	defer b.voiceMu.Unlock()
	ctx, cancel := b.timeout()
	defer cancel()
	forward(ctx)
}

func (b *Bot) join(guildID, channelID string) error {
	return b.voiceJoin(guildID, channelID, false, true)
}

func (b *Bot) leave(guildID string) {
	if err := b.voiceJoin(guildID, "", false, false); err != nil {
		b.log.Warn("failed to leave voice channel", "guild", b.guildName(guildID), "error", err)
	}
	b.destroyPlayer(guildID)
	b.reset(guildID)
}

func (b *Bot) destroyPlayer(guildID string) {
	if b.link == nil {
		return
	}
	id := toSnowflake(guildID)
	player := b.link.ExistingPlayer(id)
	if player == nil {
		return
	}
	ctx, cancel := b.timeout()
	defer cancel()
	if err := player.Destroy(ctx); err != nil {
		b.log.Debug("failed to destroy lavalink player", "guild", b.guildName(guildID), "error", err)
	}
	b.link.RemovePlayer(id)
}

func (b *Bot) reset(guildID string) {
	b.cancelIdleLeave(guildID)
	b.votes.Clear(guildID)
	b.players.Get(guildID).Reset()
	b.scheduleRender(guildID)
}
