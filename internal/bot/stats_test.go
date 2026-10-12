package bot

import (
	"testing"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

func TestFrameLoss(t *testing.T) {
	tests := []struct {
		name   string
		stats  lavalink.Stats
		report bool
		lossy  bool
	}{
		{"idle", lavalink.Stats{FrameStats: &lavalink.FrameStats{Sent: 3000}}, false, false},
		{"no frame stats", lavalink.Stats{PlayingPlayers: 1}, false, false},
		{"clean", lavalink.Stats{PlayingPlayers: 1, FrameStats: &lavalink.FrameStats{Sent: 3000}}, true, false},
		{"nulled", lavalink.Stats{PlayingPlayers: 1, FrameStats: &lavalink.FrameStats{Sent: 2900, Nulled: 100}}, true, true},
		{"deficit", lavalink.Stats{PlayingPlayers: 1, FrameStats: &lavalink.FrameStats{Sent: 2990, Deficit: 10}}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, lossy := FrameLoss(tt.stats)
			if (frames != nil) != tt.report || lossy != tt.lossy {
				t.Fatalf("FrameLoss() = %v, %v; want report=%v lossy=%v", frames, lossy, tt.report, tt.lossy)
			}
		})
	}
}
