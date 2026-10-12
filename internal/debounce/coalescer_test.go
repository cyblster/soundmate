package debounce

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTriggerCoalescesBurst(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	c := New(20*time.Millisecond, func(key string) {
		mu.Lock()
		calls[key]++
		mu.Unlock()
	})
	for range 50 {
		c.Trigger("a")
	}
	c.Trigger("b")
	c.Close()
	if calls["a"] != 1 || calls["b"] != 1 {
		t.Fatalf("calls = %v, want one call per key", calls)
	}
}

func TestTriggerAfterRunSchedulesAgain(t *testing.T) {
	var calls atomic.Int32
	c := New(5*time.Millisecond, func(string) { calls.Add(1) })
	c.Trigger("a")
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	c.Trigger("a")
	c.Close()
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestRunsForSameKeyAreSerialized(t *testing.T) {
	var active, maxActive atomic.Int32
	c := New(time.Millisecond, func(string) {
		n := active.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
	})
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() { c.Now("a") })
	}
	wg.Wait()
	c.Close()
	if maxActive.Load() != 1 {
		t.Fatalf("max concurrent runs = %d, want 1", maxActive.Load())
	}
}

func TestCloseDropsNewTriggers(t *testing.T) {
	var calls atomic.Int32
	c := New(time.Millisecond, func(string) { calls.Add(1) })
	c.Close()
	c.Trigger("a")
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("trigger after Close() must be ignored")
	}
}
