package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/config"
	"github.com/cyblster/soundmate/internal/debounce"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/storage"
	"github.com/cyblster/soundmate/internal/ui"
	"github.com/cyblster/soundmate/internal/vote"
)

const (
	testGuild   = "100"
	testChannel = "200"
	testBot     = "300"
	testUser    = "400"
	testVoice   = "500"
)

type request struct {
	Method string
	Path   string
	Body   map[string]any
	Files  []string
}

type fakeDiscord struct {
	mu       sync.Mutex
	requests []request
	nextID   int
	failures map[string]int
}

func (f *fakeDiscord) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v"+discordgo.APIVersion)
	req := request{Method: r.Method, Path: path}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &req.Body)
			for _, files := range r.MultipartForm.File {
				for _, f := range files {
					req.Files = append(req.Files, f.Filename)
				}
			}
		}
	} else if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &req.Body)
	}
	f.requests = append(f.requests, req)

	if code, ok := f.failures[r.Method+" "+path]; ok {
		return jsonResponse(http.StatusNotFound, map[string]any{"message": "Unknown", "code": code}), nil
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/messages"):
		f.nextID++
		return jsonResponse(http.StatusOK, map[string]any{"id": fmt.Sprint(9000 + f.nextID), "channel_id": testChannel}), nil
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/webhooks/"):
		f.nextID++
		return jsonResponse(http.StatusOK, map[string]any{"id": fmt.Sprint(9000 + f.nextID), "channel_id": testChannel}), nil
	case strings.Contains(path, "/messages/"):
		return jsonResponse(http.StatusOK, map[string]any{"id": "1", "channel_id": testChannel}), nil
	case r.Method == http.MethodPatch && strings.HasPrefix(path, "/channels/"):
		return jsonResponse(http.StatusOK, map[string]any{"id": testChannel}), nil
	default:
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil)), Header: http.Header{}}, nil
	}
}

func jsonResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func (f *fakeDiscord) take() []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := f.requests
	f.requests = nil
	return result
}

func (f *fakeDiscord) fail(method, path string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures == nil {
		f.failures = map[string]int{}
	}
	f.failures[method+" "+path] = code
}

type memoryStore struct {
	mu      sync.Mutex
	guilds  map[string]storage.Guild
	history map[string][]storage.HistoryEntry
}

func newMemoryStore() *memoryStore {
	return &memoryStore{guilds: map[string]storage.Guild{}, history: map[string][]storage.HistoryEntry{}}
}

func (s *memoryStore) UpsertGuild(_ context.Context, g storage.Guild) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guilds[g.GuildID] = g
	return nil
}

func (s *memoryStore) ListGuilds(context.Context) ([]storage.Guild, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []storage.Guild
	for _, g := range s.guilds {
		result = append(result, g)
	}
	return result, nil
}

func (s *memoryStore) DeleteGuild(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.guilds, id)
	return nil
}

func (s *memoryStore) AddHistory(_ context.Context, id string, e storage.HistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[id] = append([]storage.HistoryEntry{e}, s.history[id]...)
	return nil
}

func (s *memoryStore) ListHistory(_ context.Context, id string, limit int) ([]storage.HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.history[id]
	return h[:min(limit, len(h))], nil
}

type harness struct {
	bot     *Bot
	discord *fakeDiscord
	store   *memoryStore
}

func newHarness(t *testing.T, configured bool) *harness {
	t.Helper()
	store := newMemoryStore()
	if configured {
		store.guilds[testGuild] = storage.Guild{
			GuildID: testGuild, ChannelID: testChannel, PlayerMessageID: "601",
		}
	}
	cfg := config.Config{Token: "token"}
	b, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), store)
	if err != nil {
		t.Fatal(err)
	}
	b.renderer = debounce.New(time.Millisecond, b.renderLogged)
	discord := &fakeDiscord{}
	b.session.Client = &http.Client{Transport: discord}
	b.session.State.User = &discordgo.User{ID: testBot, Username: "soundmate"}
	if err := b.session.State.GuildAdd(&discordgo.Guild{
		ID:   testGuild,
		Name: "Test Server",
		Channels: []*discordgo.Channel{
			{ID: testChannel, GuildID: testGuild, Type: discordgo.ChannelTypeGuildText},
			{ID: testVoice, GuildID: testGuild, Type: discordgo.ChannelTypeGuildVoice},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.guilds.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.renderer.Close)
	return &harness{bot: b, discord: discord, store: store}
}

func (h *harness) joinVoice(t *testing.T, userID, channelID string) {
	t.Helper()
	if err := h.bot.session.State.OnInterface(h.bot.session, &discordgo.VoiceStateUpdate{
		VoiceState: &discordgo.VoiceState{GuildID: testGuild, UserID: userID, ChannelID: channelID},
	}); err != nil {
		t.Fatal(err)
	}
}

func interaction(kind discordgo.InteractionType, data discordgo.InteractionData, member *discordgo.Member) *discordgo.InteractionCreate {
	if member == nil {
		member = &discordgo.Member{User: &discordgo.User{ID: testUser, Username: "alice"}}
	}
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ID: "700", AppID: testBot, Token: "token", Type: kind, GuildID: testGuild, ChannelID: testChannel,
		Locale: discordgo.EnglishUS, Member: member, Data: data,
	}}
}

func button(id string) *discordgo.InteractionCreate {
	return interaction(discordgo.InteractionMessageComponent, discordgo.MessageComponentInteractionData{CustomID: id}, nil)
}

func callback(t *testing.T, requests []request) map[string]any {
	t.Helper()
	for _, r := range requests {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/callback") {
			return r.Body
		}
	}
	t.Fatalf("no interaction callback in %+v", requests)
	return nil
}

func callbackContent(t *testing.T, requests []request) (int, string, int) {
	t.Helper()
	body := callback(t, requests)
	data, _ := body["data"].(map[string]any)
	flags, _ := data["flags"].(float64)
	return int(body["type"].(float64)), messageText(data), int(flags)
}

func messageText(data map[string]any) string {
	if content, _ := data["content"].(string); content != "" {
		return content
	}
	return texts(data["components"])
}

func texts(node any) string {
	var parts []string
	switch v := node.(type) {
	case map[string]any:
		if content, ok := v["content"].(string); ok && v["type"] == float64(discordgo.TextDisplayComponent) {
			parts = append(parts, content)
		}
		for _, key := range []string{"components", "accessory"} {
			if child, ok := v[key]; ok {
				if text := texts(child); text != "" {
					parts = append(parts, text)
				}
			}
		}
	case []any:
		for _, c := range v {
			if text := texts(c); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func componentsV2(t *testing.T, body map[string]any) {
	t.Helper()
	flags, _ := body["flags"].(float64)
	if int(flags)&int(discordgo.MessageFlagsIsComponentsV2) == 0 {
		t.Fatalf("message must use components v2, got flags %v", body["flags"])
	}
}

func TestRenderSkipsUnchangedMessages(t *testing.T) {
	h := newHarness(t, true)
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	first := h.discord.take()
	if len(first) != 1 || first[0].Method != http.MethodPatch || first[0].Path != "/channels/200/messages/601" {
		t.Fatalf("unexpected requests: %+v", first)
	}
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	if again := h.discord.take(); len(again) != 0 {
		t.Fatalf("unchanged state must not be re-sent, got %+v", again)
	}
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song", Author: "Artist"}, music.Track{Title: "Next"})
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	if changed := h.discord.take(); len(changed) != 1 {
		t.Fatalf("changed state must update the player once, got %d requests", len(changed))
	}
}

func TestRenderUsesComponentsV2(t *testing.T) {
	h := newHarness(t, true)
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song", Source: "youtube", Identifier: "id"}, music.Track{Title: "Next"})
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	requests := h.discord.take()
	for _, r := range requests {
		componentsV2(t, r.Body)
		if r.Body["content"] != "" {
			t.Fatalf("legacy content must be cleared, got %v", r.Body["content"])
		}
		if embeds, ok := r.Body["embeds"].([]any); !ok || len(embeds) != 0 {
			t.Fatalf("legacy embeds must be cleared, got %v", r.Body["embeds"])
		}
		if attachments, ok := r.Body["attachments"].([]any); !ok || len(attachments) != 0 || len(r.Files) != 0 {
			t.Fatalf("legacy attachments must be cleared, got %v", r.Body["attachments"])
		}
	}
	player := texts(requests[0].Body["components"])
	if !strings.Contains(player, "### Song") || !strings.Contains(player, "**1.** Next") {
		t.Fatalf("player must show the current track and the queue, got %q", player)
	}
}

func TestRenderDropsLegacyQueueMessage(t *testing.T) {
	h := newHarness(t, true)
	g, _ := h.bot.guilds.Get(testGuild)
	g.QueueMessageID = "602"
	if err := h.bot.guilds.Save(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	requests := h.discord.take()
	if len(requests) != 2 || requests[0].Path != "/channels/200/messages/601" || requests[1].Method != http.MethodDelete || requests[1].Path != "/channels/200/messages/602" {
		t.Fatalf("legacy queue message must be deleted after the player update, got %+v", requests)
	}
	if stored := h.store.guilds[testGuild]; stored.QueueMessageID != "" {
		t.Fatalf("legacy queue message must be forgotten, got %q", stored.QueueMessageID)
	}
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song"})
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	if again := h.discord.take(); len(again) != 1 {
		t.Fatalf("legacy queue message must be deleted once, got %+v", again)
	}
}

func TestRenderShowsVotes(t *testing.T) {
	h := newHarness(t, true)
	h.joinVoice(t, testBot, testVoice)
	h.joinVoice(t, testUser, testVoice)
	h.joinVoice(t, "401", testVoice)
	h.joinVoice(t, "402", testVoice)
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song"})
	h.bot.votes.Cast(testGuild, vote.Skip, testUser, func(string) bool { return true }, 2)
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	player := texts(h.discord.take()[0].Body["components"])
	if want := fmt.Sprintf(i18n.Get(i18n.English).VotesCast, 1, 2); !strings.Contains(player, want) {
		t.Fatalf("player must show the vote progress %q, got %q", want, player)
	}
}

func TestRenderUsesGuildLocale(t *testing.T) {
	h := newHarness(t, true)
	g, _ := h.bot.guilds.Get(testGuild)
	g.Locale = i18n.Russian
	if err := h.bot.guilds.Save(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if err := h.bot.render(testGuild); err != nil {
		t.Fatal(err)
	}
	body := h.discord.take()[0].Body
	if text := texts(body["components"]); !strings.Contains(text, i18n.Get(i18n.Russian).IdleTitle) {
		t.Fatalf("player message must use the guild locale, got %q", text)
	}
}

func TestRenderForgetsDeletedSetup(t *testing.T) {
	h := newHarness(t, true)
	h.discord.fail(http.MethodPatch, "/channels/200/messages/601", discordgo.ErrCodeUnknownMessage)
	if err := h.bot.render(testGuild); err == nil {
		t.Fatal("render must report deleted messages")
	}
	if _, ok := h.bot.guilds.Get(testGuild); ok {
		t.Fatal("guild configuration must be removed from cache")
	}
	if _, ok := h.store.guilds[testGuild]; ok {
		t.Fatal("guild configuration must be removed from storage")
	}
}

func TestRenderIgnoresUnknownGuild(t *testing.T) {
	h := newHarness(t, false)
	if err := h.bot.render(testGuild); err != nil || len(h.discord.take()) != 0 {
		t.Fatal("unconfigured guild must not be rendered")
	}
}

func TestSetupRequiresAdmin(t *testing.T) {
	h := newHarness(t, false)
	h.bot.onInteraction(nil, interaction(discordgo.InteractionApplicationCommand,
		discordgo.ApplicationCommandInteractionData{Name: CommandSetup}, nil))
	kind, content, flags := callbackContent(t, h.discord.take())
	if kind != int(discordgo.InteractionResponseChannelMessageWithSource) || content != i18n.Get(i18n.English).SetupAdminOnly || flags != int(discordgo.MessageFlagsEphemeral) {
		t.Fatalf("unexpected response: %d %q %d", kind, content, flags)
	}
}

func TestSetupInstallsPlayer(t *testing.T) {
	h := newHarness(t, true)
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	h.bot.onInteraction(nil, interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{
		Name: CommandSetup,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: OptionLanguage, Type: discordgo.ApplicationCommandOptionString, Value: "ru"},
		},
	}, admin))
	requests := h.discord.take()

	var paths []string
	for _, r := range requests {
		paths = append(paths, r.Method+" "+r.Path)
	}
	want := []string{
		"POST /interactions/700/token/callback",
		"PUT /channels/200/permissions/300",
		"PUT /channels/200/permissions/100",
		"PATCH /channels/200",
		"POST /channels/200/messages",
		"DELETE /channels/200/messages/601",
		"POST /webhooks/300/token",
		"DELETE /webhooks/300/token/messages/@original",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected request sequence:\n%s\nwant:\n%s", strings.Join(paths, "\n"), strings.Join(want, "\n"))
	}
	if deny := requests[2].Body["deny"]; deny != fmt.Sprint(discordgo.PermissionSendMessages) {
		t.Errorf("everyone must be denied sending messages, got %v", deny)
	}
	if topic := requests[3].Body["topic"]; topic != "Канал музыкального бота soundmate" {
		t.Errorf("unexpected topic %v", topic)
	}
	g, ok := h.store.guilds[testGuild]
	if !ok || g.PlayerMessageID != "9001" || g.QueueMessageID != "" || g.Locale != i18n.Russian {
		t.Fatalf("unexpected stored guild: %+v", g)
	}
	done := requests[len(requests)-2].Body
	if done["content"] != i18n.Get(i18n.English).SetupUpdated+"\n- Language: English → Русский" || done["flags"] != float64(discordgo.MessageFlagsEphemeral) {
		t.Errorf("repeated setup must list the changes privately after the player in the user locale, got %v", done)
	}
	if cached, _ := h.bot.guilds.Get(testGuild); cached != g {
		t.Fatal("registry must be updated after setup")
	}
}

func TestSetupRejectsNonTextChannel(t *testing.T) {
	h := newHarness(t, false)
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	e := interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{Name: CommandSetup}, admin)
	e.ChannelID = testVoice
	h.bot.onInteraction(nil, e)
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).SetupWrongChannel {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestAddRequiresConfiguredGuild(t *testing.T) {
	h := newHarness(t, false)
	h.bot.onInteraction(nil, button(ui.IDAdd))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).NotConfigured {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestAddRequiresLavalink(t *testing.T) {
	h := newHarness(t, true)
	h.bot.onInteraction(nil, button(ui.IDAdd))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).PlayerUnavailable {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestAddChecksVoiceState(t *testing.T) {
	h := newHarness(t, true)
	h.bot.nodeReady.Store(true)
	m := i18n.Get(i18n.English)

	h.bot.onInteraction(nil, button(ui.IDAdd))
	if _, content, _ := callbackContent(t, h.discord.take()); content != m.UserNotInVoice {
		t.Fatalf("unexpected response %q", content)
	}

	h.joinVoice(t, testUser, testVoice)
	h.bot.onInteraction(nil, button(ui.IDAdd))
	body := callback(t, h.discord.take())
	if int(body["type"].(float64)) != int(discordgo.InteractionResponseModal) {
		t.Fatalf("expected modal response, got %v", body)
	}
	data := body["data"].(map[string]any)
	if data["custom_id"] != ui.IDAddModal || data["title"] != m.ModalTitle {
		t.Fatalf("unexpected modal: %v", data)
	}
}

func TestAddRejectsWhenBotPlaysElsewhere(t *testing.T) {
	h := newHarness(t, true)
	h.bot.nodeReady.Store(true)
	h.joinVoice(t, testUser, testVoice)
	h.joinVoice(t, testBot, "999")
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song"})
	h.bot.onInteraction(nil, button(ui.IDAdd))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).BotInAnotherChannel {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestVoteRequiresSameChannelAndTrack(t *testing.T) {
	m := i18n.Get(i18n.English)
	for _, id := range []string{ui.IDSkip, ui.IDDisconnect} {
		h := newHarness(t, true)
		h.joinVoice(t, testUser, testVoice)
		h.bot.onInteraction(nil, button(id))
		if _, content, _ := callbackContent(t, h.discord.take()); content != m.UserNotWithBot {
			t.Fatalf("%s: unexpected response %q", id, content)
		}
		h.joinVoice(t, testBot, testVoice)
		h.bot.onInteraction(nil, button(id))
		kind, content, flags := callbackContent(t, h.discord.take())
		if kind != int(discordgo.InteractionResponseChannelMessageWithSource) || content != m.NothingPlaying || flags != int(discordgo.MessageFlagsEphemeral) {
			t.Fatalf("%s: expected private notice, got type %d %q flags %d", id, kind, content, flags)
		}
	}
}

func command(name string, options ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.InteractionCreate {
	return interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{Name: name, Options: options}, nil)
}

func TestControlCommandsRequireSameChannel(t *testing.T) {
	m := i18n.Get(i18n.English)
	for _, name := range []string{CommandSkip, CommandDisconnect} {
		h := newHarness(t, true)
		h.bot.onInteraction(nil, command(name))
		if _, content, flags := callbackContent(t, h.discord.take()); content != m.UserNotInVoice || flags != int(discordgo.MessageFlagsEphemeral) {
			t.Fatalf("/%s: unexpected response %q", name, content)
		}
		h.joinVoice(t, testUser, testVoice)
		h.bot.onInteraction(nil, command(name))
		if _, content, _ := callbackContent(t, h.discord.take()); content != m.UserNotWithBot {
			t.Fatalf("/%s: unexpected response %q", name, content)
		}
	}
}

func TestAddCommandWithoutQueryOpensModal(t *testing.T) {
	h := newHarness(t, true)
	h.bot.nodeReady.Store(true)
	h.joinVoice(t, testUser, testVoice)
	h.bot.onInteraction(nil, command(CommandAdd, &discordgo.ApplicationCommandInteractionDataOption{
		Name: OptionQuery, Type: discordgo.ApplicationCommandOptionString, Value: "   ",
	}, &discordgo.ApplicationCommandInteractionDataOption{
		Name: OptionSource, Type: discordgo.ApplicationCommandOptionString, Value: ui.SourceSoundCloud,
	}))
	body := callback(t, h.discord.take())
	data := body["data"].(map[string]any)
	if int(body["type"].(float64)) != int(discordgo.InteractionResponseModal) || data["custom_id"] != ui.IDAddModal {
		t.Fatalf("expected add modal, got %v", body)
	}
	options := data["components"].([]any)[1].(map[string]any)["component"].(map[string]any)["options"].([]any)
	if options[1].(map[string]any)["default"] != true {
		t.Fatalf("requested source must be preselected, got %v", options)
	}
}

func TestAddCommandChecksSetupAndVoice(t *testing.T) {
	query := &discordgo.ApplicationCommandInteractionDataOption{Name: OptionQuery, Type: discordgo.ApplicationCommandOptionString, Value: "song"}
	h := newHarness(t, false)
	h.bot.onInteraction(nil, command(CommandAdd, query))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).NotConfigured {
		t.Fatalf("unexpected response %q", content)
	}

	h = newHarness(t, true)
	h.bot.nodeReady.Store(true)
	h.bot.onInteraction(nil, command(CommandAdd, query))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).UserNotInVoice {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestHistory(t *testing.T) {
	for name, e := range map[string]*discordgo.InteractionCreate{"button": button(ui.IDHistory), "command": command(CommandHistory)} {
		h := newHarness(t, true)
		_ = h.store.AddHistory(context.Background(), testGuild, storage.HistoryEntry{Author: "Artist", Title: "Song", URI: "https://example.com/song"})
		h.bot.onInteraction(nil, e)
		body := callback(t, h.discord.take())
		data := body["data"].(map[string]any)
		want := "### " + i18n.Get(i18n.English).HistoryTitle + "\n**1.** [Song](https://example.com/song)\n-# Artist"
		if got := texts(data["components"]); got != want {
			t.Fatalf("%s: unexpected history %q", name, got)
		}
		if int(data["flags"].(float64)) != int(discordgo.MessageFlagsEphemeral|discordgo.MessageFlagsIsComponentsV2) {
			t.Fatalf("%s: history must be an ephemeral components v2 message", name)
		}
	}
}

func TestSelectExpiredSearch(t *testing.T) {
	h := newHarness(t, true)
	h.bot.onInteraction(nil, interaction(discordgo.InteractionMessageComponent,
		discordgo.MessageComponentInteractionData{CustomID: ui.SelectID("missing"), Values: []string{"0"}}, nil))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).SearchExpired {
		t.Fatalf("unexpected response %q", content)
	}
}

func TestSelectRejectsInvalidChoice(t *testing.T) {
	h := newHarness(t, true)
	id := h.bot.searches.Put(pendingSearch{interaction: button(ui.IDAdd).Interaction, tracks: []music.Track{{Title: "Song"}}})
	h.bot.onInteraction(nil, interaction(discordgo.InteractionMessageComponent,
		discordgo.MessageComponentInteractionData{CustomID: ui.SelectID(id), Values: []string{"7"}}, nil))
	kind, content, _ := callbackContent(t, h.discord.take())
	if kind != int(discordgo.InteractionResponseUpdateMessage) || content != i18n.Get(i18n.English).GenericError {
		t.Fatalf("unexpected response %d %q", kind, content)
	}
}

func TestSelectChecksVoiceState(t *testing.T) {
	h := newHarness(t, true)
	id := h.bot.searches.Put(pendingSearch{interaction: button(ui.IDAdd).Interaction, tracks: []music.Track{{Title: "Song"}}})
	h.bot.onInteraction(nil, interaction(discordgo.InteractionMessageComponent,
		discordgo.MessageComponentInteractionData{CustomID: ui.SelectID(id), Values: []string{"0"}}, nil))
	if _, content, _ := callbackContent(t, h.discord.take()); content != i18n.Get(i18n.English).UserNotInVoice {
		t.Fatalf("unexpected response %q", content)
	}
	if h.bot.searches.Len() != 0 {
		t.Fatal("used search must be removed")
	}
}

func TestInteractionsOutsideGuildAreIgnored(t *testing.T) {
	h := newHarness(t, true)
	e := button(ui.IDAdd)
	e.GuildID = ""
	h.bot.onInteraction(nil, e)
	if len(h.discord.take()) != 0 {
		t.Fatal("direct message interactions must be ignored")
	}
}

func TestGuildDeleteRemovesConfiguration(t *testing.T) {
	h := newHarness(t, true)
	h.bot.players.Get(testGuild).Enqueue(music.Track{Title: "Song"})
	h.bot.onGuildDelete(nil, &discordgo.GuildDelete{Guild: &discordgo.Guild{ID: testGuild, Unavailable: true}})
	if _, ok := h.bot.guilds.Get(testGuild); !ok {
		t.Fatal("outage must not remove the configuration")
	}
	h.bot.onGuildDelete(nil, &discordgo.GuildDelete{Guild: &discordgo.Guild{ID: testGuild}})
	if _, ok := h.store.guilds[testGuild]; ok {
		t.Fatal("configuration must be deleted when the bot is removed")
	}
	if h.bot.players.Get(testGuild).Active() {
		t.Fatal("player state must be dropped")
	}
}

func TestMessagesForUsesUserLocale(t *testing.T) {
	h := newHarness(t, true)
	g, _ := h.bot.guilds.Get(testGuild)
	g.Locale = i18n.Russian
	h.store.guilds[testGuild] = g
	if _, err := h.bot.guilds.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	e := button(ui.IDAdd)
	for _, c := range []struct {
		user discordgo.Locale
		want i18n.Locale
	}{
		{user: discordgo.EnglishUS, want: i18n.English},
		{user: discordgo.EnglishGB, want: i18n.English},
		{user: discordgo.Russian, want: i18n.Russian},
		{user: discordgo.German, want: i18n.English},
		{user: "", want: i18n.English},
	} {
		e.Interaction.Locale = c.user
		if h.bot.messagesFor(e.Interaction) != i18n.Get(c.want) {
			t.Errorf("user %v: want %s", c.user, c.want)
		}
	}
}

func TestSetupDefaultsToServerLocale(t *testing.T) {
	h := newHarness(t, true)
	g, _ := h.bot.guilds.Get(testGuild)
	g.Locale = i18n.Russian
	h.store.guilds[testGuild] = g
	if _, err := h.bot.guilds.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	for _, c := range []struct {
		server *discordgo.Locale
		want   i18n.Locale
	}{
		{server: nil, want: i18n.English},
		{server: ptr(discordgo.German), want: i18n.English},
		{server: ptr(discordgo.EnglishGB), want: i18n.English},
		{server: ptr(discordgo.Russian), want: i18n.Russian},
	} {
		e := interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{Name: CommandSetup}, admin)
		e.GuildLocale = c.server
		h.bot.onInteraction(nil, e)
		h.discord.take()
		if stored := h.store.guilds[testGuild]; stored.Locale != c.want {
			t.Errorf("setup without a language on server %v must use %s, got %q", c.server, c.want, stored.Locale)
		}
	}

	e := interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{
		Name:    CommandSetup,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{{Name: OptionLanguage, Type: discordgo.ApplicationCommandOptionString, Value: "en"}},
	}, admin)
	e.GuildLocale = ptr(discordgo.Russian)
	h.bot.onInteraction(nil, e)
	h.discord.take()
	if stored := h.store.guilds[testGuild]; stored.Locale != i18n.English {
		t.Fatalf("explicit language must override the server locale, got %q", stored.Locale)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSetupRejectsUnknownLanguage(t *testing.T) {
	h := newHarness(t, false)
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	h.bot.onInteraction(nil, interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{
		Name: CommandSetup,
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: OptionLanguage, Type: discordgo.ApplicationCommandOptionString, Value: "de"},
		},
	}, admin))
	requests := h.discord.take()
	if _, content, _ := callbackContent(t, requests); content != i18n.Get(i18n.English).SetupUnknownLanguage || len(requests) != 1 {
		t.Fatalf("unexpected response %q after %d requests", content, len(requests))
	}
	if _, ok := h.store.guilds[testGuild]; ok {
		t.Fatal("guild must not be configured with an unknown language")
	}
}

func TestSetupSlowMode(t *testing.T) {
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	setup := func(h *harness, options ...*discordgo.ApplicationCommandInteractionDataOption) storage.Guild {
		h.bot.onInteraction(nil, interaction(discordgo.InteractionApplicationCommand,
			discordgo.ApplicationCommandInteractionData{Name: CommandSetup, Options: options}, admin))
		h.discord.take()
		return h.store.guilds[testGuild]
	}
	slowMode := func(seconds int) *discordgo.ApplicationCommandInteractionDataOption {
		return &discordgo.ApplicationCommandInteractionDataOption{Name: OptionSlowMode, Type: discordgo.ApplicationCommandOptionInteger, Value: float64(seconds)}
	}

	h := newHarness(t, false)
	if g := setup(h); g.SlowMode != time.Minute {
		t.Fatalf("new channel must default to one minute, got %v", g.SlowMode)
	}
	if g := setup(h, slowMode(300)); g.SlowMode != 5*time.Minute {
		t.Fatalf("explicit slow mode must be stored, got %v", g.SlowMode)
	}
	if g := setup(h); g.SlowMode != 5*time.Minute {
		t.Fatalf("setup without the option must keep the current slow mode, got %v", g.SlowMode)
	}
	if g := setup(h, slowMode(45)); g.SlowMode != 45*time.Second {
		t.Fatalf("any number of seconds must be accepted, got %v", g.SlowMode)
	}
	if g := setup(h, slowMode(100000)); g.SlowMode != MaxSlowMode {
		t.Fatalf("slow mode must be capped, got %v", g.SlowMode)
	}
	if g := setup(h, slowMode(0)); g.SlowMode != 0 {
		t.Fatalf("slow mode must be possible to turn off, got %v", g.SlowMode)
	}
}

func TestCommandDescriptionsFitDiscordLimits(t *testing.T) {
	for _, c := range Commands() {
		descriptions := append([]string{c.Description}, slices.Collect(maps.Values(*c.DescriptionLocalizations))...)
		for _, o := range c.Options {
			descriptions = append(descriptions, o.Description)
			descriptions = append(descriptions, slices.Collect(maps.Values(o.DescriptionLocalizations))...)
			for _, ch := range o.Choices {
				descriptions = append(descriptions, ch.Name)
			}
		}
		for _, d := range descriptions {
			if n := utf8.RuneCountInString(d); n == 0 || n > 100 {
				t.Errorf("/%s: description %q has %d characters", c.Name, d, n)
			}
		}
	}
	setup := Commands()[0]
	option := setup.Options[1]
	if option.Name != OptionSlowMode || option.Type != discordgo.ApplicationCommandOptionInteger || option.Required || len(option.Choices) != 0 {
		t.Fatalf("setup must have an optional slow mode option, got %+v", option)
	}
	if option.MinValue == nil || *option.MinValue != 0 || option.MaxValue != 21600 {
		t.Fatalf("slow mode must accept 0 to 21600 seconds like Discord, got %v..%v", option.MinValue, option.MaxValue)
	}
}

func TestSetupChanges(t *testing.T) {
	ru := i18n.Get(i18n.Russian)
	previous := storage.Guild{GuildID: testGuild, ChannelID: "200", SlowMode: time.Minute}
	if got := SetupChanges(ru, previous, previous); got != ru.SetupUnchanged {
		t.Fatalf("unchanged setup must say so, got %q", got)
	}
	current := storage.Guild{GuildID: testGuild, ChannelID: "201", Locale: i18n.Russian}
	want := ru.SetupUpdated +
		"\n- Канал: <#200> → <#201>" +
		"\n- Язык: English → Русский" +
		"\n- Медленный режим, секунд: 60 → выключен"
	if got := SetupChanges(ru, previous, current); got != want {
		t.Fatalf("unexpected changes:\n%s\nwant:\n%s", got, want)
	}
	if got := SetupChanges(ru, storage.Guild{Locale: i18n.English}, storage.Guild{SlowMode: 45 * time.Second}); got != ru.SetupUpdated+"\n- Медленный режим, секунд: выключен → 45" {
		t.Fatalf("legacy locale must equal the default, got %q", got)
	}
}

func TestFirstSetupConfirmsInstallation(t *testing.T) {
	h := newHarness(t, false)
	admin := &discordgo.Member{User: &discordgo.User{ID: testUser}, Permissions: discordgo.PermissionAdministrator}
	h.bot.onInteraction(nil, interaction(discordgo.InteractionApplicationCommand, discordgo.ApplicationCommandInteractionData{Name: CommandSetup}, admin))
	requests := h.discord.take()
	if done := requests[len(requests)-2].Body; done["content"] != i18n.Get(i18n.English).SetupDone {
		t.Fatalf("first setup must confirm the installation, got %v", done)
	}
}
