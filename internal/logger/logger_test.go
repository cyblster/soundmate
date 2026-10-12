package logger

import (
	"bytes"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var timePrefix = regexp.MustCompile(`(?m)^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3} `)

func lines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	out := ansi.ReplaceAllString(strings.TrimSuffix(buf.String(), "\n"), "")
	if n := len(strings.Split(out, "\n")); len(timePrefix.FindAllString(out, -1)) != n {
		t.Fatalf("every line must start with a timestamp:\n%s", out)
	}
	return strings.Split(timePrefix.ReplaceAllString(out, ""), "\n")
}

func TestNewTextRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, false)
	log.Debug("hidden")
	log.Info("visible", "guild", "test")
	got := lines(t, &buf)
	want := "INFO  bot       visible" + strings.Repeat(" ", messageWidth-len("visible")) + " guild=test"
	if len(got) != 1 || got[0] != want {
		t.Errorf("unexpected output:\n%q\nwant\n%q", got, want)
	}
}

func TestNewWithDebug(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, true).Debug("details")
	if got := lines(t, &buf); got[0] != "DEBUG bot       details" {
		t.Errorf("unexpected output: %q", got)
	}
}

func TestNewAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, false)
	log.Info("short", "a", 1)
	log.With("component", "storage").Warn("a somewhat longer message", "b", 2)
	log.Error("ошибка", "c", 3)
	got := lines(t, &buf)
	column := func(l string) int { return len([]rune(l[:strings.LastIndex(l, " ")])) }
	if column(got[0]) != column(got[1]) || column(got[1]) != column(got[2]) {
		t.Errorf("attributes must be aligned:\n%s", strings.Join(got, "\n"))
	}
	if !strings.HasPrefix(got[1], "WARN  storage   ") || !strings.HasPrefix(got[2], "ERROR bot       ") {
		t.Errorf("unexpected level or source columns:\n%s", strings.Join(got, "\n"))
	}
}

func TestNewFormatsValues(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, false).With("node", "main").WithGroup("req")
	log.Info("multi\nline",
		"empty", "",
		"spaced", "a b",
		"err", errors.New("boom"),
		"at", time.Date(2026, 1, 2, 3, 4, 5, 6e6, time.UTC),
		slog.Group("user", "id", 7),
		"component", "grouped",
	)
	got := lines(t, &buf)[0]
	for _, want := range []string{
		`INFO  bot       "multi\nline"`,
		` node=main req.empty="" req.spaced="a b" req.err=boom req.at="2026-01-02 03:04:05.006" req.user.id=7 req.component=grouped`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q does not contain %q", got, want)
		}
	}
}

func TestNewColorsLevelsAndSources(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, true)
	log.Debug("d")
	log.Info("i")
	log.With("component", "discordgo").Warn("w")
	log.With("component", "unknown").Error("e")
	got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	for n, want := range []string{
		"\x1b[2mDEBUG\x1b[0m \x1b[36mbot      \x1b[0m d",
		"\x1b[32mINFO \x1b[0m \x1b[36mbot      \x1b[0m i",
		"\x1b[33mWARN \x1b[0m \x1b[96mdiscordgo\x1b[0m w",
		"\x1b[31mERROR\x1b[0m unknown   e",
	} {
		if !strings.HasPrefix(got[n], colorFaint) || !strings.HasSuffix(got[n], want) {
			t.Errorf("line %d = %q, want suffix %q", n, got[n], want)
		}
	}
}

func TestDiscordAdapterMapsLevels(t *testing.T) {
	var buf bytes.Buffer
	adapter := DiscordAdapter(New(&buf, true))
	adapter(discordgo.LogError, 0, "failed %d", 1)
	adapter(discordgo.LogWarning, 0, "warned")
	adapter(discordgo.LogInformational, 0, "informed")
	adapter(discordgo.LogDebug, 0, "debugged")
	want := []string{
		"ERROR discordgo failed 1",
		"WARN  discordgo warned",
		"INFO  discordgo informed",
		"DEBUG discordgo debugged",
	}
	if got := lines(t, &buf); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("unexpected output:\n%s", strings.Join(got, "\n"))
	}
}

func TestDiscordLevel(t *testing.T) {
	if DiscordLevel(true) != discordgo.LogInformational || DiscordLevel(false) != discordgo.LogWarning {
		t.Error("unexpected discordgo log levels")
	}
}
