package bot

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/lavalink"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/ui"
)

func findComponent(node any, kind discordgo.ComponentType) map[string]any {
	switch v := node.(type) {
	case map[string]any:
		if v["type"] == float64(kind) {
			return v
		}
		return findComponent(v["components"], kind)
	case []any:
		for _, c := range v {
			if found := findComponent(c, kind); found != nil {
				return found
			}
		}
	}
	return nil
}

func memberButton(id, userID string) *discordgo.InteractionCreate {
	return interaction(discordgo.InteractionMessageComponent, discordgo.MessageComponentInteractionData{CustomID: id},
		&discordgo.Member{User: &discordgo.User{ID: userID, Username: "user" + userID}})
}

func newVoteHarness(t *testing.T, listeners ...string) *playbackHarness {
	t.Helper()
	h := newPlaybackHarness(t)
	h.bot.idleTimeout = time.Hour
	h.joinVoice(t, testBot, testVoice)
	for _, id := range listeners {
		h.joinVoice(t, id, testVoice)
	}
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a", "b", "c")); err != nil {
		t.Fatal(err)
	}
	h.discord.take()
	return h
}

func TestSingleListenerSkipsAndDisconnectsAtOnce(t *testing.T) {
	m := i18n.Get(i18n.English)
	h := newVoteHarness(t, testUser)

	h.bot.onInteraction(nil, command(CommandSkip))
	if kind, content, flags := callbackContent(t, h.discord.take()); kind != int(discordgo.InteractionResponseChannelMessageWithSource) || content != m.Skipped || flags != int(discordgo.MessageFlagsEphemeral) {
		t.Fatalf("unexpected /skip response %d %q %d", kind, content, flags)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("played = %v", played)
	}

	h.bot.onInteraction(nil, button(ui.IDDisconnect))
	if _, content, _ := callbackContent(t, h.discord.take()); content != m.Disconnected {
		t.Fatalf("unexpected disconnect response %q", content)
	}
	if joins := h.voiceJoins(); joins[len(joins)-1] != "" || h.bot.players.Get(testGuild).Active() {
		t.Fatalf("bot must leave and reset the player, joins = %v", joins)
	}
}

func TestSkipNeedsMajority(t *testing.T) {
	m := i18n.Get(i18n.English)
	h := newVoteHarness(t, testUser, "401", "402")

	h.bot.onInteraction(nil, button(ui.IDSkip))
	if _, content, _ := callbackContent(t, h.discord.take()); content != fmt.Sprintf(m.VoteSkip, 1, 2) {
		t.Fatalf("unexpected first vote response %q", content)
	}
	if played := h.link.player.history(); len(played) != 1 {
		t.Fatalf("one vote of three listeners must not skip, played = %v", played)
	}

	h.bot.onInteraction(nil, memberButton(ui.IDSkip, "401"))
	if _, content, _ := callbackContent(t, h.discord.take()); content != m.Skipped {
		t.Fatalf("unexpected second vote response %q", content)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("majority must skip, played = %v", played)
	}

	h.bot.onInteraction(nil, memberButton(ui.IDSkip, "402"))
	if _, content, _ := callbackContent(t, h.discord.take()); content != fmt.Sprintf(m.VoteSkip, 1, 2) {
		t.Fatalf("votes must restart for the next track, got %q", content)
	}
}

func TestDisconnectNeedsMajority(t *testing.T) {
	m := i18n.Get(i18n.English)
	h := newVoteHarness(t, testUser, "401")
	h.bot.onInteraction(nil, command(CommandDisconnect))
	if _, content, _ := callbackContent(t, h.discord.take()); content != fmt.Sprintf(m.VoteDisconnect, 1, 2) || !h.bot.players.Get(testGuild).Active() {
		t.Fatalf("one of two listeners must not disconnect the bot, got %q", content)
	}
	h.bot.onInteraction(nil, memberButton(ui.IDDisconnect, "401"))
	if _, content, _ := callbackContent(t, h.discord.take()); content != m.Disconnected || h.bot.players.Get(testGuild).Active() || !slices.Contains(h.voiceJoins(), "") {
		t.Fatalf("two of two listeners must disconnect the bot, got %q", content)
	}
}

func TestRepeatedVoteIsWithdrawn(t *testing.T) {
	m := i18n.Get(i18n.English)
	h := newVoteHarness(t, testUser, "401", "402")
	h.bot.onInteraction(nil, button(ui.IDSkip))
	h.discord.take()
	h.bot.onInteraction(nil, button(ui.IDSkip))
	if _, content, _ := callbackContent(t, h.discord.take()); content != m.VoteRetracted {
		t.Fatalf("unexpected response %q", content)
	}
	h.bot.onInteraction(nil, memberButton(ui.IDSkip, "401"))
	if _, content, _ := callbackContent(t, h.discord.take()); content != fmt.Sprintf(m.VoteSkip, 1, 2) {
		t.Fatalf("withdrawn vote must not count, got %q", content)
	}
}

func TestRequesterSkipsOwnTrack(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testBot, testVoice)
	for _, id := range []string{testUser, "401", "402"} {
		h.joinVoice(t, id, testVoice)
	}
	tracks := queueTracks("a", "b")
	tracks[0].RequesterID = testUser
	if err := h.bot.enqueue(testGuild, testVoice, tracks); err != nil {
		t.Fatal(err)
	}
	h.bot.onInteraction(nil, button(ui.IDSkip))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).Skipped {
		t.Fatalf("requester must skip without a vote, got %q", content)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("played = %v", played)
	}
}

func TestVotePassesWhenListenersLeave(t *testing.T) {
	h := newVoteHarness(t, testUser, "401")
	h.bot.onInteraction(nil, button(ui.IDSkip))
	h.discord.take()
	h.joinVoice(t, "401", "")
	h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: "401"}})
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("remaining votes must skip once enough listeners left, played = %v", played)
	}
	notice := false
	for _, r := range h.discord.take() {
		if r.Path == "/channels/200/messages" && r.Body["content"] == i18n.Get(i18n.English).Skipped {
			notice = true
		}
	}
	if !notice {
		t.Fatal("channel must be told why the track was skipped")
	}
}

func TestVotesOfLeftListenersDoNotCount(t *testing.T) {
	m := i18n.Get(i18n.English)
	h := newVoteHarness(t, testUser, "401", "402", "403")
	h.bot.onInteraction(nil, memberButton(ui.IDSkip, "401"))
	h.joinVoice(t, "401", "999")
	h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: "401", ChannelID: "999"}})
	h.discord.take()
	h.bot.onInteraction(nil, button(ui.IDSkip))
	if _, content, _ := callbackContent(t, h.discord.take()); content != fmt.Sprintf(m.VoteSkip, 1, 2) {
		t.Fatalf("vote of a listener who left must not count, got %q", content)
	}
}

func addCommand(member *discordgo.Member) *discordgo.InteractionCreate {
	return interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{
		Name:    CommandAdd,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: OptionQuery, Type: discordgo.ApplicationCommandOptionString, Value: "https://www.youtube.com/watch?v=song"}},
	}, member)
}

func TestSlowModeLimitsAdding(t *testing.T) {
	h := newPlaybackHarness(t)
	h.setSlowMode(t, time.Minute)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeTrack, Data: lavalinkTrack("song")}

	h.bot.onInteraction(nil, addCommand(nil))
	h.discord.take()
	h.bot.onInteraction(nil, addCommand(nil))
	_, content, _ := callbackContent(t, h.discord.take())
	until, ok := h.bot.cooldowns.Remaining(testGuild + ":" + testUser)
	if !ok || content != fmt.Sprintf(i18n.Get(i18n.English).SlowMode, until.Unix()) || !strings.Contains(content, "<t:") {
		t.Fatalf("second add within a minute must be rejected, got %q", content)
	}
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 0 || len(h.link.node.identifiers) != 1 {
		t.Fatalf("rejected add must not search or enqueue, queue = %d", total)
	}

	h.bot.onInteraction(nil, button(ui.IDAdd))
	if _, content, _ := callbackContent(t, h.discord.take()); !strings.HasPrefix(content, "⏳") {
		t.Fatalf("add button must respect slow mode before opening the form, got %q", content)
	}

	other := &discordgo.Member{User: &discordgo.User{ID: "401"}}
	h.joinVoice(t, "401", testVoice)
	h.bot.onInteraction(nil, addCommand(other))
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 1 {
		t.Fatalf("slow mode must be per user, queue = %d", total)
	}

	moderator := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionManageMessages}
	h.bot.onInteraction(nil, addCommand(moderator))
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 2 {
		t.Fatalf("moderators must bypass slow mode, queue = %d", total)
	}
}

func TestFailedAddDoesNotStartSlowMode(t *testing.T) {
	h := newPlaybackHarness(t)
	h.setSlowMode(t, time.Minute)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeEmpty, Data: lavalink.Empty{}}
	h.bot.onInteraction(nil, addCommand(nil))
	if _, ok := h.bot.cooldowns.Remaining(testGuild + ":" + testUser); ok {
		t.Fatal("slow mode must start only after a track was added")
	}
}

func (h *harness) setSlowMode(t *testing.T, period time.Duration) {
	t.Helper()
	g, _ := h.bot.guilds.Get(testGuild)
	g.SlowMode = period
	if err := h.bot.guilds.Save(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

func TestSlowModeOffAllowsAdding(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeTrack, Data: lavalinkTrack("song")}
	h.bot.onInteraction(nil, addCommand(nil))
	h.bot.onInteraction(nil, addCommand(nil))
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 1 {
		t.Fatalf("disabled slow mode must not limit adding, queue = %d", total)
	}
}

func TestPlayerShowsGuildSlowMode(t *testing.T) {
	h := newHarness(t, true)
	h.setSlowMode(t, 2*time.Minute)
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	m := i18n.Get(i18n.English)
	if player := texts(h.discord.take()[0].Body["components"]); !strings.Contains(player, m.SlowModeHint) {
		t.Fatalf("player must show the slow mode, got %q", player)
	}
}
