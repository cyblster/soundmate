package bot

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
	"github.com/cyblster/soundmate/internal/music"
	"github.com/cyblster/soundmate/internal/ui"
)

func TestCheckEnqueue(t *testing.T) {
	tests := []struct {
		user, bot string
		active    bool
		want      VoiceCheck
	}{
		{"", "", false, VoiceUserAbsent},
		{"", "10", true, VoiceUserAbsent},
		{"10", "", false, VoiceOK},
		{"10", "10", true, VoiceOK},
		{"10", "20", true, VoiceBotElsewhere},
		{"10", "20", false, VoiceOK},
	}
	for _, tt := range tests {
		if got := CheckEnqueue(tt.user, tt.bot, tt.active); got != tt.want {
			t.Errorf("CheckEnqueue(%q, %q, %v) = %v, want %v", tt.user, tt.bot, tt.active, got, tt.want)
		}
	}
}

func TestCheckControl(t *testing.T) {
	tests := []struct {
		user, bot string
		want      VoiceCheck
	}{
		{"", "10", VoiceUserAbsent},
		{"10", "", VoiceNotWithBot},
		{"10", "20", VoiceNotWithBot},
		{"10", "10", VoiceOK},
	}
	for _, tt := range tests {
		if got := CheckControl(tt.user, tt.bot); got != tt.want {
			t.Errorf("CheckControl(%q, %q) = %v, want %v", tt.user, tt.bot, got, tt.want)
		}
	}
}

func TestVoiceCheckMessage(t *testing.T) {
	m := i18n.Get(i18n.Russian)
	want := map[VoiceCheck]string{
		VoiceOK:           "",
		VoiceUserAbsent:   m.UserNotInVoice,
		VoiceBotElsewhere: m.BotInAnotherChannel,
		VoiceNotWithBot:   m.UserNotWithBot,
	}
	for check, msg := range want {
		if got := check.Message(m); got != msg {
			t.Errorf("%v.Message() = %q, want %q", check, got, msg)
		}
	}
}

func TestCountListeners(t *testing.T) {
	states := []*discordgo.VoiceState{
		{UserID: "bot", ChannelID: "1"},
		{UserID: "a", ChannelID: "1"},
		{UserID: "b", ChannelID: "2"},
		nil,
	}
	if n := CountListeners(states, "1", "bot"); n != 1 {
		t.Errorf("CountListeners(1) = %d, want 1", n)
	}
	if n := CountListeners(states, "3", "bot"); n != 0 {
		t.Errorf("CountListeners(3) = %d, want 0", n)
	}
}

func TestIsAdmin(t *testing.T) {
	if IsAdmin(nil) || IsAdmin(&discordgo.Member{Permissions: discordgo.PermissionSendMessages}) {
		t.Error("regular members must not be admins")
	}
	if !IsAdmin(&discordgo.Member{Permissions: discordgo.PermissionAdministrator | discordgo.PermissionSendMessages}) {
		t.Error("administrator permission must be detected")
	}
}

func TestExemptFromSlowMode(t *testing.T) {
	if ExemptFromSlowMode(nil) || ExemptFromSlowMode(&discordgo.Member{Permissions: discordgo.PermissionSendMessages | discordgo.PermissionVoiceConnect}) {
		t.Error("regular members must respect slow mode")
	}
	for _, p := range []int64{discordgo.PermissionAdministrator, discordgo.PermissionManageMessages, discordgo.PermissionManageChannels} {
		if !ExemptFromSlowMode(&discordgo.Member{Permissions: p}) {
			t.Errorf("permission %d must bypass slow mode like Discord does", p)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		member *discordgo.Member
		want   string
	}{
		{nil, ""},
		{&discordgo.Member{}, ""},
		{&discordgo.Member{Nick: "nick", User: &discordgo.User{Username: "user", GlobalName: "global"}}, "nick"},
		{&discordgo.Member{User: &discordgo.User{Username: "user", GlobalName: "global"}}, "global"},
		{&discordgo.Member{User: &discordgo.User{Username: "user"}}, "user"},
	}
	for _, tt := range tests {
		if got := DisplayName(tt.member); got != tt.want {
			t.Errorf("DisplayName(%+v) = %q, want %q", tt.member, got, tt.want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	unknownMessage := &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeUnknownMessage}}
	unknownChannel := &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeUnknownChannel}}
	status404 := &discordgo.RESTError{Response: &http.Response{StatusCode: http.StatusNotFound}}
	forbidden := &discordgo.RESTError{Response: &http.Response{StatusCode: http.StatusForbidden}, Message: &discordgo.APIErrorMessage{Code: 50001}}
	for _, err := range []error{unknownMessage, unknownChannel, status404, fmt.Errorf("wrapped: %w", unknownMessage)} {
		if !IsNotFound(err) {
			t.Errorf("IsNotFound(%v) = false, want true", err)
		}
	}
	for _, err := range []error{nil, errors.New("network"), forbidden} {
		if IsNotFound(err) {
			t.Errorf("IsNotFound(%v) = true, want false", err)
		}
	}
}

func TestCommands(t *testing.T) {
	commands := Commands()
	names := make([]string, len(commands))
	for n, c := range commands {
		names[n] = c.Name
		if len(c.Name) > 32 || len(c.Description) > 100 || c.Contexts == nil || len(*c.Contexts) != 1 || (*c.Contexts)[0] != discordgo.InteractionContextGuild {
			t.Errorf("command %q must be a guild command within Discord limits", c.Name)
		}
		if c.NameLocalizations != nil || c.DescriptionLocalizations == nil || len(*c.DescriptionLocalizations) == 0 {
			t.Errorf("command %q must keep an english name and localize its description", c.Name)
		}
		for _, d := range *c.DescriptionLocalizations {
			if d == "" || len([]rune(d)) > 100 {
				t.Errorf("command %q has an invalid localized description %q", c.Name, d)
			}
		}
		if c.Name != CommandSetup && c.DefaultMemberPermissions != nil {
			t.Errorf("command %q must be available to everyone", c.Name)
		}
	}
	if want := []string{CommandSetup, CommandAdd, CommandSkip, CommandDisconnect, CommandHistory}; fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("commands = %v, want %v", names, want)
	}
	add := commands[1]
	if len(add.Options) != 2 || add.Options[0].Name != OptionQuery || add.Options[0].Required || add.Options[0].MaxLength != ui.MaxQuery {
		t.Fatalf("add must have an optional query option, got %+v", add.Options)
	}
	if source := add.Options[1]; source.Name != OptionSource || source.Required || len(source.Choices) != 2 || source.Choices[0].Value != ui.SourceYouTube || source.Choices[1].Name != "SoundCloud" {
		t.Fatalf("add must have an optional source option, got %+v", source)
	}
	if (*commands[2].DescriptionLocalizations)[discordgo.Russian] != i18n.Get(i18n.Russian).CommandSkipDescription {
		t.Error("skip description must be localized to russian")
	}
	setup := commands[0]
	if setup.Name != CommandSetup || setup.Description == "" {
		t.Fatalf("unexpected command: %+v", setup)
	}
	if setup.DefaultMemberPermissions == nil || *setup.DefaultMemberPermissions != discordgo.PermissionAdministrator {
		t.Error("setup must be restricted to administrators by default")
	}
	if (*setup.DescriptionLocalizations)[discordgo.Russian] != i18n.Get(i18n.Russian).CommandSetupDescription {
		t.Error("setup description must be localized to russian")
	}
	option := setup.Options[0]
	if option.Name != OptionLanguage || option.NameLocalizations != nil || option.Required || len(option.Choices) != len(i18n.Supported()) {
		t.Fatalf("unexpected language option: %+v", option)
	}
	for _, c := range option.Choices {
		if _, err := i18n.Parse(c.Value.(string)); err != nil || c.Name != c.Value {
			t.Errorf("choice %q must be a supported locale code", c.Value)
		}
	}
	if len(setup.Name) > 32 || len(setup.Description) > 100 || len(option.Description) > 100 {
		t.Error("command name or description exceeds Discord limits")
	}
}

func TestOptionString(t *testing.T) {
	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "other", Type: discordgo.ApplicationCommandOptionString, Value: "x"},
		{Name: OptionLanguage, Type: discordgo.ApplicationCommandOptionString, Value: "ru"},
	}
	if got := OptionString(options, OptionLanguage); got != "ru" {
		t.Errorf("OptionString() = %q, want ru", got)
	}
	if got := OptionString(options, "missing"); got != "" {
		t.Errorf("OptionString() for missing option = %q", got)
	}
}

func TestFingerprint(t *testing.T) {
	m := i18n.Get(i18n.English)
	track := music.Track{Title: "Song", Source: "youtube"}
	a := Fingerprint(ui.PlayerMessage(m, ui.Player{}))
	if a == 0 {
		t.Fatal("fingerprint must not be zero")
	}
	if b := Fingerprint(ui.PlayerMessage(m, ui.Player{})); a != b {
		t.Fatal("fingerprint must be deterministic")
	}
	if c := Fingerprint(ui.PlayerMessage(m, ui.Player{Current: &track})); a == c {
		t.Fatal("different players must produce different fingerprints")
	}
	if d := Fingerprint(ui.PlayerMessage(i18n.Get(i18n.Russian), ui.Player{})); a == d {
		t.Fatal("different locales must produce different fingerprints")
	}
	if e := Fingerprint(ui.PlayerMessage(m, ui.Player{Current: &track, Skip: ui.Votes{Count: 1, Needed: 2}})); e == Fingerprint(ui.PlayerMessage(m, ui.Player{Current: &track})) {
		t.Fatal("votes must change the fingerprint")
	}
}

func TestToSnowflake(t *testing.T) {
	if toSnowflake("123456789012345678").String() != "123456789012345678" {
		t.Error("valid ids must be converted")
	}
	if toSnowflake("invalid") != 0 {
		t.Error("invalid ids must be converted to zero")
	}
}
