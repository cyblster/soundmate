package vote

import "sync"

type Kind int

const (
	Skip Kind = iota
	Disconnect
)

var Kinds = []Kind{Skip, Disconnect}

type Result struct {
	Added  bool
	Count  int
	Needed int
	Passed bool
}

func Needed(listeners int) int {
	return max(listeners, 1)/2 + 1
}

type key struct {
	guildID string
	kind    Kind
}

type Box struct {
	mu    sync.Mutex
	polls map[key]map[string]struct{}
}

func NewBox() *Box {
	return &Box{polls: map[key]map[string]struct{}{}}
}

func (b *Box) Cast(guildID string, kind Kind, userID string, eligible func(userID string) bool, needed int) Result {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := key{guildID, kind}
	voters := b.polls[k]
	if _, ok := voters[userID]; ok {
		delete(voters, userID)
		return Result{Count: countLocked(voters, eligible), Needed: needed}
	}
	if voters == nil {
		voters = map[string]struct{}{}
		b.polls[k] = voters
	}
	voters[userID] = struct{}{}
	r := Result{Added: true, Count: countLocked(voters, eligible), Needed: needed}
	if r.Count >= needed {
		r.Passed = true
		delete(b.polls, k)
	}
	return r
}

func (b *Box) Check(guildID string, kind Kind, eligible func(userID string) bool, needed int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := key{guildID, kind}
	voters, ok := b.polls[k]
	if !ok || countLocked(voters, eligible) < needed {
		return false
	}
	delete(b.polls, k)
	return true
}

func (b *Box) Count(guildID string, kind Kind, eligible func(userID string) bool) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return countLocked(b.polls[key{guildID, kind}], eligible)
}

func (b *Box) Clear(guildID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, kind := range Kinds {
		delete(b.polls, key{guildID, kind})
	}
}

func countLocked(voters map[string]struct{}, eligible func(string) bool) int {
	n := 0
	for id := range voters {
		if eligible(id) {
			n++
		}
	}
	return n
}
