package bot

import (
	"errors"
	"net/http"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/i18n"
)

type VoiceCheck int

const (
	VoiceOK VoiceCheck = iota
	VoiceUserAbsent
	VoiceBotElsewhere
	VoiceNotWithBot
)

func CheckEnqueue(userChannel, botChannel string, active bool) VoiceCheck {
	switch {
	case userChannel == "":
		return VoiceUserAbsent
	case active && botChannel != "" && botChannel != userChannel:
		return VoiceBotElsewhere
	default:
		return VoiceOK
	}
}

func CheckControl(userChannel, botChannel string) VoiceCheck {
	switch {
	case userChannel == "":
		return VoiceUserAbsent
	case botChannel == "" || botChannel != userChannel:
		return VoiceNotWithBot
	default:
		return VoiceOK
	}
}

func (c VoiceCheck) Message(m *i18n.Messages) string {
	switch c {
	case VoiceUserAbsent:
		return m.UserNotInVoice
	case VoiceBotElsewhere:
		return m.BotInAnotherChannel
	case VoiceNotWithBot:
		return m.UserNotWithBot
	default:
		return ""
	}
}

func CountListeners(states []*discordgo.VoiceState, channelID, botID string) int {
	n := 0
	for _, s := range states {
		if s != nil && s.ChannelID == channelID && s.UserID != botID {
			n++
		}
	}
	return n
}

func IsAdmin(member *discordgo.Member) bool {
	return member != nil && member.Permissions&discordgo.PermissionAdministrator != 0
}

const slowModeExemptPermissions = discordgo.PermissionAdministrator | discordgo.PermissionManageMessages | discordgo.PermissionManageChannels

func ExemptFromSlowMode(member *discordgo.Member) bool {
	return member != nil && member.Permissions&slowModeExemptPermissions != 0
}

func DisplayName(member *discordgo.Member) string {
	if member == nil {
		return ""
	}
	if member.Nick != "" {
		return member.Nick
	}
	if member.User != nil {
		return member.User.DisplayName()
	}
	return ""
}

func IsNotFound(err error) bool {
	var restErr *discordgo.RESTError
	if !errors.As(err, &restErr) {
		return false
	}
	if restErr.Message != nil {
		switch restErr.Message.Code {
		case discordgo.ErrCodeUnknownMessage, discordgo.ErrCodeUnknownChannel:
			return true
		}
	}
	return restErr.Response != nil && restErr.Response.StatusCode == http.StatusNotFound
}
