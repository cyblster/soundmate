package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/ui"
)

type fakePlayer struct {
	disgolink.Player
	mu        sync.Mutex
	played    []string
	destroyed int
	stopped   int
	failPlay  bool
}

func (p *fakePlayer) Update(_ context.Context, opts ...lavalink.PlayerUpdateOpt) error {
	update := &lavalink.PlayerUpdate{}
	for _, opt := range opts {
		opt(update)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failPlay {
		return errors.New("lavalink unavailable")
	}
	if update.Track != nil && update.Track.Encoded != nil {
		if update.Track.Encoded.IsNull() {
			p.stopped++
		} else {
			p.played = append(p.played, update.Track.Encoded.Value())
		}
	}
	return nil
}

func (p *fakePlayer) Destroy(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyed++
	return nil
}

func (p *fakePlayer) stopCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func (p *fakePlayer) destroyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.destroyed
}

func (p *fakePlayer) history() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.played)
}

type fakeNode struct {
	disgolink.Node
	result      *lavalink.LoadResult
	err         error
	identifiers []string
}

func (n *fakeNode) LoadTracks(_ context.Context, identifier string) (*lavalink.LoadResult, error) {
	n.identifiers = append(n.identifiers, identifier)
	return n.result, n.err
}

type fakeLink struct {
	disgolink.Client
	mu          sync.Mutex
	player      *fakePlayer
	exists      bool
	node        *fakeNode
	voiceStates []string
	voiceActive atomic.Int32
	voiceRace   atomic.Bool
}

func (l *fakeLink) Player(snowflake.ID) disgolink.Player {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.exists = true
	return l.player
}

func (l *fakeLink) ExistingPlayer(snowflake.ID) disgolink.Player {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.exists {
		return nil
	}
	return l.player
}

func (l *fakeLink) RemovePlayer(snowflake.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.exists = false
}

func (l *fakeLink) BestNode() disgolink.Node {
	return l.node
}

func (l *fakeLink) enterVoice() func() {
	if l.voiceActive.Add(1) > 1 {
		l.voiceRace.Store(true)
	}
	time.Sleep(time.Millisecond)
	return func() { l.voiceActive.Add(-1) }
}

func (l *fakeLink) OnVoiceStateUpdate(_ context.Context, _ snowflake.ID, channelID *snowflake.ID, _ string) {
	defer l.enterVoice()()
	l.mu.Lock()
	defer l.mu.Unlock()
	if channelID == nil {
		l.voiceStates = append(l.voiceStates, "")
		return
	}
	l.voiceStates = append(l.voiceStates, channelID.String())
}

func (l *fakeLink) OnVoiceServerUpdate(context.Context, snowflake.ID, string, string) {
	defer l.enterVoice()()
}

func (l *fakeLink) Close() {}

type playbackHarness struct {
	*harness
	link  *fakeLink
	joins []string
	mu    sync.Mutex
}

func newPlaybackHarness(t *testing.T) *playbackHarness {
	t.Helper()
	h := &playbackHarness{harness: newHarness(t, true), link: &fakeLink{player: &fakePlayer{}, node: &fakeNode{}}}
	h.bot.link = h.link
	h.bot.nodeReady.Store(true)
	h.bot.voiceJoin = func(_, channelID string, _, _ bool) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.joins = append(h.joins, channelID)
		return nil
	}
	return h
}

func (h *playbackHarness) voiceJoins() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.joins)
}

func (h *playbackHarness) waitForLeave(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if joins := h.voiceJoins(); joins[len(joins)-1] == "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("bot must leave after the idle timeout, joins = %v", h.voiceJoins())
}

func queueTracks(names ...string) []music.Track {
	result := make([]music.Track, len(names))
	for i, n := range names {
		result[i] = music.Track{Encoded: "enc-" + n, Title: n, Author: "Artist", URI: "https://example.com/" + n, Requester: "alice"}
	}
	return result
}

func lavalinkTrack(name string) lavalink.Track {
	uri := "https://www.youtube.com/watch?v=" + name
	return lavalink.Track{Encoded: "enc-" + name, Info: lavalink.TrackInfo{Identifier: name, Title: name, Author: "Artist", URI: &uri, SourceName: "youtube", Length: 200_000}}
}

func endEvent(reason lavalink.TrackEndReason) lavalink.TrackEndEvent {
	return lavalink.TrackEndEvent{Reason: reason, GuildID_: snowflake.MustParse(testGuild)}
}

func TestEnqueueStartsPlaybackOnce(t *testing.T) {
	h := newPlaybackHarness(t)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a", "b")); err != nil {
		t.Fatal(err)
	}
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("c")); err != nil {
		t.Fatal(err)
	}
	if joins := h.voiceJoins(); !slices.Equal(joins, []string{testVoice}) {
		t.Fatalf("voice joins = %v, want one join", joins)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a"}) {
		t.Fatalf("played = %v, want [enc-a]", played)
	}
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 2 {
		t.Fatalf("queue length = %d, want 2", total)
	}
}

func TestEnqueueJoinFailureResetsPlayer(t *testing.T) {
	h := newPlaybackHarness(t)
	h.bot.voiceJoin = func(string, string, bool, bool) error { return errors.New("gateway closed") }
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err == nil {
		t.Fatal("enqueue must report voice join failures")
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player must be reset after a failed join")
	}
	if h.link.player.destroyed != 1 || h.link.ExistingPlayer(snowflake.MustParse(testGuild)) != nil {
		t.Fatal("lavalink player must be destroyed after a failed join")
	}
}

func TestEnqueueSendsTrackBeforeJoiningVoice(t *testing.T) {
	h := newPlaybackHarness(t)
	var playedAtJoin []string
	h.bot.voiceJoin = func(string, string, bool, bool) error {
		playedAtJoin = h.link.player.history()
		return nil
	}
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(playedAtJoin, []string{"enc-a"}) {
		t.Fatalf("track must reach lavalink before the voice join so its response cannot overwrite voice state, played at join = %v", playedAtJoin)
	}
}

func TestTrackEndAdvancesQueueAndLeavesAtEnd(t *testing.T) {
	h := newPlaybackHarness(t)
	h.bot.idleTimeout = time.Hour
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a", "b")); err != nil {
		t.Fatal(err)
	}
	h.bot.onTrackEnd(h.link.player, endEvent(lavalink.TrackEndReasonReplaced))
	if played := h.link.player.history(); len(played) != 1 {
		t.Fatalf("replaced tracks must not advance the queue, played %v", played)
	}
	h.bot.onTrackEnd(h.link.player, endEvent(lavalink.TrackEndReasonFinished))
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("played = %v", played)
	}
	h.bot.onTrackEnd(h.link.player, endEvent(lavalink.TrackEndReasonFinished))
	if joins := h.voiceJoins(); !slices.Equal(joins, []string{testVoice}) {
		t.Fatalf("bot must stay in the channel right after the queue ends, joins = %v", joins)
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player must be idle after the queue ends")
	}
	if guilds := h.bot.idleGuilds(); !slices.Equal(guilds, []string{testGuild}) {
		t.Fatalf("idle leave must be scheduled, idle guilds = %v", guilds)
	}
	h.bot.idleTimeout = time.Millisecond
	h.bot.scheduleIdleLeave(testGuild)
	h.waitForLeave(t)
	if joins := h.voiceJoins(); !slices.Equal(joins, []string{testVoice, ""}) {
		t.Fatalf("bot must leave once, joins = %v", joins)
	}
	if destroyed := h.link.player.destroyCount(); destroyed != 1 {
		t.Fatalf("player must be destroyed once, destroyed = %d", destroyed)
	}
}

func TestSkipAndStuckAdvance(t *testing.T) {
	h := newPlaybackHarness(t)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a", "b", "c")); err != nil {
		t.Fatal(err)
	}
	h.bot.skip(testGuild)
	h.bot.onTrackStuck(h.link.player, lavalink.TrackStuckEvent{GuildID_: snowflake.MustParse(testGuild)})
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b", "enc-c"}) {
		t.Fatalf("played = %v", played)
	}
}

func TestPlayFailureSkipsBrokenTracks(t *testing.T) {
	h := newPlaybackHarness(t)
	h.bot.idleTimeout = 20 * time.Millisecond
	h.link.player.failPlay = true
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a", "b")); err != nil {
		t.Fatal(err)
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player must give up after every track failed")
	}
	h.waitForLeave(t)
}

func TestTrackStartSavesHistory(t *testing.T) {
	h := newPlaybackHarness(t)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.bot.onTrackStart(h.link.player, lavalink.TrackStartEvent{GuildID_: snowflake.MustParse(testGuild)})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if entries, _ := h.store.ListHistory(context.Background(), testGuild, 10); len(entries) == 1 {
			if entries[0].Title != "a" || entries[0].URI != "https://example.com/a" {
				t.Fatalf("unexpected history entry: %+v", entries[0])
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("history entry was not saved")
}

func TestBotLeavesEmptyChannel(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testUser, testVoice)
	h.joinVoice(t, testBot, testVoice)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.joinVoice(t, testUser, "")
	h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: testUser}})
	if joins := h.voiceJoins(); joins[len(joins)-1] != "" {
		t.Fatalf("bot must leave an empty channel, joins = %v", joins)
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player must be reset")
	}
}

func TestBotVoiceEventsAreSerialized(t *testing.T) {
	h := newPlaybackHarness(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: testBot, ChannelID: testVoice, SessionID: "s"}})
		}()
		go func() {
			defer wg.Done()
			h.bot.onVoiceServerUpdate(nil, &discordgo.VoiceServerUpdate{GuildID: testGuild, Token: "t", Endpoint: "e"})
		}()
	}
	wg.Wait()
	if h.link.voiceRace.Load() {
		t.Fatal("voice state and server updates must not reach lavalink concurrently")
	}
}

func TestBotVoiceStateIsForwardedToLavalink(t *testing.T) {
	h := newPlaybackHarness(t)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: testBot, ChannelID: testVoice, SessionID: "s"}})
	h.bot.onVoiceStateUpdate(nil, &discordgo.VoiceStateUpdate{VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: testBot, SessionID: "s"}})
	if !slices.Equal(h.link.voiceStates, []string{testVoice, ""}) {
		t.Fatalf("voice states forwarded = %v", h.link.voiceStates)
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("bot disconnect must reset the queue")
	}
}

func modalSubmit(query string) *discordgo.InteractionCreate {
	return interaction(discordgo.InteractionModalSubmit, discordgo.ModalSubmitInteractionData{
		CustomID: ui.IDAddModal,
		Components: []discordgo.MessageComponent{&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: ui.IDQuery, Value: query},
		}}},
	}, nil)
}

func originalEdit(t *testing.T, requests []request) map[string]any {
	t.Helper()
	for _, r := range requests {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.Path, "/messages/@original") {
			return r.Body
		}
	}
	t.Fatalf("no original response edit in %+v", requests)
	return nil
}

func TestModalSearchShowsResultsAndSelectPlays(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeSearch, Data: lavalink.Search{
		lavalinkTrack("a"), lavalinkTrack("b"), lavalinkTrack("c"), lavalinkTrack("d"), lavalinkTrack("e"), lavalinkTrack("f"),
	}}
	h.bot.onInteraction(nil, modalSubmit("never gonna"))
	if h.link.node.identifiers[0] != "ytsearch:never gonna" {
		t.Fatalf("plain text must be searched on youtube, got %q", h.link.node.identifiers[0])
	}
	requests := h.discord.take()
	if kind, _, flags := callbackContent(t, requests); kind != int(discordgo.InteractionResponseDeferredChannelMessageWithSource) || flags != int(discordgo.MessageFlagsEphemeral) {
		t.Fatalf("modal must be deferred ephemerally, got type %d flags %d", kind, flags)
	}
	edit := originalEdit(t, requests)
	componentsV2(t, edit)
	menu := findComponent(edit["components"], discordgo.SelectMenuComponent)
	options := menu["options"].([]any)
	if len(options) != ui.SearchLimit {
		t.Fatalf("expected %d options, got %d", ui.SearchLimit, len(options))
	}
	searchID, ok := ui.ParseSelectID(menu["custom_id"].(string))
	if !ok || h.bot.searches.Len() != 1 {
		t.Fatal("search results must be stored for selection")
	}

	h.bot.onInteraction(nil, interaction(discordgo.InteractionMessageComponent,
		discordgo.MessageComponentInteractionData{CustomID: ui.SelectID(searchID), Values: []string{"2"}}, nil))
	kind, content, _ := callbackContent(t, h.discord.take())
	if kind != int(discordgo.InteractionResponseUpdateMessage) || !strings.Contains(content, "c") {
		t.Fatalf("unexpected select response %d %q", kind, content)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-c"}) {
		t.Fatalf("played = %v, want [enc-c]", played)
	}
	if current := h.bot.players.Get(testGuild).Current(); current.Requester != "alice" {
		t.Fatalf("requester = %q, want alice", current.Requester)
	}
}

func TestSearchSources(t *testing.T) {
	source := &discordgo.Label{Component: &discordgo.SelectMenu{CustomID: ui.IDSource, Values: []string{ui.SourceSoundCloud}}}
	submit := interaction(discordgo.InteractionModalSubmit, discordgo.ModalSubmitInteractionData{
		CustomID: ui.IDAddModal,
		Components: []discordgo.MessageComponent{
			&discordgo.Label{Component: &discordgo.TextInput{CustomID: ui.IDQuery, Value: "daft punk"}},
			source,
		},
	}, nil)
	slash := command(CommandAdd,
		&discordgo.ApplicationCommandInteractionDataOption{Name: OptionQuery, Type: discordgo.ApplicationCommandOptionString, Value: "daft punk"},
		&discordgo.ApplicationCommandInteractionDataOption{Name: OptionSource, Type: discordgo.ApplicationCommandOptionString, Value: ui.SourceSoundCloud},
	)
	for name, e := range map[string]*discordgo.InteractionCreate{"modal": submit, "command": slash} {
		h := newPlaybackHarness(t)
		h.joinVoice(t, testUser, testVoice)
		h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeSearch, Data: lavalink.Search{lavalinkTrack("a")}}
		h.bot.onInteraction(nil, e)
		if len(h.link.node.identifiers) != 1 || h.link.node.identifiers[0] != "scsearch:daft punk" {
			t.Fatalf("%s: plain text must be searched on soundcloud, got %v", name, h.link.node.identifiers)
		}
	}
}

func TestSearchIdentifier(t *testing.T) {
	tests := []struct{ query, source, want string }{
		{"song", "", "ytsearch:song"},
		{"song", ui.SourceYouTube, "ytsearch:song"},
		{"song", ui.SourceSoundCloud, "scsearch:song"},
		{"https://soundcloud.com/forss/flickermood", ui.SourceYouTube, "https://soundcloud.com/forss/flickermood"},
	}
	for _, tt := range tests {
		if got := SearchIdentifier(tt.query, tt.source); got != tt.want {
			t.Errorf("SearchIdentifier(%q, %q) = %q, want %q", tt.query, tt.source, got, tt.want)
		}
	}
}

func TestModalURLEnqueuesPlaylist(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypePlaylist, Data: lavalink.Playlist{
		Info:   lavalink.PlaylistInfo{Name: "Mix"},
		Tracks: []lavalink.Track{lavalinkTrack("a"), lavalinkTrack("b"), lavalinkTrack("c")},
	}}
	url := "https://www.youtube.com/playlist?list=PL1"
	h.bot.onInteraction(nil, modalSubmit(url))
	if h.link.node.identifiers[0] != url {
		t.Fatalf("urls must be loaded directly, got %q", h.link.node.identifiers[0])
	}
	edit := originalEdit(t, h.discord.take())
	if want := "Added 3 tracks from **Mix**"; !strings.Contains(edit["content"].(string), want) {
		t.Fatalf("confirmation = %q, want %q", edit["content"], want)
	}
	if total := h.bot.players.Get(testGuild).Snapshot(-1).Total; total != 2 {
		t.Fatalf("queue length = %d, want 2", total)
	}
}

func TestModalReportsLoadProblems(t *testing.T) {
	m := i18n.Get(i18n.English)
	tests := []struct {
		name   string
		result *lavalink.LoadResult
		err    error
		want   string
	}{
		{"empty", &lavalink.LoadResult{LoadType: lavalink.LoadTypeEmpty, Data: lavalink.Empty{}}, nil, m.SearchNothing},
		{"empty search", &lavalink.LoadResult{LoadType: lavalink.LoadTypeSearch, Data: lavalink.Search{}}, nil, m.SearchNothing},
		{"exception", &lavalink.LoadResult{LoadType: lavalink.LoadTypeError, Data: lavalink.Exception{Message: "sign in to confirm", Severity: lavalink.SeverityCommon}}, nil, m.SearchFailed},
		{"transport", nil, errors.New("connection refused"), m.SearchFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newPlaybackHarness(t)
			h.joinVoice(t, testUser, testVoice)
			h.link.node.result, h.link.node.err = tt.result, tt.err
			h.bot.onInteraction(nil, modalSubmit("query"))
			if got := originalEdit(t, h.discord.take())["content"]; got != tt.want {
				t.Fatalf("content = %q, want %q", got, tt.want)
			}
			if h.bot.players.Get(testGuild).Active() {
				t.Fatal("nothing must be played")
			}
		})
	}
}

func TestTrackExceptionNotifiesChannel(t *testing.T) {
	h := newPlaybackHarness(t)
	h.discord.take()
	h.bot.onTrackException(h.link.player, lavalink.TrackExceptionEvent{
		Track:     lavalinkTrack("broken_*"),
		Exception: lavalink.Exception{Message: "All clients failed", Severity: lavalink.SeveritySuspicious},
		GuildID_:  snowflake.MustParse(testGuild),
	})
	requests := h.discord.take()
	if len(requests) != 1 || requests[0].Method != "POST" || requests[0].Path != "/channels/"+testChannel+"/messages" {
		t.Fatalf("unexpected requests: %+v", requests)
	}
	if want := fmt.Sprintf(i18n.Get(i18n.English).TrackFailed, `broken\_\*`); requests[0].Body["content"] != want {
		t.Fatalf("notice = %v, want %q", requests[0].Body["content"], want)
	}
}

func TestAddCommandWithQueryEnqueuesTrack(t *testing.T) {
	h := newPlaybackHarness(t)
	h.joinVoice(t, testUser, testVoice)
	h.link.node.result = &lavalink.LoadResult{LoadType: lavalink.LoadTypeTrack, Data: lavalinkTrack("song")}
	url := "https://www.youtube.com/watch?v=song"
	h.bot.onInteraction(nil, command(CommandAdd, &discordgo.ApplicationCommandInteractionDataOption{
		Name: OptionQuery, Type: discordgo.ApplicationCommandOptionString, Value: " " + url + " ",
	}))
	if h.link.node.identifiers[0] != url {
		t.Fatalf("query must be trimmed and loaded directly, got %q", h.link.node.identifiers[0])
	}
	requests := h.discord.take()
	if kind, _, flags := callbackContent(t, requests); kind != int(discordgo.InteractionResponseDeferredChannelMessageWithSource) || flags != int(discordgo.MessageFlagsEphemeral) {
		t.Fatalf("command must be deferred ephemerally, got type %d flags %d", kind, flags)
	}
	if want := fmt.Sprintf(i18n.Get(i18n.English).TrackAdded, "song"); originalEdit(t, requests)["content"] != want {
		t.Fatalf("confirmation = %v, want %q", originalEdit(t, requests)["content"], want)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-song"}) {
		t.Fatalf("played = %v, want [enc-song]", played)
	}
}

func TestEnqueueDuringIdleCancelsLeave(t *testing.T) {
	h := newPlaybackHarness(t)
	h.bot.idleTimeout = time.Hour
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.bot.onTrackEnd(h.link.player, endEvent(lavalink.TrackEndReasonFinished))
	if len(h.bot.idleGuilds()) != 1 {
		t.Fatal("idle leave must be scheduled after the queue ends")
	}
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("b")); err != nil {
		t.Fatal(err)
	}
	if joins := h.voiceJoins(); slices.Contains(joins, "") {
		t.Fatalf("bot must not leave after a track was added, joins = %v", joins)
	}
	if played := h.link.player.history(); !slices.Equal(played, []string{"enc-a", "enc-b"}) {
		t.Fatalf("played = %v", played)
	}
	if len(h.bot.idleGuilds()) != 0 {
		t.Fatal("idle timer must be cancelled")
	}
}

func TestLeaveCancelsIdleTimer(t *testing.T) {
	h := newPlaybackHarness(t)
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.bot.onTrackEnd(h.link.player, endEvent(lavalink.TrackEndReasonFinished))
	if guilds := h.bot.idleGuilds(); !slices.Equal(guilds, []string{testGuild}) {
		t.Fatalf("idle guilds = %v", guilds)
	}
	h.bot.leave(testGuild)
	if len(h.bot.idleGuilds()) != 0 {
		t.Fatal("leave must cancel the idle timer")
	}
	h.bot.skip(testGuild)
	if len(h.bot.idleGuilds()) != 0 {
		t.Fatal("skip without a current track must do nothing")
	}
}

func TestSkipLastTrackStopsPlayback(t *testing.T) {
	h := newPlaybackHarness(t)
	h.bot.idleTimeout = time.Hour
	if err := h.bot.enqueue(testGuild, testVoice, queueTracks("a")); err != nil {
		t.Fatal(err)
	}
	h.bot.skip(testGuild)
	if stopped := h.link.player.stopCount(); stopped != 1 {
		t.Fatalf("skipping the last track must stop playback, stopped = %d", stopped)
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player must be idle after the last track is skipped")
	}
	if joins := h.voiceJoins(); !slices.Equal(joins, []string{testVoice}) {
		t.Fatalf("bot must stay in the channel until the idle timeout, joins = %v", joins)
	}
	if guilds := h.bot.idleGuilds(); !slices.Equal(guilds, []string{testGuild}) {
		t.Fatalf("idle leave must be scheduled, idle guilds = %v", guilds)
	}
}
