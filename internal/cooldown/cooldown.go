package cooldown

import (
	"sync"
	"time"
)

type Tracker struct {
	mu    sync.Mutex
	until map[string]time.Time
	now   func() time.Time
}

func New() *Tracker {
	return &Tracker{until: map[string]time.Time{}, now: time.Now}
}

func (t *Tracker) Remaining(key string) (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.until[key]
	if !ok || !t.now().Before(until) {
		return time.Time{}, false
	}
	return until, true
}

func (t *Tracker) Start(key string, period time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, until := range t.until {
		if !now.Before(until) {
			delete(t.until, k)
		}
	}
	if period > 0 {
		t.until[key] = now.Add(period)
	}
}
