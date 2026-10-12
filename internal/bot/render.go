package bot

import (
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/storage"
	"github.com/cyblster/soundmate/internal/ui"
)

func (b *Bot) scheduleRender(guildID string) {
	b.renderer.Trigger(guildID)
}

func (b *Bot) renderLogged(guildID string) {
	if err := b.render(guildID); err != nil {
		b.log.Warn("failed to update player messages", "guild", b.guildName(guildID), "error", err)
	}
}

func (b *Bot) render(guildID string) error {
	g, ok := b.guilds.Get(guildID)
	if !ok {
		return nil
	}
	player := ui.PlayerMessage(i18n.Get(b.guildLocale(guildID)), b.playerState(guildID))
	err := b.editMessage(g.ChannelID, g.PlayerMessageID, player)
	if err == nil && g.QueueMessageID != "" {
		b.dropQueueMessage(g)
	}
	if IsNotFound(err) {
		ctx, cancel := b.timeout()
		defer cancel()
		if delErr := b.guilds.Delete(ctx, guildID); delErr != nil {
			b.log.Error("failed to delete guild configuration", "guild", b.guildName(guildID), "error", delErr)
		}
		b.forgetRendered(g.PlayerMessageID, g.QueueMessageID)
		return fmt.Errorf("player channel or messages were deleted, run /setup again: %w", err)
	}
	return err
}

func (b *Bot) editMessage(channelID, messageID string, components []discordgo.MessageComponent) error {
	print := Fingerprint(components)
	if last, ok := b.rendered.Load(messageID); ok && last == print {
		return nil
	}
	ctx, cancel := b.timeout()
	defer cancel()
	content := ""
	_, err := b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:          messageID,
		Channel:     channelID,
		Content:     &content,
		Embeds:      &[]*discordgo.MessageEmbed{},
		Components:  &components,
		Attachments: &[]*discordgo.MessageAttachment{},
		Flags:       discordgo.MessageFlagsIsComponentsV2,
	}, discordgo.WithContext(ctx))
	if err != nil {
		b.rendered.Delete(messageID)
		return err
	}
	b.rendered.Store(messageID, print)
	return nil
}

func (b *Bot) forgetRendered(messageIDs ...string) {
	for _, id := range messageIDs {
		b.rendered.Delete(id)
	}
}

func Fingerprint(components []discordgo.MessageComponent) uint64 {
	data, err := json.Marshal(components)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

func (b *Bot) playerState(guildID string) ui.Player {
	snap := b.players.Get(guildID).Snapshot(ui.QueueLimit)
	skip, disconnect := b.voteState(guildID)
	return ui.Player{
		Current:    snap.Current,
		Upcoming:   snap.Upcoming,
		Total:      snap.Total,
		Length:     snap.Length,
		Skip:       skip,
		Disconnect: disconnect,
		SlowMode:   b.slowModePeriod(guildID),
	}
}

func (b *Bot) dropQueueMessage(g storage.Guild) {
	if err := b.session.ChannelMessageDelete(g.ChannelID, g.QueueMessageID); err != nil && !IsNotFound(err) {
		b.log.Warn("failed to delete legacy queue message", "guild", b.guildName(g.GuildID), "error", err)
		return
	}
	b.forgetRendered(g.QueueMessageID)
	g.QueueMessageID = ""
	ctx, cancel := b.timeout()
	defer cancel()
	if err := b.guilds.Save(ctx, g); err != nil {
		b.log.Warn("failed to forget legacy queue message", "guild", b.guildName(g.GuildID), "error", err)
		return
	}
	b.log.Info("merged queue into the player message", "guild", b.guildName(g.GuildID))
}
