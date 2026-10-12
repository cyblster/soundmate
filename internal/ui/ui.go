package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/format"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/storage"
)

const (
	Color = 0xED4245

	QueueLimit   = 10
	HistoryLimit = storage.HistorySize
	SearchLimit  = 5

	IDAdd        = "btn_add"
	IDSkip       = "btn_skip"
	IDDisconnect = "btn_disconnect"
	IDHistory    = "btn_history"
	IDAddModal   = "modal_add"
	IDQuery      = "query"
	IDSource     = "source"
	selectPrefix = "track_select:"

	MaxText       = 4000
	textReserve   = 400
	playerReserve = 1000
	maxTitle      = 200
	maxAuthor     = 80
	maxRequester  = 64
	maxLinkURL    = 120
	maxSelectText = 100
	MaxQuery      = 300

	SourceYouTube    = "youtube"
	SourceSoundCloud = "soundcloud"

	metaSeparator = " · "
)

var SearchSources = []string{SourceYouTube, SourceSoundCloud}

type Votes struct {
	Count  int
	Needed int
}

type Player struct {
	Current    *music.Track
	Skip       Votes
	Disconnect Votes
	SlowMode   time.Duration
	Upcoming   []music.Track
	Total      int
	Length     time.Duration
}

func SelectID(searchID string) string {
	return selectPrefix + searchID
}

func ParseSelectID(customID string) (string, bool) {
	id, ok := strings.CutPrefix(customID, selectPrefix)
	return id, ok && id != ""
}

func TrackDuration(m *i18n.Messages, t music.Track) string {
	switch {
	case t.IsStream:
		return m.DurationLive
	case t.Duration <= 0:
		return m.DurationUnknown
	default:
		return format.Duration(t.Duration)
	}
}

func escapeFit(text string, limit int) string {
	for n := limit; n > 0; {
		escaped := format.EscapeMarkdown(format.Truncate(text, n))
		excess := utf8.RuneCountInString(escaped) - limit
		if excess <= 0 {
			return escaped
		}
		n -= excess
	}
	return ""
}

func link(title, uri string, limit int) string {
	if uri != "" && len(uri) <= maxLinkURL && format.IsURL(uri) {
		if room := limit - len(uri) - 4; room > 0 {
			return "[" + escapeFit(title, room) + "](" + uri + ")"
		}
	}
	return escapeFit(title, limit)
}

func container(components ...discordgo.MessageComponent) []discordgo.MessageComponent {
	color := Color
	return []discordgo.MessageComponent{discordgo.Container{AccentColor: &color, Components: components}}
}

func text(content string) discordgo.TextDisplay {
	return discordgo.TextDisplay{Content: content}
}

func subtext(content string) string {
	return "-# " + content
}

func divider() discordgo.Separator {
	visible := true
	spacing := discordgo.SeparatorSpacingSizeLarge
	return discordgo.Separator{Divider: &visible, Spacing: &spacing}
}

func joinMeta(parts ...string) string {
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, metaSeparator)
}

func entry(n int, title, uri, meta string, limit int) string {
	prefix := fmt.Sprintf("**%d.** ", n)
	if meta != "" {
		meta = "\n" + subtext(meta)
	}
	room := limit - utf8.RuneCountInString(prefix) - utf8.RuneCountInString(meta)
	return prefix + link(title, uri, room) + meta
}

func entries(lines []string) discordgo.TextDisplay {
	return text(strings.Join(lines, "\n\n"))
}

func entryLimit(count int) int {
	return (MaxText - textReserve) / max(count, 1)
}

func trackMeta(m *i18n.Messages, t music.Track) string {
	return joinMeta(escapeFit(t.Author, maxAuthor), TrackDuration(m, t))
}

func trackEntries(m *i18n.Messages, tracks []music.Track, limit int) discordgo.TextDisplay {
	lines := make([]string, len(tracks))
	for i, t := range tracks {
		lines[i] = entry(i+1, t.Title, t.URI, trackMeta(m, t), limit)
	}
	return entries(lines)
}

type IconButton struct {
	CustomID string
	Emoji    string
}

func (IconButton) Type() discordgo.ComponentType {
	return discordgo.ButtonComponent
}

func (b IconButton) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     discordgo.ComponentType  `json:"type"`
		CustomID string                   `json:"custom_id"`
		Style    discordgo.ButtonStyle    `json:"style"`
		Emoji    discordgo.ComponentEmoji `json:"emoji"`
	}{b.Type(), b.CustomID, discordgo.SecondaryButton, discordgo.ComponentEmoji{Name: b.Emoji}})
}

func PlayerMessage(m *i18n.Messages, p Player) []discordgo.MessageComponent {
	var body []discordgo.MessageComponent
	if p.Current == nil {
		body = []discordgo.MessageComponent{text("### " + m.IdleTitle + "\n" + subtext(m.IdleHint))}
	} else {
		body = nowPlaying(m, *p.Current)
	}
	body = append(append(body, divider()), controls(m, p)...)
	if p.Current != nil {
		body = append(append(body, divider()), queue(m, p)...)
	}
	if p.SlowMode > 0 {
		body = append(body, divider(), text(subtext(m.SlowModeHint)))
	}
	return container(body...)
}

func gap() discordgo.Separator {
	hidden := false
	spacing := discordgo.SeparatorSpacingSizeSmall
	return discordgo.Separator{Divider: &hidden, Spacing: &spacing}
}

func action(description string, button IconButton) discordgo.Section {
	return discordgo.Section{Components: []discordgo.MessageComponent{text(description)}, Accessory: button}
}

func votedAction(m *i18n.Messages, description string, v Votes, button IconButton) discordgo.Section {
	if v.Count > 0 && v.Needed > 1 {
		description += "\n" + subtext(fmt.Sprintf(m.VotesCast, v.Count, v.Needed))
	}
	return action(description, button)
}

func controls(m *i18n.Messages, p Player) []discordgo.MessageComponent {
	add := action(m.ActionAdd, IconButton{CustomID: IDAdd, Emoji: "💬"})
	history := action(m.ActionHistory, IconButton{CustomID: IDHistory, Emoji: "🔖"})
	if p.Current == nil {
		return []discordgo.MessageComponent{add, gap(), gap(), history}
	}
	return []discordgo.MessageComponent{
		add,
		votedAction(m, m.ActionSkip, p.Skip, IconButton{CustomID: IDSkip, Emoji: "⏭️"}),
		votedAction(m, m.ActionDisconnect, p.Disconnect, IconButton{CustomID: IDDisconnect, Emoji: "⏹️"}),
		gap(),
		gap(),
		history,
	}
}

func nowPlaying(m *i18n.Messages, t music.Track) []discordgo.MessageComponent {
	header := subtext(m.NowPlaying) + "\n### " + link(t.Title, t.URI, maxTitle)
	if t.Author != "" {
		header += "\n" + escapeFit(t.Author, maxAuthor)
	}
	var requester string
	if t.Requester != "" {
		requester = fmt.Sprintf(m.RequestedBy, escapeFit(t.Requester, maxRequester))
	}
	details := []discordgo.MessageComponent{
		text(header),
		text(subtext(joinMeta(TrackDuration(m, t), format.SourceName(t.Source), requester))),
	}
	thumb := format.Thumbnail(t.Source, t.Identifier, t.ArtworkURL)
	if !format.IsURL(thumb) {
		return details
	}
	return []discordgo.MessageComponent{discordgo.Section{
		Components: details,
		Accessory:  discordgo.Thumbnail{Media: discordgo.UnfurledMediaItem{URL: thumb}},
	}}
}

func queue(m *i18n.Messages, p Player) []discordgo.MessageComponent {
	shown := p.Upcoming[:min(len(p.Upcoming), QueueLimit, max(p.Total, 0))]
	if len(shown) == 0 {
		return []discordgo.MessageComponent{text("### " + m.QueueTitle + "\n" + subtext(m.QueueEmpty))}
	}
	summary := fmt.Sprintf(m.QueueCount, p.Total)
	if p.Length > 0 {
		summary = joinMeta(summary, format.Duration(p.Length))
	}
	body := []discordgo.MessageComponent{
		text("### " + m.QueueTitle + "\n" + subtext(summary)),
		trackEntries(m, shown, (MaxText-playerReserve)/len(shown)),
	}
	if rest := p.Total - len(shown); rest > 0 {
		body = append(body, text(subtext(fmt.Sprintf(m.QueueOverflow, rest))))
	}
	return body
}

func HistoryMessage(m *i18n.Messages, history []storage.HistoryEntry) []discordgo.MessageComponent {
	shown := history[:min(len(history), HistoryLimit)]
	if len(shown) == 0 {
		return container(text("### " + m.HistoryTitle + "\n" + subtext(m.HistoryEmpty)))
	}
	limit := entryLimit(len(shown))
	lines := make([]string, len(shown))
	for i, e := range shown {
		lines[i] = entry(i+1, e.Title, e.URI, escapeFit(e.Author, maxAuthor), limit)
	}
	return container(text("### "+m.HistoryTitle), divider(), entries(lines))
}

func SearchMessage(m *i18n.Messages, searchID string, tracks []music.Track) []discordgo.MessageComponent {
	shown := tracks[:min(len(tracks), SearchLimit)]
	header := "### " + m.SearchTitle
	if len(shown) > 0 {
		if source := format.SourceName(shown[0].Source); source != "" {
			header += "\n" + subtext(source)
		}
	}
	options := make([]discordgo.SelectMenuOption, len(shown))
	for i, t := range shown {
		options[i] = discordgo.SelectMenuOption{
			Label:       format.Truncate(fmt.Sprintf("%d. %s", i+1, t.Title), maxSelectText),
			Description: format.Truncate(joinMeta(t.Author, TrackDuration(m, t)), maxSelectText),
			Value:       strconv.Itoa(i),
		}
	}
	return container(
		text(header),
		divider(),
		trackEntries(m, shown, entryLimit(len(shown))),
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{CustomID: SelectID(searchID), Placeholder: m.SearchPlaceholder, Options: options},
		}},
	)
}

func Notice(content string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{text(content)}
}

func TextLength(components []discordgo.MessageComponent) int {
	n := 0
	for _, c := range components {
		switch v := c.(type) {
		case discordgo.Container:
			n += TextLength(v.Components)
		case discordgo.Section:
			n += TextLength(v.Components)
		case discordgo.TextDisplay:
			n += utf8.RuneCountInString(v.Content)
		}
	}
	return n
}

func AddModal(m *i18n.Messages, source string) *discordgo.InteractionResponseData {
	required := true
	options := make([]discordgo.SelectMenuOption, len(SearchSources))
	for n, s := range SearchSources {
		options[n] = discordgo.SelectMenuOption{Label: format.SourceName(s), Value: s, Default: s == SearchSource(source)}
	}
	return &discordgo.InteractionResponseData{
		CustomID: IDAddModal,
		Title:    m.ModalTitle,
		Components: []discordgo.MessageComponent{
			discordgo.Label{
				Label: m.ModalQueryLabel,
				Component: discordgo.TextInput{
					CustomID:    IDQuery,
					Placeholder: m.ModalQueryPlaceholder,
					Style:       discordgo.TextInputShort,
					Required:    &required,
					MinLength:   1,
					MaxLength:   MaxQuery,
				},
			},
			discordgo.Label{
				Label: m.ModalSourceLabel,
				Component: discordgo.SelectMenu{
					MenuType:    discordgo.StringSelectMenu,
					CustomID:    IDSource,
					Placeholder: m.ModalSourceLabel,
					Options:     options,
				},
			},
		},
	}
}

func SearchSource(source string) string {
	for _, s := range SearchSources {
		if strings.EqualFold(source, s) {
			return s
		}
	}
	return SourceYouTube
}

func ModalValue(components []discordgo.MessageComponent, customID string) string {
	for _, c := range components {
		if value, ok := componentValue(c, customID); ok {
			return value
		}
	}
	return ""
}

func componentValue(c discordgo.MessageComponent, customID string) (string, bool) {
	switch v := c.(type) {
	case *discordgo.ActionsRow:
		return componentsValue(v.Components, customID)
	case discordgo.ActionsRow:
		return componentsValue(v.Components, customID)
	case *discordgo.Label:
		return componentValue(v.Component, customID)
	case discordgo.Label:
		return componentValue(v.Component, customID)
	case *discordgo.TextInput:
		return strings.TrimSpace(v.Value), v.CustomID == customID
	case discordgo.TextInput:
		return strings.TrimSpace(v.Value), v.CustomID == customID
	case *discordgo.SelectMenu:
		return firstValue(v.Values), v.CustomID == customID
	case discordgo.SelectMenu:
		return firstValue(v.Values), v.CustomID == customID
	}
	return "", false
}

func componentsValue(components []discordgo.MessageComponent, customID string) (string, bool) {
	for _, c := range components {
		if value, ok := componentValue(c, customID); ok {
			return value, true
		}
	}
	return "", false
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
