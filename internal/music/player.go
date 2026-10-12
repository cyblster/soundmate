package music

import (
	"sync"
	"time"
)

type Player struct {
	mu      sync.Mutex
	current *Track
	queue   []Track
}

type Snapshot struct {
	Current  *Track
	Upcoming []Track
	Total    int
	Length   time.Duration
}

func (p *Player) Enqueue(tracks ...Track) (start *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(tracks) == 0 {
		return nil
	}
	p.queue = append(p.queue, tracks...)
	if p.current != nil {
		return nil
	}
	return p.advanceLocked()
}

func (p *Player) Advance() *Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.advanceLocked()
}

func (p *Player) advanceLocked() *Track {
	if len(p.queue) == 0 {
		p.current = nil
		return nil
	}
	next := p.queue[0]
	p.queue[0] = Track{}
	p.queue = p.queue[1:]
	if len(p.queue) == 0 {
		p.queue = nil
	}
	p.current = &next
	c := next
	return &c
}

func (p *Player) Current() *Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return nil
	}
	c := *p.current
	return &c
}

func (p *Player) Active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current != nil
}

func (p *Player) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = nil
	p.queue = nil
}

func (p *Player) Snapshot(limit int) Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := Snapshot{Total: len(p.queue)}
	for _, t := range p.queue {
		if !t.IsStream {
			s.Length += t.Duration
		}
	}
	if p.current != nil {
		c := *p.current
		s.Current = &c
	}
	n := len(p.queue)
	if limit >= 0 {
		n = min(limit, n)
	}
	s.Upcoming = make([]Track, n)
	copy(s.Upcoming, p.queue[:n])
	return s
}

type Manager struct {
	mu      sync.Mutex
	players map[string]*Player
}

func NewManager() *Manager {
	return &Manager{players: map[string]*Player{}}
}

func (m *Manager) Get(guildID string) *Player {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.players[guildID]
	if !ok {
		p = &Player{}
		m.players[guildID] = p
	}
	return p
}

func (m *Manager) Delete(guildID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.players, guildID)
}

func (m *Manager) Active() []string {
	m.mu.Lock()
	players := make(map[string]*Player, len(m.players))
	for id, p := range m.players {
		players[id] = p
	}
	m.mu.Unlock()
	var result []string
	for id, p := range players {
		if p.Active() {
			result = append(result, id)
		}
	}
	return result
}
