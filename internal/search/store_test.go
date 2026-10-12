package search

import (
	"sync"
	"testing"
	"time"
)

func TestPutTake(t *testing.T) {
	s := NewStore[string](time.Minute, nil)
	id := s.Put("tracks")
	if len(id) != 16 {
		t.Fatalf("unexpected id %q", id)
	}
	v, ok := s.Take(id)
	if !ok || v != "tracks" {
		t.Fatalf("Take() = %q, %v", v, ok)
	}
	if _, ok := s.Take(id); ok {
		t.Fatal("entry must be removed after Take()")
	}
	if _, ok := s.Take("missing"); ok {
		t.Fatal("Take() of unknown id must fail")
	}
}

func TestExpireCallsCallback(t *testing.T) {
	var mu sync.Mutex
	var expired []int
	done := make(chan struct{})
	s := NewStore[int](5*time.Millisecond, func(v int) {
		mu.Lock()
		expired = append(expired, v)
		mu.Unlock()
		close(done)
	})
	id := s.Put(42)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("entry did not expire")
	}
	if _, ok := s.Take(id); ok {
		t.Fatal("expired entry must not be returned")
	}
	if len(expired) != 1 || expired[0] != 42 || s.Len() != 0 {
		t.Fatalf("expired = %v, len = %d", expired, s.Len())
	}
}

func TestTakeCancelsExpiration(t *testing.T) {
	called := make(chan struct{}, 1)
	s := NewStore[int](10*time.Millisecond, func(int) { called <- struct{}{} })
	id := s.Put(1)
	if _, ok := s.Take(id); !ok {
		t.Fatal("Take() failed")
	}
	select {
	case <-called:
		t.Fatal("expiration callback must not run for taken entries")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestPutAvoidsCollisions(t *testing.T) {
	s := NewStore[int](time.Minute, nil)
	ids := []string{"same", "same", "other"}
	s.newID = func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}
	first := s.Put(1)
	second := s.Put(2)
	if first != "same" || second != "other" {
		t.Fatalf("ids = %q, %q; want same, other", first, second)
	}
}
