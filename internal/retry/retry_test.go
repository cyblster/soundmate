package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	p := Policy{Delay: time.Second, MaxDelay: 5 * time.Second}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}
	for i, w := range want {
		if got := p.Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if got := (Policy{Delay: time.Second}).Backoff(4); got != 8*time.Second {
		t.Errorf("unbounded Backoff(4) = %v, want 8s", got)
	}
}

func TestDoSucceedsAfterRetries(t *testing.T) {
	calls := 0
	var retries []int
	err := Policy{Attempts: 5, Delay: time.Millisecond}.Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	}, func(attempt int, _ time.Duration, _ error) { retries = append(retries, attempt) })
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if calls != 3 || len(retries) != 2 {
		t.Fatalf("calls = %d, retries = %v; want 3 calls and 2 retries", calls, retries)
	}
}

func TestDoReturnsLastError(t *testing.T) {
	sentinel := errors.New("boom")
	calls := 0
	err := Policy{Attempts: 3, Delay: time.Millisecond}.Do(context.Background(), func(context.Context) error {
		calls++
		return sentinel
	}, nil)
	if !errors.Is(err, sentinel) || calls != 3 {
		t.Fatalf("Do() = %v after %d calls, want boom after 3 calls", err, calls)
	}
}

func TestDoStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Policy{Attempts: 10, Delay: time.Hour}.Do(ctx, func(context.Context) error {
		calls++
		cancel()
		return errors.New("fail")
	}, nil)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("Do() = %v after %d calls, want context.Canceled after 1 call", err, calls)
	}
}

func TestDoRunsAtLeastOnce(t *testing.T) {
	calls := 0
	_ = Policy{}.Do(context.Background(), func(context.Context) error {
		calls++
		return errors.New("fail")
	}, nil)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
