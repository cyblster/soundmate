package bot

import "time"

func (b *Bot) scheduleIdleLeave(guildID string) {
	b.idleMu.Lock()
	defer b.idleMu.Unlock()
	if t, ok := b.idleTimers[guildID]; ok {
		t.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(b.idleTimeout, func() {
		b.idleMu.Lock()
		current, ok := b.idleTimers[guildID]
		if !ok || current != timer {
			b.idleMu.Unlock()
			return
		}
		delete(b.idleTimers, guildID)
		b.idleMu.Unlock()
		if b.players.Get(guildID).Active() {
			return
		}
		b.log.Info("idle timeout reached, leaving", "guild", b.guildName(guildID))
		b.leave(guildID)
	})
	b.idleTimers[guildID] = timer
}

func (b *Bot) cancelIdleLeave(guildID string) {
	b.idleMu.Lock()
	defer b.idleMu.Unlock()
	if t, ok := b.idleTimers[guildID]; ok {
		t.Stop()
		delete(b.idleTimers, guildID)
	}
}

func (b *Bot) idleGuilds() []string {
	b.idleMu.Lock()
	defer b.idleMu.Unlock()
	result := make([]string, 0, len(b.idleTimers))
	for guildID := range b.idleTimers {
		result = append(result, guildID)
	}
	return result
}
