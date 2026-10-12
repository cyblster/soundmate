package bot

import (
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/ui"
)

const (
	noticeTTL  = 10 * time.Second
	confirmTTL = 5 * time.Second
	historyTTL = 60 * time.Second

	lateDelivery = 2 * time.Second
)

func (b *Bot) respond(i *discordgo.Interaction, response *discordgo.InteractionResponse) bool {
	age := interactionAge(i)
	start := time.Now()
	if err := b.session.InteractionRespond(i, response); err != nil {
		b.log.Warn("failed to respond to interaction", "guild", b.guildName(i.GuildID), "interaction", interactionName(i), "type", response.Type,
			"age", age, "took", time.Since(start).Round(time.Millisecond), "error", err)
		return false
	}
	return true
}

func (b *Bot) reply(i *discordgo.Interaction, content string, ttl time.Duration) {
	ok := b.respond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
	if ok {
		b.deleteAfter(i, ttl)
	}
}

func (b *Bot) deferReply(i *discordgo.Interaction) bool {
	return b.respond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
}

func (b *Bot) updateMessage(i *discordgo.Interaction, content string) bool {
	return b.respond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Components: ui.Notice(content),
			Flags:      discordgo.MessageFlagsIsComponentsV2,
		},
	})
}

func (b *Bot) editReply(i *discordgo.Interaction, content string, ttl time.Duration) {
	_, err := b.session.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content:    &content,
		Embeds:     &[]*discordgo.MessageEmbed{},
		Components: &[]discordgo.MessageComponent{},
	})
	if err != nil {
		b.log.Warn("failed to edit interaction response", "guild", b.guildName(i.GuildID), "error", err)
		return
	}
	b.deleteAfter(i, ttl)
}

func (b *Bot) deleteAfter(i *discordgo.Interaction, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	time.AfterFunc(ttl, func() {
		_ = b.session.InteractionResponseDelete(i)
	})
}

func (b *Bot) notify(guildID, content string, ttl time.Duration) {
	g, ok := b.guilds.Get(guildID)
	if !ok {
		return
	}
	msg, err := b.session.ChannelMessageSend(g.ChannelID, content)
	if err != nil {
		b.log.Warn("failed to send notice", "guild", b.guildName(guildID), "error", err)
		return
	}
	time.AfterFunc(ttl, func() {
		_ = b.session.ChannelMessageDelete(msg.ChannelID, msg.ID)
	})
}

func interactionName(i *discordgo.Interaction) string {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		return "/" + i.ApplicationCommandData().Name
	case discordgo.InteractionMessageComponent:
		id := i.MessageComponentData().CustomID
		if _, ok := ui.ParseSelectID(id); ok {
			return ui.SelectID("")
		}
		return id
	case discordgo.InteractionModalSubmit:
		return i.ModalSubmitData().CustomID
	}
	return i.Type.String()
}

func interactionAge(i *discordgo.Interaction) time.Duration {
	created, err := discordgo.SnowflakeTimestamp(i.ID)
	if err != nil {
		return 0
	}
	return time.Since(created).Round(time.Millisecond)
}

func (b *Bot) followUp(i *discordgo.Interaction, content string, ttl time.Duration) bool {
	msg, err := b.session.FollowupMessageCreate(i, true, &discordgo.WebhookParams{Content: content, Flags: discordgo.MessageFlagsEphemeral})
	if err != nil {
		b.log.Warn("failed to send follow-up message", "guild", b.guildName(i.GuildID), "interaction", interactionName(i), "error", err)
		return false
	}
	if ttl > 0 {
		time.AfterFunc(ttl, func() {
			_ = b.session.FollowupMessageDelete(i, msg.ID)
		})
	}
	return true
}
