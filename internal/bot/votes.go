package bot

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/ui"
	"github.com/cyblster/soundmate/internal/vote"
)

func (b *Bot) handleVote(i *discordgo.Interaction, kind vote.Kind) {
	m := b.messagesFor(i)
	userID := i.Member.User.ID
	botChannel := b.voiceChannel(i.GuildID, b.botID())
	if check := CheckControl(b.voiceChannel(i.GuildID, userID), botChannel); check != VoiceOK {
		b.reply(i, check.Message(m), noticeTTL)
		return
	}
	current := b.players.Get(i.GuildID).Current()
	if current == nil {
		b.reply(i, m.NothingPlaying, noticeTTL)
		return
	}
	if kind == vote.Skip && current.RequesterID != "" && current.RequesterID == userID {
		b.reply(i, m.Skipped, confirmTTL)
		b.skip(i.GuildID)
		return
	}
	r := b.votes.Cast(i.GuildID, kind, userID, b.inChannel(i.GuildID, botChannel), vote.Needed(b.listeners(i.GuildID, botChannel)))
	b.log.Info("vote cast", "guild", b.guildName(i.GuildID), "kind", voteName(kind), "user", DisplayName(i.Member),
		"added", r.Added, "votes", r.Count, "needed", r.Needed)
	switch {
	case r.Passed:
		b.reply(i, votePassedMessage(m, kind), confirmTTL)
		b.executeVote(i.GuildID, kind)
	case r.Added:
		b.reply(i, fmt.Sprintf(voteCastMessage(m, kind), r.Count, r.Needed), confirmTTL)
		b.scheduleRender(i.GuildID)
	default:
		b.reply(i, m.VoteRetracted, confirmTTL)
		b.scheduleRender(i.GuildID)
	}
}

func (b *Bot) recheckVotes(guildID, botChannel string) {
	if !b.players.Get(guildID).Active() {
		return
	}
	needed := vote.Needed(b.listeners(guildID, botChannel))
	for _, kind := range vote.Kinds {
		if b.votes.Check(guildID, kind, b.inChannel(guildID, botChannel), needed) {
			b.log.Info("vote passed after listeners left", "guild", b.guildName(guildID), "kind", voteName(kind))
			b.notify(guildID, votePassedMessage(i18n.Get(b.guildLocale(guildID)), kind), noticeTTL)
			b.executeVote(guildID, kind)
			return
		}
	}
}

func (b *Bot) executeVote(guildID string, kind vote.Kind) {
	switch kind {
	case vote.Skip:
		b.skip(guildID)
	case vote.Disconnect:
		b.log.Info("disconnected by vote", "guild", b.guildName(guildID))
		b.leave(guildID)
	}
}

func (b *Bot) voteState(guildID string) (skip, disconnect ui.Votes) {
	botChannel := b.voiceChannel(guildID, b.botID())
	if botChannel == "" {
		return
	}
	needed := vote.Needed(b.listeners(guildID, botChannel))
	eligible := b.inChannel(guildID, botChannel)
	skip = ui.Votes{Count: b.votes.Count(guildID, vote.Skip, eligible), Needed: needed}
	disconnect = ui.Votes{Count: b.votes.Count(guildID, vote.Disconnect, eligible), Needed: needed}
	return
}

func (b *Bot) inChannel(guildID, channelID string) func(userID string) bool {
	return func(userID string) bool {
		return channelID != "" && b.voiceChannel(guildID, userID) == channelID
	}
}

func voteName(kind vote.Kind) string {
	if kind == vote.Disconnect {
		return "disconnect"
	}
	return "skip"
}

func votePassedMessage(m *i18n.Messages, kind vote.Kind) string {
	if kind == vote.Disconnect {
		return m.Disconnected
	}
	return m.Skipped
}

func voteCastMessage(m *i18n.Messages, kind vote.Kind) string {
	if kind == vote.Disconnect {
		return m.VoteDisconnect
	}
	return m.VoteSkip
}
