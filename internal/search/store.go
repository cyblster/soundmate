package search

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Store[T any] struct {
	ttl       time.Duration
	onExpire  func(T)
	mu        sync.Mutex
	entries   map[string]*entry[T]
	newID     func() string
	afterFunc func(time.Duration, func()) *time.Timer
}

type entry[T any] struct {
	value T
	timer *time.Timer
}

func NewStore[T any](ttl time.Duration, onExpire func(T)) *Store[T] {
	return &Store[T]{
		ttl:       ttl,
		onExpire:  onExpire,
		entries:   map[string]*entry[T]{},
		newID:     randomID,
		afterFunc: time.AfterFunc,
	}
}

func (s *Store[T]) Put(value T) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.newID()
	for _, exists := s.entries[id]; exists; _, exists = s.entries[id] {
		id = s.newID()
	}
	e := &entry[T]{value: value}
	e.timer = s.afterFunc(s.ttl, func() { s.expire(id, e) })
	s.entries[id] = e
	return id
}

func (s *Store[T]) Take(id string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		var zero T
		return zero, false
	}
	e.timer.Stop()
	delete(s.entries, id)
	return e.value, true
}

func (s *Store[T]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *Store[T]) expire(id string, e *entry[T]) {
	s.mu.Lock()
	current, ok := s.entries[id]
	if !ok || current != e {
		s.mu.Unlock()
		return
	}
	delete(s.entries, id)
	s.mu.Unlock()
	if s.onExpire != nil {
		s.onExpire(e.value)
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
