package i18n

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Locale string

const (
	English Locale = "en"
	Russian Locale = "ru"
)

const Default = English

type Messages struct {
	CommandSetupDescription         string
	CommandSetupLanguageDescription string
	CommandSetupSlowModeDescription string
	CommandAddDescription           string
	CommandAddQueryDescription      string
	CommandAddSourceDescription     string
	CommandSkipDescription          string
	CommandDisconnectDescription    string
	CommandHistoryDescription       string

	SetupAdminOnly       string
	SetupWrongChannel    string
	SetupUnknownLanguage string
	SetupChannelTopic    string
	SetupInProgress      string
	SetupDone            string
	SetupFailed          string
	SetupUpdated         string
	SetupUnchanged       string
	SetupChannelChange   string
	SetupLanguageChange  string
	SetupSlowModeChange  string
	SlowModeDisabled     string
	LanguageName         string

	ActionAdd        string
	ActionSkip       string
	ActionDisconnect string
	ActionHistory    string

	UserNotInVoice      string
	UserNotWithBot      string
	BotInAnotherChannel string
	NotConfigured       string
	PlayerUnavailable   string
	GenericError        string
	NothingPlaying      string
	SlowMode            string
	Skipped             string
	Disconnected        string
	VoteSkip            string
	VoteDisconnect      string
	VoteRetracted       string

	ModalTitle            string
	ModalQueryLabel       string
	ModalQueryPlaceholder string
	ModalSourceLabel      string

	SearchTitle       string
	SearchPlaceholder string
	SearchNothing     string
	SearchFailed      string
	SearchExpired     string

	TrackAdded    string
	TrackFailed   string
	PlaylistAdded string

	NowPlaying      string
	RequestedBy     string
	IdleTitle       string
	IdleHint        string
	DurationLive    string
	DurationUnknown string

	QueueTitle    string
	QueueCount    string
	QueueEmpty    string
	QueueOverflow string
	SlowModeHint  string
	VotesCast     string

	HistoryTitle string
	HistoryEmpty string
}

var catalog = map[Locale]*Messages{
	English: &english,
	Russian: &russian,
}

func Supported() []Locale {
	return []Locale{English, Russian}
}

func Parse(value string) (Locale, error) {
	normalized := Locale(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := catalog[normalized]; ok {
		return normalized, nil
	}
	names := make([]string, 0, len(catalog))
	for _, l := range Supported() {
		names = append(names, string(l))
	}
	return "", fmt.Errorf("unsupported locale %q, supported locales: %s", value, strings.Join(names, ", "))
}

func Get(locale Locale) *Messages {
	if m, ok := catalog[locale]; ok {
		return m
	}
	return catalog[Default]
}

var discordLocales = map[discordgo.Locale]Locale{
	discordgo.Russian:   Russian,
	discordgo.EnglishUS: English,
	discordgo.EnglishGB: English,
}

func FromDiscord(locale discordgo.Locale) (Locale, bool) {
	l, ok := discordLocales[locale]
	return l, ok
}

func Localizations(pick func(*Messages) string) *map[discordgo.Locale]string {
	result := make(map[discordgo.Locale]string, len(discordLocales))
	for discord, l := range discordLocales {
		result[discord] = pick(Get(l))
	}
	return &result
}
