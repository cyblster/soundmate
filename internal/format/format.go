package format

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	days := total / 86400
	hours := total % 86400 / 3600
	minutes := total % 3600 / 60
	seconds := total % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%02d:%02d:%02d:%02d", days, hours, minutes, seconds)
	case hours > 0:
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	default:
		return fmt.Sprintf("%02d:%02d", minutes, seconds)
	}
}

func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func IsURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "`", "\\`",
	"|", `\|`, "[", `\[`, "]", `\]`, ">", `\>`,
)

func EscapeMarkdown(s string) string {
	return markdownEscaper.Replace(s)
}

func Thumbnail(source, identifier, artwork string) string {
	if strings.EqualFold(source, "youtube") && identifier != "" {
		return "https://i.ytimg.com/vi/" + url.PathEscape(identifier) + "/mqdefault.jpg"
	}
	if strings.EqualFold(source, "soundcloud") {
		return strings.Replace(artwork, "-original.", "-t500x500.", 1)
	}
	return artwork
}

var sourceNames = map[string]string{
	"youtube":    "YouTube",
	"soundcloud": "SoundCloud",
	"bandcamp":   "Bandcamp",
	"twitch":     "Twitch",
	"vimeo":      "Vimeo",
	"http":       "HTTP",
}

func SourceName(source string) string {
	if name, ok := sourceNames[strings.ToLower(source)]; ok {
		return name
	}
	if source == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(source)
	return strings.ToUpper(string(r)) + source[size:]
}
