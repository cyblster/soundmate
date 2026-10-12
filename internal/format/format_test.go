package format

import (
	"testing"
	"time"
	"unicode/utf8"
)

func TestDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00"},
		{-time.Second, "00:00"},
		{999 * time.Millisecond, "00:00"},
		{59 * time.Second, "00:59"},
		{3*time.Minute + 5*time.Second, "03:05"},
		{time.Hour, "01:00:00"},
		{23*time.Hour + 59*time.Minute + 59*time.Second, "23:59:59"},
		{24 * time.Hour, "01:00:00:00"},
		{50*time.Hour + 2*time.Second, "02:02:00:02"},
	}
	for _, tt := range tests {
		if got := Duration(tt.in); got != tt.want {
			t.Errorf("Duration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 8, "this is…"},
		{"Привет, мир", 7, "Привет…"},
		{"anything", 0, ""},
	}
	for _, tt := range tests {
		got := Truncate(tt.in, tt.limit)
		if got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
		if tt.limit > 0 && utf8.RuneCountInString(got) > tt.limit {
			t.Errorf("Truncate(%q, %d) exceeds the limit", tt.in, tt.limit)
		}
	}
}

func TestIsURL(t *testing.T) {
	tests := map[string]bool{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": true,
		"http://example.com":                          true,
		"  https://youtu.be/dQw4w9WgXcQ  ":            true,
		"rick astley never gonna give you up":         false,
		"ftp://example.com/file.mp3":                  false,
		"https://":                                    false,
		"youtube.com/watch?v=1":                       false,
		"":                                            false,
	}
	for in, want := range tests {
		if got := IsURL(in); got != want {
			t.Errorf("IsURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEscapeMarkdown(t *testing.T) {
	in := `[Live] *Best* _of_ ~2024~ | a\b`
	want := `\[Live\] \*Best\* \_of\_ \~2024\~ \| a\\b`
	if got := EscapeMarkdown(in); got != want {
		t.Errorf("EscapeMarkdown() = %q, want %q", got, want)
	}
}

func TestThumbnail(t *testing.T) {
	if got := Thumbnail("youtube", "dQw4w9WgXcQ", "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"); got != "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg" {
		t.Errorf("unexpected youtube thumbnail: %q", got)
	}
	if got := Thumbnail("soundcloud", "123", "https://i1.sndcdn.com/artworks-1-original.jpg"); got != "https://i1.sndcdn.com/artworks-1-t500x500.jpg" {
		t.Errorf("soundcloud artwork must be resized, got %q", got)
	}
	if got := Thumbnail("bandcamp", "123", "https://cdn/art-original.jpg"); got != "https://cdn/art-original.jpg" {
		t.Errorf("other sources must keep artwork, got %q", got)
	}
	if got := Thumbnail("http", "", ""); got != "" {
		t.Errorf("missing artwork must produce empty thumbnail, got %q", got)
	}
}

func TestSourceName(t *testing.T) {
	tests := map[string]string{
		"youtube":    "YouTube",
		"SoundCloud": "SoundCloud",
		"deezer":     "Deezer",
		"":           "",
	}
	for in, want := range tests {
		if got := SourceName(in); got != want {
			t.Errorf("SourceName(%q) = %q, want %q", in, got, want)
		}
	}
}
