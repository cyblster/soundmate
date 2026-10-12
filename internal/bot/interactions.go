package bot

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/lavalink"

	"github.com/cyblster/soundmate/internal/format"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/ui"
	"github.com/cyblster/soundmate/internal/vote"
)

func (b *Bot) onInteraction(_ *discordgo.Session, e *discordgo.InteractionCreate) {
	i := e.Interaction
	if i.GuildID == "" || i.Member == nil || i.Member.User == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("panic while handling interaction", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	if age := interactionAge(i); age > lateDelivery {
		b.log.Warn("interaction delivered late", "guild", b.guildName(i.GuildID), "interaction", interactionName(i), "age", age)
	}
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch i.ApplicationCommandData().Name {
		case CommandSetup:
			b.handleSetup(i)
		case CommandAdd:
			b.handleAddCommand(i)
		case CommandSkip:
			b.handleVote(i, vote.Skip)
		case CommandDisconnect:
			b.handleVote(i, vote.Disconnect)
		case CommandHistory:
			b.handleHistory(i)
		}
	case discordgo.InteractionMessageComponent:
		data := i.MessageComponentData()
		switch data.CustomID {
		case ui.IDAdd:
			b.handleAdd(i, ui.SourceYouTube)
		case ui.IDSkip:
			b.handleVote(i, vote.Skip)
		case ui.IDDisconnect:
			b.handleVote(i, vote.Disconnect)
		case ui.IDHistory:
			b.handleHistory(i)
		default:
			if id, ok := ui.ParseSelectID(data.CustomID); ok {
				b.handleSelect(i, id, data.Values)
			}
		}
	case discordgo.InteractionModalSubmit:
		if i.ModalSubmitData().CustomID == ui.IDAddModal {
			b.handleAddModal(i)
		}
	}
}

func (b *Bot) checkEnqueue(i *discordgo.Interaction, m *i18n.Messages) (string, bool) {
	if _, ok := b.guilds.Get(i.GuildID); !ok {
		b.reply(i, m.NotConfigured, noticeTTL)
		return "", false
	}
	if !b.nodeReady.Load() {
		b.reply(i, m.PlayerUnavailable, noticeTTL)
		return "", false
	}
	userChannel := b.voiceChannel(i.GuildID, i.Member.User.ID)
	botChannel := b.voiceChannel(i.GuildID, b.botID())
	if check := CheckEnqueue(userChannel, botChannel, b.players.Get(i.GuildID).Active()); check != VoiceOK {
		b.reply(i, check.Message(m), noticeTTL)
		return "", false
	}
	if notice, ok := b.slowMode(i, m); !ok {
		b.reply(i, notice, noticeTTL)
		return "", false
	}
	return userChannel, true
}

func (b *Bot) slowMode(i *discordgo.Interaction, m *i18n.Messages) (string, bool) {
	if ExemptFromSlowMode(i.Member) {
		return "", true
	}
	if until, ok := b.cooldowns.Remaining(cooldownKey(i)); ok {
		return fmt.Sprintf(m.SlowMode, until.Unix()), false
	}
	return "", true
}

func (b *Bot) slowModePeriod(guildID string) time.Duration {
	if g, ok := b.guilds.Get(guildID); ok {
		return g.SlowMode
	}
	return 0
}

func cooldownKey(i *discordgo.Interaction) string {
	return i.GuildID + ":" + i.Member.User.ID
}

func requester(i *discordgo.Interaction) music.Requester {
	return music.Requester{ID: i.Member.User.ID, Name: DisplayName(i.Member)}
}

func (b *Bot) handleAdd(i *discordgo.Interaction, source string) {
	m := b.messagesFor(i)
	if _, ok := b.checkEnqueue(i, m); !ok {
		return
	}
	b.respond(i, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: ui.AddModal(m, source)})
}

func (b *Bot) handleAddModal(i *discordgo.Interaction) {
	m := b.messagesFor(i)
	channel, ok := b.checkEnqueue(i, m)
	if !ok {
		return
	}
	components := i.ModalSubmitData().Components
	query := ui.ModalValue(components, ui.IDQuery)
	if query == "" {
		b.reply(i, m.SearchNothing, noticeTTL)
		return
	}
	b.addQuery(i, m, channel, query, ui.ModalValue(components, ui.IDSource))
}

func (b *Bot) handleAddCommand(i *discordgo.Interaction) {
	options := i.ApplicationCommandData().Options
	query := strings.TrimSpace(OptionString(options, OptionQuery))
	source := OptionString(options, OptionSource)
	if query == "" {
		b.handleAdd(i, source)
		return
	}
	m := b.messagesFor(i)
	channel, ok := b.checkEnqueue(i, m)
	if !ok {
		return
	}
	b.addQuery(i, m, channel, query, source)
}

func SearchIdentifier(query, source string) string {
	if format.IsURL(query) {
		return query
	}
	if ui.SearchSource(source) == ui.SourceSoundCloud {
		return lavalink.SearchTypeSoundCloud.Apply(query)
	}
	return lavalink.SearchTypeYouTube.Apply(query)
}

func (b *Bot) addQuery(i *discordgo.Interaction, m *i18n.Messages, channel, query, source string) {
	if !b.deferReply(i) {
		return
	}
	identifier := SearchIdentifier(query, source)
	ctx, cancel := b.timeout()
	result, err := b.loadTracks(ctx, identifier)
	cancel()
	if err != nil {
		b.log.Error("failed to load tracks", "guild", b.guildName(i.GuildID), "query", query, "error", err)
		b.editReply(i, m.SearchFailed, noticeTTL)
		return
	}
	requester := requester(i)
	switch data := result.Data.(type) {
	case lavalink.Track:
		track := music.FromLavalink(data, requester)
		b.enqueueAndReport(i, m, channel, []music.Track{track}, fmt.Sprintf(m.TrackAdded, format.EscapeMarkdown(track.Title)))
	case lavalink.Playlist:
		tracks := music.FromLavalinkList(data.Tracks, requester)
		if len(tracks) == 0 {
			b.editReply(i, m.SearchNothing, noticeTTL)
			return
		}
		b.enqueueAndReport(i, m, channel, tracks, fmt.Sprintf(m.PlaylistAdded, len(tracks), format.EscapeMarkdown(data.Info.Name)))
	case lavalink.Search:
		if len(data) == 0 {
			b.editReply(i, m.SearchNothing, noticeTTL)
			return
		}
		tracks := music.FromLavalinkList(data[:min(len(data), ui.SearchLimit)], requester)
		b.showSearch(i, m, tracks)
	case lavalink.Exception:
		b.log.Warn("lavalink failed to load tracks", "guild", b.guildName(i.GuildID), "query", query, "severity", data.Severity, "message", data.Message)
		b.editReply(i, m.SearchFailed, noticeTTL)
	default:
		b.editReply(i, m.SearchNothing, noticeTTL)
	}
}

func (b *Bot) showSearch(i *discordgo.Interaction, m *i18n.Messages, tracks []music.Track) {
	id := b.searches.Put(pendingSearch{interaction: i, tracks: tracks})
	components := ui.SearchMessage(m, id, tracks)
	_, err := b.session.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Components: &components,
		Flags:      discordgo.MessageFlagsIsComponentsV2,
	})
	if err != nil {
		b.searches.Take(id)
		b.log.Warn("failed to show search results", "guild", b.guildName(i.GuildID), "error", err)
	}
}

func (b *Bot) enqueueAndReport(i *discordgo.Interaction, m *i18n.Messages, channel string, tracks []music.Track, confirmation string) {
	if err := b.enqueue(i.GuildID, channel, tracks); err != nil {
		b.log.Error("failed to enqueue tracks", "guild", b.guildName(i.GuildID), "error", err)
		b.editReply(i, m.GenericError, noticeTTL)
		return
	}
	b.cooldowns.Start(cooldownKey(i), b.slowModePeriod(i.GuildID))
	b.editReply(i, confirmation, confirmTTL)
}

func (b *Bot) handleSelect(i *discordgo.Interaction, searchID string, values []string) {
	m := b.messagesFor(i)
	pending, ok := b.searches.Take(searchID)
	if !ok {
		b.reply(i, m.SearchExpired, noticeTTL)
		return
	}
	defer b.deleteAfter(pending.interaction, confirmTTL)
	index := -1
	if len(values) == 1 {
		if v, err := strconv.Atoi(values[0]); err == nil && v >= 0 && v < len(pending.tracks) {
			index = v
		}
	}
	if index < 0 {
		b.updateMessage(i, m.GenericError)
		return
	}
	userChannel := b.voiceChannel(i.GuildID, i.Member.User.ID)
	botChannel := b.voiceChannel(i.GuildID, b.botID())
	if check := CheckEnqueue(userChannel, botChannel, b.players.Get(i.GuildID).Active()); check != VoiceOK {
		b.updateMessage(i, check.Message(m))
		return
	}
	if notice, ok := b.slowMode(i, m); !ok {
		b.updateMessage(i, notice)
		return
	}
	track := pending.tracks[index]
	if !b.updateMessage(i, fmt.Sprintf(m.TrackAdded, format.EscapeMarkdown(track.Title))) {
		return
	}
	if err := b.enqueue(i.GuildID, userChannel, []music.Track{track}); err != nil {
		b.log.Error("failed to enqueue track", "guild", b.guildName(i.GuildID), "error", err)
		return
	}
	b.cooldowns.Start(cooldownKey(i), b.slowModePeriod(i.GuildID))
}

func (b *Bot) handleHistory(i *discordgo.Interaction) {
	m := b.messagesFor(i)
	ctx, cancel := b.timeout()
	entries, err := b.store.ListHistory(ctx, i.GuildID, ui.HistoryLimit)
	cancel()
	if err != nil {
		b.log.Error("failed to load history", "guild", b.guildName(i.GuildID), "error", err)
		b.reply(i, m.GenericError, noticeTTL)
		return
	}
	ok := b.respond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Components: ui.HistoryMessage(m, entries),
			Flags:      discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
		},
	})
	if ok {
		b.deleteAfter(i, historyTTL)
	}
}
