package bot

import (
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/format"
	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/ui"
)

const (
	CommandSetup      = "setup"
	CommandAdd        = "add"
	CommandSkip       = "skip"
	CommandDisconnect = "disconnect"
	CommandHistory    = "history"
	OptionLanguage    = "language"
	OptionSlowMode    = "slowmode"
	OptionQuery       = "query"
	OptionSource      = "source"
)

const MaxSlowMode = 6 * time.Hour

func sourceChoices() []*discordgo.ApplicationCommandOptionChoice {
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(ui.SearchSources))
	for n, s := range ui.SearchSources {
		choices[n] = &discordgo.ApplicationCommandOptionChoice{Name: format.SourceName(s), Value: s}
	}
	return choices
}

func Commands() []*discordgo.ApplicationCommand {
	minSlowMode := 0.0
	admin := int64(discordgo.PermissionAdministrator)
	en := i18n.Get(i18n.English)
	guildOnly := func() *[]discordgo.InteractionContextType {
		return &[]discordgo.InteractionContextType{discordgo.InteractionContextGuild}
	}
	return []*discordgo.ApplicationCommand{
		{
			Name:                     CommandSetup,
			Description:              en.CommandSetupDescription,
			DescriptionLocalizations: i18n.Localizations(func(m *i18n.Messages) string { return m.CommandSetupDescription }),
			DefaultMemberPermissions: &admin,
			Contexts:                 guildOnly(),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:                     discordgo.ApplicationCommandOptionString,
					Name:                     OptionLanguage,
					Description:              en.CommandSetupLanguageDescription,
					DescriptionLocalizations: *i18n.Localizations(func(m *i18n.Messages) string { return m.CommandSetupLanguageDescription }),
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: string(i18n.English), Value: string(i18n.English)},
						{Name: string(i18n.Russian), Value: string(i18n.Russian)},
					},
				},
				{
					Type:                     discordgo.ApplicationCommandOptionInteger,
					Name:                     OptionSlowMode,
					Description:              en.CommandSetupSlowModeDescription,
					DescriptionLocalizations: *i18n.Localizations(func(m *i18n.Messages) string { return m.CommandSetupSlowModeDescription }),
					MinValue:                 &minSlowMode,
					MaxValue:                 MaxSlowMode.Seconds(),
				},
			},
		},
		{
			Name:                     CommandAdd,
			Description:              en.CommandAddDescription,
			DescriptionLocalizations: i18n.Localizations(func(m *i18n.Messages) string { return m.CommandAddDescription }),
			Contexts:                 guildOnly(),
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:                     discordgo.ApplicationCommandOptionString,
					Name:                     OptionQuery,
					Description:              en.CommandAddQueryDescription,
					DescriptionLocalizations: *i18n.Localizations(func(m *i18n.Messages) string { return m.CommandAddQueryDescription }),
					MaxLength:                ui.MaxQuery,
				},
				{
					Type:                     discordgo.ApplicationCommandOptionString,
					Name:                     OptionSource,
					Description:              en.CommandAddSourceDescription,
					DescriptionLocalizations: *i18n.Localizations(func(m *i18n.Messages) string { return m.CommandAddSourceDescription }),
					Choices:                  sourceChoices(),
				},
			},
		},
		{
			Name:                     CommandSkip,
			Description:              en.CommandSkipDescription,
			DescriptionLocalizations: i18n.Localizations(func(m *i18n.Messages) string { return m.CommandSkipDescription }),
			Contexts:                 guildOnly(),
		},
		{
			Name:                     CommandDisconnect,
			Description:              en.CommandDisconnectDescription,
			DescriptionLocalizations: i18n.Localizations(func(m *i18n.Messages) string { return m.CommandDisconnectDescription }),
			Contexts:                 guildOnly(),
		},
		{
			Name:                     CommandHistory,
			Description:              en.CommandHistoryDescription,
			DescriptionLocalizations: i18n.Localizations(func(m *i18n.Messages) string { return m.CommandHistoryDescription }),
			Contexts:                 guildOnly(),
		},
	}
}

func OptionString(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range options {
		if o.Name == name && o.Type == discordgo.ApplicationCommandOptionString {
			return o.StringValue()
		}
	}
	return ""
}

func OptionInt(options []*discordgo.ApplicationCommandInteractionDataOption, name string) (int64, bool) {
	for _, o := range options {
		if o.Name == name && o.Type == discordgo.ApplicationCommandOptionInteger {
			return o.IntValue(), true
		}
	}
	return 0, false
}
