package logger

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestLavalinkAdapterDowngradesStatsRace(t *testing.T) {
	race := errors.New("expected ready message but got lavalink.StatsMessage")

	var buf bytes.Buffer
	log := LavalinkAdapter(New(&buf, false)).With(slog.String("node_name", "main"))
	log.Error("failed to reconnect node", slog.Any("err", race), slog.Int("try", 0))
	if buf.Len() != 0 {
		t.Fatalf("stats race must be hidden without debug: %q", buf.String())
	}
	log.Error("failed to reconnect node", slog.Any("err", errors.New("connection refused")), slog.Int("try", 1))
	if out := ansi.ReplaceAllString(buf.String(), ""); !strings.Contains(out, "ERROR disgolink") || !strings.Contains(out, " try=1") || !strings.Contains(out, "node_name=main") {
		t.Fatalf("other errors must stay errors: %q", out)
	}

	buf.Reset()
	LavalinkAdapter(New(&buf, true)).Error("failed to reconnect node", slog.Any("err", race))
	if out := ansi.ReplaceAllString(buf.String(), ""); !strings.Contains(out, "DEBUG disgolink") || !strings.Contains(out, "StatsMessage") {
		t.Fatalf("stats race must be logged as debug: %q", out)
	}
}
