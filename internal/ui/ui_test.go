package ui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/storage"
)

var en = i18n.Get(i18n.English)
var ru = i18n.Get(i18n.Russian)

func sampleTrack(n int) music.Track {
	return music.Track{
		Encoded:    fmt.Sprintf("enc%d", n),
		Identifier: fmt.Sprintf("id%d", n),
		Title:      fmt.Sprintf("Song %d", n),
		Author:     fmt.Sprintf("Artist %d", n),
		URI:        fmt.Sprintf("https://www.youtube.com/watch?v=id%d", n),
		Source:     "youtube",
		Duration:   3*time.Minute + time.Duration(n)*time.Second,
		Requester:  "alice",
	}
}

func longTrack(n int) music.Track {
	t := sampleTrack(n)
	t.Title = strings.Repeat("Очень длинное название ", 30)
	t.Author = strings.Repeat("Исполнитель ", 40)
	t.URI = "https://www.youtube.com/watch?v=" + strings.Repeat("x", 200)
	t.Requester = strings.Repeat("Пользователь", 10)
	return t
}

func card(t *testing.T, components []discordgo.MessageComponent) discordgo.Container {
	t.Helper()
	if len(components) != 1 {
		t.Fatalf("message must be a single card, got %d components", len(components))
	}
	c, ok := components[0].(discordgo.Container)
	if !ok || c.AccentColor == nil || *c.AccentColor != Color {
		t.Fatalf("message must be an accented container, got %#v", components[0])
	}
	return c
}

func walk(components []discordgo.MessageComponent, visit func(discordgo.MessageComponent)) {
	for _, c := range components {
		visit(c)
		switch v := c.(type) {
		case discordgo.Container:
			walk(v.Components, visit)
		case discordgo.Section:
			walk(v.Components, visit)
			visit(v.Accessory)
		case discordgo.ActionsRow:
			walk(v.Components, visit)
		}
	}
}

func texts(components []discordgo.MessageComponent) string {
	var parts []string
	walk(components, func(c discordgo.MessageComponent) {
		if v, ok := c.(discordgo.TextDisplay); ok {
			parts = append(parts, v.Content)
		}
	})
	return strings.Join(parts, "\n")
}

func thumbnail(components []discordgo.MessageComponent) string {
	url := ""
	walk(components, func(c discordgo.MessageComponent) {
		if v, ok := c.(discordgo.Thumbnail); ok {
			url = v.Media.URL
		}
	})
	return url
}

func TestSelectID(t *testing.T) {
	id, ok := ParseSelectID(SelectID("abc123"))
	if !ok || id != "abc123" {
		t.Fatalf("ParseSelectID(SelectID()) = %q, %v", id, ok)
	}
	for _, bad := range []string{"btn_add", "track_select:", ""} {
		if _, ok := ParseSelectID(bad); ok {
			t.Errorf("ParseSelectID(%q) must fail", bad)
		}
	}
}

func TestTrackDuration(t *testing.T) {
	if got := TrackDuration(en, music.Track{IsStream: true, Duration: time.Hour}); got != en.DurationLive {
		t.Errorf("stream duration = %q", got)
	}
	if got := TrackDuration(ru, music.Track{}); got != ru.DurationUnknown {
		t.Errorf("unknown duration = %q", got)
	}
	if got := TrackDuration(en, music.Track{Duration: 61 * time.Second}); got != "01:01" {
		t.Errorf("duration = %q", got)
	}
}

func isDivider(c discordgo.MessageComponent) bool {
	s, ok := c.(discordgo.Separator)
	return ok && s.Divider != nil && *s.Divider
}

func describe(c discordgo.MessageComponent) string {
	switch v := c.(type) {
	case discordgo.TextDisplay:
		return "text " + v.Content
	case discordgo.Separator:
		if isDivider(v) {
			return "divider"
		}
		return "gap"
	case discordgo.Section:
		if b, ok := v.Accessory.(IconButton); ok {
			return b.Emoji + " " + b.CustomID + " | " + texts(v.Components)
		}
		return "section"
	}
	return fmt.Sprintf("%T", c)
}

func layout(c discordgo.Container) []string {
	result := make([]string, len(c.Components))
	for i, component := range c.Components {
		result[i] = describe(component)
	}
	return result
}

func playing(upcoming []music.Track, total int, length time.Duration) Player {
	current := sampleTrack(0)
	return Player{Current: &current, Upcoming: upcoming, Total: total, Length: length}
}

func TestPlayerIdle(t *testing.T) {
	c := card(t, PlayerMessage(ru, Player{}))
	want := []string{
		"text ### " + ru.IdleTitle + "\n-# " + ru.IdleHint,
		"divider",
		"💬 " + IDAdd + " | " + ru.ActionAdd,
		"gap",
		"gap",
		"🔖 " + IDHistory + " | " + ru.ActionHistory,
	}
	if got := layout(c); !slices.Equal(got, want) {
		t.Fatalf("unexpected idle layout:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if thumbnail(c.Components) != "" {
		t.Fatal("idle player must not show a cover")
	}
}

func TestPlayerNowPlaying(t *testing.T) {
	track := sampleTrack(1)
	c := card(t, PlayerMessage(en, Player{Current: &track, Skip: Votes{Needed: 3}, Disconnect: Votes{Count: 1, Needed: 1}}))
	want := []string{
		"section",
		"divider",
		"💬 " + IDAdd + " | " + en.ActionAdd,
		"⏭️ " + IDSkip + " | " + en.ActionSkip,
		"⏹️ " + IDDisconnect + " | " + en.ActionDisconnect,
		"gap",
		"gap",
		"🔖 " + IDHistory + " | " + en.ActionHistory,
		"divider",
		"text ### " + en.QueueTitle + "\n-# " + en.QueueEmpty,
	}
	if got := layout(c); !slices.Equal(got, want) {
		t.Fatalf("unexpected layout:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := texts(c.Components[:1]); got != "-# Now playing\n### [Song 1](https://www.youtube.com/watch?v=id1)\nArtist 1\n-# 03:01 · YouTube · added by alice" {
		t.Fatalf("unexpected track details: %q", got)
	}
	if got := thumbnail(c.Components); got != "https://i.ytimg.com/vi/id1/mqdefault.jpg" {
		t.Fatalf("unexpected cover %q", got)
	}
}

func TestPlayerShowsStartedVotes(t *testing.T) {
	track := sampleTrack(1)
	c := card(t, PlayerMessage(ru, Player{Current: &track, Skip: Votes{Count: 1, Needed: 3}, Disconnect: Votes{Count: 2, Needed: 2}}))
	if got := describe(c.Components[3]); got != "⏭️ "+IDSkip+" | "+ru.ActionSkip+"\n-# "+fmt.Sprintf(ru.VotesCast, 1, 3) {
		t.Fatalf("started vote must be shown under the action, got %q", got)
	}
	if got := describe(c.Components[4]); got != "⏹️ "+IDDisconnect+" | "+ru.ActionDisconnect+"\n-# "+fmt.Sprintf(ru.VotesCast, 2, 2) {
		t.Fatalf("unexpected disconnect vote: %q", got)
	}
}

func TestIconButtonJSON(t *testing.T) {
	data, err := json.Marshal(IconButton{CustomID: IDSkip, Emoji: "⏭️"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `{"type":2,"custom_id":"btn_skip","style":2,"emoji":{"name":"⏭️"}}` {
		t.Fatalf("icon button must be grey and carry only an emoji, got %s", got)
	}
}

func TestPlayerWithoutCoverOrLinks(t *testing.T) {
	msg := PlayerMessage(en, Player{Current: &music.Track{Title: "Radio", IsStream: true, Source: "http"}})
	c := card(t, msg)
	if _, ok := c.Components[0].(discordgo.TextDisplay); !ok || thumbnail(msg) != "" {
		t.Fatalf("player without a cover must use plain text, got %#v", c.Components[0])
	}
	if got := texts(c.Components[:2]); got != "-# Now playing\n### Radio\n-# Live · HTTP" {
		t.Fatalf("unexpected player text: %q", got)
	}
}

func TestPlayerShowsSlowMode(t *testing.T) {
	for _, p := range []Player{{}, playing(nil, 0, 0)} {
		p.SlowMode = time.Minute
		c := card(t, PlayerMessage(ru, p))
		last := c.Components[len(c.Components)-1]
		if describe(last) != "text -# "+ru.SlowModeHint || !isDivider(c.Components[len(c.Components)-2]) {
			t.Fatalf("slow mode must close the card after a divider, got %v", layout(c))
		}
		walk(c.Components, func(component discordgo.MessageComponent) {
			if s, ok := component.(discordgo.Separator); ok && isDivider(s) && (s.Spacing == nil || *s.Spacing != discordgo.SeparatorSpacingSizeLarge) {
				t.Fatalf("dividers must use large spacing, got %#v", s)
			}
		})
	}
	if got := texts(PlayerMessage(en, Player{})); strings.Contains(got, "🐢") {
		t.Fatalf("player must not mention a disabled slow mode, got %q", got)
	}
}

func TestQueueWithTracks(t *testing.T) {
	msg := PlayerMessage(en, playing([]music.Track{sampleTrack(1), sampleTrack(2)}, 2, 6*time.Minute+3*time.Second))
	want := "### Up next\n-# Tracks: 2 · 06:03\n" +
		"**1.** [Song 1](https://www.youtube.com/watch?v=id1)\n-# Artist 1 · 03:01\n\n" +
		"**2.** [Song 2](https://www.youtube.com/watch?v=id2)\n-# Artist 2 · 03:02"
	if got := texts(msg); !strings.HasSuffix(got, "\n"+want) {
		t.Fatalf("unexpected queue:\n%s\nwant:\n%s", got, want)
	}
	if got := texts(PlayerMessage(en, playing([]music.Track{{Title: "Radio", IsStream: true}}, 1, 0))); !strings.Contains(got, "### Up next\n-# Tracks: 1\n") {
		t.Fatalf("queue of streams must not show a length, got %q", got)
	}
}

func TestQueueOverflow(t *testing.T) {
	var upcoming []music.Track
	for i := range QueueLimit + 5 {
		upcoming = append(upcoming, sampleTrack(i))
	}
	got := texts(PlayerMessage(ru, playing(upcoming, 40, time.Hour)))
	if strings.Count(got, "**") != 2*QueueLimit || !strings.Contains(got, "-# "+fmt.Sprintf(ru.QueueOverflow, 40-QueueLimit)) {
		t.Fatalf("queue must show %d tracks and the rest as a counter, got:\n%s", QueueLimit, got)
	}
}

func TestHistoryMessage(t *testing.T) {
	if got := texts(HistoryMessage(en, nil)); got != "### "+en.HistoryTitle+"\n-# "+en.HistoryEmpty {
		t.Fatalf("unexpected empty history: %q", got)
	}
	msg := HistoryMessage(en, []storage.HistoryEntry{{Author: "A", Title: "T", URI: "https://x.y/z"}})
	card(t, msg)
	if got := texts(msg); got != "### "+en.HistoryTitle+"\n**1.** [T](https://x.y/z)\n-# A" {
		t.Fatalf("unexpected history: %q", got)
	}
}

func TestSearchMessage(t *testing.T) {
	tracks := []music.Track{sampleTrack(1), sampleTrack(2), sampleTrack(3), sampleTrack(4), sampleTrack(5), sampleTrack(6)}
	msg := SearchMessage(en, "sid", tracks)
	got := texts(msg)
	if !strings.HasPrefix(got, "### "+en.SearchTitle+"\n-# YouTube\n**1.** [Song 1]") || strings.Count(got, "**") != 2*SearchLimit {
		t.Fatalf("unexpected search results:\n%s", got)
	}
	var menu discordgo.SelectMenu
	walk(msg, func(c discordgo.MessageComponent) {
		if v, ok := c.(discordgo.SelectMenu); ok {
			menu = v
		}
	})
	if menu.CustomID != "track_select:sid" || len(menu.Options) != SearchLimit || menu.Options[4].Value != "4" {
		t.Fatalf("unexpected select menu: %+v", menu)
	}
	if menu.Options[0].Label != "1. Song 1" || menu.Options[0].Description != "Artist 1 · 03:01" {
		t.Fatalf("unexpected option: %+v", menu.Options[0])
	}
	walk(SearchMessage(en, "sid", []music.Track{longTrack(1)}), func(c discordgo.MessageComponent) {
		if v, ok := c.(discordgo.SelectMenu); ok {
			if o := v.Options[0]; utf8.RuneCountInString(o.Label) > 100 || utf8.RuneCountInString(o.Description) > 100 {
				t.Fatalf("select option is too long: %d/%d", utf8.RuneCountInString(o.Label), utf8.RuneCountInString(o.Description))
			}
		}
	})
}

func TestMessagesRespectDiscordLimits(t *testing.T) {
	var queue []music.Track
	var history []storage.HistoryEntry
	for i := range 30 {
		lt := longTrack(i)
		queue = append(queue, lt)
		history = append(history, storage.HistoryEntry{Author: lt.Author, Title: lt.Title, URI: lt.URI})
	}
	current := longTrack(99)
	messages := map[string][]discordgo.MessageComponent{
		"history": HistoryMessage(ru, history),
		"search":  SearchMessage(ru, "sid", queue),
		"player":  PlayerMessage(ru, Player{Current: &current, Skip: Votes{Count: 10, Needed: 20}, SlowMode: 5 * time.Minute, Upcoming: queue, Total: 1000, Length: time.Hour}),
	}
	for name, msg := range messages {
		if n := TextLength(msg); n > MaxText {
			t.Errorf("%s text length %d exceeds %d", name, n, MaxText)
		}
		count := 0
		walk(msg, func(discordgo.MessageComponent) { count++ })
		if count > 40 {
			t.Errorf("%s has %d components", name, count)
		}
		walk(msg, func(c discordgo.MessageComponent) {
			if b, ok := c.(discordgo.Button); ok && utf8.RuneCountInString(b.Label) > 80 {
				t.Errorf("%s button label is too long", name)
			}
		})
		if _, err := json.Marshal(msg); err != nil {
			t.Errorf("marshal %s: %v", name, err)
		}
	}
}

func TestAddModalAndValueExtraction(t *testing.T) {
	modal := AddModal(ru, "")
	if modal.CustomID != IDAddModal || modal.Title != ru.ModalTitle || len(modal.Components) != 2 {
		t.Fatalf("unexpected modal: %+v", modal)
	}
	queryLabel := modal.Components[0].(discordgo.Label)
	input := queryLabel.Component.(discordgo.TextInput)
	if queryLabel.Label != ru.ModalQueryLabel || input.CustomID != IDQuery || input.Required == nil || !*input.Required {
		t.Fatalf("unexpected text input: %+v %+v", queryLabel, input)
	}
	sourceLabel := modal.Components[1].(discordgo.Label)
	menu := sourceLabel.Component.(discordgo.SelectMenu)
	if sourceLabel.Label != ru.ModalSourceLabel || menu.CustomID != IDSource || menu.Type() != discordgo.SelectMenuComponent || len(menu.Options) != 2 {
		t.Fatalf("unexpected source select: %+v %+v", sourceLabel, menu)
	}
	if !menu.Options[0].Default || menu.Options[1].Default || menu.Options[1].Label != "SoundCloud" {
		t.Fatalf("youtube must be selected by default: %+v", menu.Options)
	}
	soundcloud := AddModal(ru, SourceSoundCloud).Components[1].(discordgo.Label).Component.(discordgo.SelectMenu)
	if soundcloud.Options[0].Default || !soundcloud.Options[1].Default {
		t.Fatalf("requested source must be preselected: %+v", soundcloud.Options)
	}
	if _, err := json.Marshal(modal); err != nil {
		t.Fatalf("marshal modal: %v", err)
	}

	payload := `{"type":5,"data":{"custom_id":"modal_add","components":[` +
		`{"type":18,"id":1,"component":{"type":4,"id":2,"custom_id":"query","value":"  never gonna give you up  "}},` +
		`{"type":18,"id":3,"component":{"type":3,"id":4,"custom_id":"source","values":["soundcloud"]}}]}}`
	var interaction discordgo.Interaction
	if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
		t.Fatalf("unmarshal interaction: %v", err)
	}
	data := interaction.ModalSubmitData()
	if got := ModalValue(data.Components, IDQuery); got != "never gonna give you up" {
		t.Fatalf("ModalValue() = %q", got)
	}
	if got := ModalValue(data.Components, IDSource); got != SourceSoundCloud {
		t.Fatalf("ModalValue() for source = %q", got)
	}
	if got := ModalValue(data.Components, "missing"); got != "" {
		t.Fatalf("ModalValue() for unknown id = %q", got)
	}
}
func TestSearchSource(t *testing.T) {
	tests := map[string]string{"": SourceYouTube, "youtube": SourceYouTube, "SoundCloud": SourceSoundCloud, "deezer": SourceYouTube}
	for in, want := range tests {
		if got := SearchSource(in); got != want {
			t.Errorf("SearchSource(%q) = %q, want %q", in, got, want)
		}
	}
}
func TestLinkEscapesAndSkipsInvalidURLs(t *testing.T) {
	if got := link("[Live] Song", "https://example.com/a", 200); got != `[\[Live\] Song](https://example.com/a)` {
		t.Errorf("unexpected link: %q", got)
	}
	if got := link("Song", "not a url", 200); got != "Song" {
		t.Errorf("invalid url must render plain title, got %q", got)
	}
	if got := link("Song title", "https://example.com/a/very/long/path", 20); got != "Song title" {
		t.Errorf("link that does not fit must fall back to the plain title, got %q", got)
	}
	if got := link("Song", "https://example.com/"+strings.Repeat("a", maxLinkURL), 1000); got != "Song" {
		t.Errorf("overlong url must be dropped, got %q", got)
	}
}
func TestEscapeFit(t *testing.T) {
	tests := []struct {
		text  string
		limit int
	}{
		{"plain text", 5},
		{strings.Repeat("*", 50), 10},
		{strings.Repeat("_a", 40), 11},
		{"short", 100},
	}
	for _, tt := range tests {
		got := escapeFit(tt.text, tt.limit)
		if n := utf8.RuneCountInString(got); n > tt.limit || n == 0 {
			t.Errorf("escapeFit(%q, %d) = %q (%d runes)", tt.text, tt.limit, got, n)
		}
	}
	if escapeFit("anything", 0) != "" {
		t.Error("zero limit must produce an empty string")
	}
}
