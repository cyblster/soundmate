package logger

import (
	"context"
	"log/slog"
	"strings"
)

const lavalinkStatsRace = "expected ready message but got lavalink.StatsMessage"

func LavalinkAdapter(log *slog.Logger) *slog.Logger {
	return slog.New(lavalinkHandler{log.Handler()}).With(slog.String(sourceKey, "disgolink"))
}

type lavalinkHandler struct {
	slog.Handler
}

func (h lavalinkHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelError && statsRace(r) {
		if !h.Handler.Enabled(ctx, slog.LevelDebug) {
			return nil
		}
		r.Level = slog.LevelDebug
	}
	return h.Handler.Handle(ctx, r)
}

func (h lavalinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return lavalinkHandler{h.Handler.WithAttrs(attrs)}
}

func (h lavalinkHandler) WithGroup(name string) slog.Handler {
	return lavalinkHandler{h.Handler.WithGroup(name)}
}

func statsRace(r slog.Record) bool {
	race := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "err" && strings.Contains(a.Value.String(), lavalinkStatsRace) {
			race = true
			return false
		}
		return true
	})
	return race
}
