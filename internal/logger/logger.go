package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

const (
	timeFormat    = "2006-01-02 15:04:05.000"
	levelWidth    = 5
	sourceWidth   = 9
	messageWidth  = 40
	sourceKey     = "component"
	defaultSource = "bot"

	colorReset = "\x1b[0m"
	colorFaint = "\x1b[2m"
)

var sourceColors = map[string]string{
	"bot":       "\x1b[36m",
	"discordgo": "\x1b[96m",
	"disgolink": "\x1b[95m",
	"storage":   "\x1b[94m",
}

func levelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "\x1b[31m"
	case level >= slog.LevelWarn:
		return "\x1b[33m"
	case level >= slog.LevelInfo:
		return "\x1b[32m"
	default:
		return colorFaint
	}
}

func paint(color, s string) string {
	if color == "" {
		return s
	}
	return color + s + colorReset
}

func New(w io.Writer, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(&handler{mu: &sync.Mutex{}, w: w, level: level, source: defaultSource})
}

type handler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Level
	source string
	attrs  string
	group  string
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	source := h.source
	var attrs strings.Builder
	attrs.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		if h.group == "" && a.Key == sourceKey {
			source = a.Value.String()
			return true
		}
		appendAttr(&attrs, h.group, a)
		return true
	})

	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	message := r.Message
	if strings.ContainsFunc(message, unicode.IsControl) {
		message = strconv.Quote(message)
	}

	var b strings.Builder
	b.WriteString(paint(colorFaint, t.UTC().Format(timeFormat)))
	b.WriteByte(' ')
	b.WriteString(paint(levelColor(r.Level), pad(r.Level.String(), levelWidth)))
	b.WriteByte(' ')
	b.WriteString(paint(sourceColors[source], pad(source, sourceWidth)))
	b.WriteByte(' ')
	if attrs.Len() > 0 {
		b.WriteString(pad(message, messageWidth))
		b.WriteString(attrs.String())
	} else {
		b.WriteString(message)
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range attrs {
		if h.group == "" && a.Key == sourceKey {
			c.source = a.Value.String()
			continue
		}
		appendAttr(&b, h.group, a)
	}
	c.attrs = b.String()
	return &c
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.group += name + "."
	return &c
}

func appendAttr(b *strings.Builder, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, ga := range a.Value.Group() {
			appendAttr(b, prefix, ga)
		}
		return
	}
	b.WriteByte(' ')
	b.WriteString(quote(prefix + a.Key))
	b.WriteByte('=')
	value := a.Value.String()
	if a.Value.Kind() == slog.KindTime {
		value = a.Value.Time().UTC().Format(timeFormat)
	}
	b.WriteString(quote(value))
}

func quote(s string) string {
	if s == "" || strings.ContainsFunc(s, func(r rune) bool {
		return r == ' ' || r == '=' || r == '"' || !unicode.IsPrint(r)
	}) {
		return strconv.Quote(s)
	}
	return s
}

func pad(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func DiscordLevel(debug bool) int {
	if debug {
		return discordgo.LogInformational
	}
	return discordgo.LogWarning
}

func DiscordAdapter(log *slog.Logger) func(level, caller int, format string, args ...any) {
	log = log.With(slog.String(sourceKey, "discordgo"))
	return func(level, _ int, format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		switch level {
		case discordgo.LogError:
			log.Error(message)
		case discordgo.LogWarning:
			log.Warn(message)
		case discordgo.LogInformational:
			log.Info(message)
		default:
			log.Debug(message)
		}
	}
}
