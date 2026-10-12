package bot

import (
	"context"
	"time"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
)

const statsInterval = 15 * time.Second

func FrameLoss(stats lavalink.Stats) (frames *lavalink.FrameStats, lossy bool) {
	if stats.PlayingPlayers == 0 || stats.FrameStats == nil {
		return nil, false
	}
	return stats.FrameStats, stats.FrameStats.Nulled > 0 || stats.FrameStats.Deficit > 0
}

func (b *Bot) watchFrames(ctx context.Context, node disgolink.Node) {
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()
	var last lavalink.Duration = -1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		stats := node.Stats()
		if stats.Uptime == last {
			continue
		}
		last = stats.Uptime
		frames, lossy := FrameLoss(stats)
		if frames == nil {
			continue
		}
		attrs := []any{"players", stats.PlayingPlayers, "sent", frames.Sent, "nulled", frames.Nulled, "deficit", frames.Deficit}
		if lossy {
			b.log.Warn("audio frames lost", attrs...)
			continue
		}
		b.log.Debug("audio frames", attrs...)
	}
}
