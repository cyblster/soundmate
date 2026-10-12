package cooldown

import (
	"testing"
	"time"
)

func TestTracker(t *testing.T) {
	now := time.Unix(1000, 0)
	tr := New()
	tr.now = func() time.Time { return now }
	if _, ok := tr.Remaining("a"); ok {
		t.Fatal("unknown key must not be cooling down")
	}
	tr.Start("a", time.Minute)
	if until, ok := tr.Remaining("a"); !ok || !until.Equal(now.Add(time.Minute)) {
		t.Fatalf("Remaining() = %v, %v", until, ok)
	}
	if _, ok := tr.Remaining("b"); ok {
		t.Fatal("keys must be independent")
	}
	now = now.Add(time.Minute)
	if _, ok := tr.Remaining("a"); ok {
		t.Fatal("cooldown must end after the period")
	}
	tr.Start("b", 0)
	if _, ok := tr.until["a"]; ok {
		t.Fatal("expired entries must be pruned")
	}
	if _, ok := tr.Remaining("b"); ok {
		t.Fatal("zero period must not start a cooldown")
	}
}
