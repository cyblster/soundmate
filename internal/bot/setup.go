package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/storage"
	"github.com/cyblster/soundmate/internal/ui"
)

const botChannelPermissions = discordgo.PermissionViewChannel |
	discordgo.PermissionSendMessages |
	discordgo.PermissionEmbedLinks |
	discordgo.PermissionReadMessageHistory

func (b *Bot) handleSetup(i *discordgo.Interaction) {
	m := b.messagesFor(i)
	if !IsAdmin(i.Member) {
		b.reply(i, m.SetupAdminOnly, noticeTTL)
		return
	}
	channel, err := b.session.State.Channel(i.ChannelID)
	if err != nil || channel.Type != discordgo.ChannelTypeGuildText {
		b.reply(i, m.SetupWrongChannel, noticeTTL)
		return
	}
	previous, configured := b.guilds.Get(i.GuildID)
	locale := serverLocale(i)
	if option := OptionString(i.ApplicationCommandData().Options, OptionLanguage); option != "" {
		parsed, err := i18n.Parse(option)
		if err != nil {
			b.reply(i, m.SetupUnknownLanguage, noticeTTL)
			return
		}
		locale = parsed
	}
	slowMode := defaultSlowMode
	if configured {
		slowMode = previous.SlowMode
	}
	if seconds, ok := OptionInt(i.ApplicationCommandData().Options, OptionSlowMode); ok {
		slowMode = min(time.Duration(max(seconds, 0))*time.Second, MaxSlowMode)
	}
	if !b.respond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: m.SetupInProgress, Flags: discordgo.MessageFlagsEphemeral},
	}) {
		return
	}

	guild, err := b.install(i.GuildID, i.ChannelID, locale, slowMode)
	if err != nil {
		b.log.Error("setup failed", "guild", b.guildName(i.GuildID), "channel_id", i.ChannelID, "error", err)
		b.editReply(i, m.SetupFailed, 3*noticeTTL)
		return
	}
	if configured {
		b.removeMessages(previous)
	}
	done, ttl := m.SetupDone, noticeTTL
	if configured {
		done, ttl = SetupChanges(m, previous, guild), 3*noticeTTL
	}
	if b.followUp(i, done, ttl) {
		_ = b.session.InteractionResponseDelete(i)
	} else {
		b.editReply(i, done, ttl)
	}
	b.log.Info("installed on server", "guild", b.guildName(guild.GuildID), "channel_id", guild.ChannelID, "locale", guild.Locale, "slow_mode", guild.SlowMode)
}

func (b *Bot) install(guildID, channelID string, locale i18n.Locale, slowMode time.Duration) (storage.Guild, error) {
	m := i18n.Get(locale)
	ctx, cancel := b.timeout()
	defer cancel()
	opts := discordgo.WithContext(ctx)

	botID := b.botID()
	if err := b.session.ChannelPermissionSet(channelID, botID, discordgo.PermissionOverwriteTypeMember, botChannelPermissions, 0, opts); err != nil {
		return storage.Guild{}, fmt.Errorf("allow bot to post: %w", err)
	}
	if err := b.session.ChannelPermissionSet(channelID, guildID, discordgo.PermissionOverwriteTypeRole, 0, discordgo.PermissionSendMessages, opts); err != nil {
		return storage.Guild{}, fmt.Errorf("make channel read-only: %w", err)
	}
	name := "soundmate"
	if u := b.session.State.User; u != nil {
		name = u.Username
	}
	if _, err := b.session.ChannelEdit(channelID, &discordgo.ChannelEdit{Topic: fmt.Sprintf(m.SetupChannelTopic, name)}, opts); err != nil {
		return storage.Guild{}, fmt.Errorf("set channel topic: %w", err)
	}

	state := b.playerState(guildID)
	state.SlowMode = slowMode
	playerMsg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Components:      ui.PlayerMessage(m, state),
		Flags:           discordgo.MessageFlagsIsComponentsV2,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}, opts)
	if err != nil {
		return storage.Guild{}, fmt.Errorf("send player message: %w", err)
	}

	guild := storage.Guild{
		GuildID:         guildID,
		ChannelID:       channelID,
		PlayerMessageID: playerMsg.ID,
		Locale:          locale,
		SlowMode:        slowMode,
	}
	if err := b.guilds.Save(ctx, guild); err != nil {
		_ = b.session.ChannelMessageDelete(channelID, playerMsg.ID)
		return storage.Guild{}, fmt.Errorf("save guild: %w", err)
	}
	return guild, nil
}

func (b *Bot) removeMessages(g storage.Guild) {
	b.forgetRendered(g.PlayerMessageID, g.QueueMessageID)
	for _, id := range []string{g.PlayerMessageID, g.QueueMessageID} {
		if id == "" {
			continue
		}
		if err := b.session.ChannelMessageDelete(g.ChannelID, id); err != nil && !IsNotFound(err) {
			b.log.Debug("failed to delete previous player message", "guild", b.guildName(g.GuildID), "error", err)
		}
	}
}

func SetupChanges(m *i18n.Messages, previous, current storage.Guild) string {
	var changes []string
	if previous.ChannelID != current.ChannelID {
		changes = append(changes, fmt.Sprintf(m.SetupChannelChange, "<#"+previous.ChannelID+">", "<#"+current.ChannelID+">"))
	}
	if from, to := storedLocale(previous.Locale), storedLocale(current.Locale); from != to {
		changes = append(changes, fmt.Sprintf(m.SetupLanguageChange, i18n.Get(from).LanguageName, i18n.Get(to).LanguageName))
	}
	if previous.SlowMode != current.SlowMode {
		changes = append(changes, fmt.Sprintf(m.SetupSlowModeChange, slowModeValue(m, previous.SlowMode), slowModeValue(m, current.SlowMode)))
	}
	if len(changes) == 0 {
		return m.SetupUnchanged
	}
	return m.SetupUpdated + "\n- " + strings.Join(changes, "\n- ")
}

func storedLocale(l i18n.Locale) i18n.Locale {
	if l == "" {
		return i18n.Default
	}
	return l
}

func slowModeValue(m *i18n.Messages, d time.Duration) string {
	if d <= 0 {
		return m.SlowModeDisabled
	}
	return strconv.Itoa(int(d / time.Second))
}
