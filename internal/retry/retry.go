package retry

import (
	"context"
	"time"
)

type Policy struct {
	Attempts int
	Delay    time.Duration
	MaxDelay time.Duration
}

func (p Policy) Backoff(attempt int) time.Duration {
	d := p.Delay
	for i := 1; i < attempt; i++ {
		d *= 2
		if p.MaxDelay > 0 && d >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		return p.MaxDelay
	}
	return d
}

func (p Policy) Do(ctx context.Context, fn func(context.Context) error, onRetry func(attempt int, wait time.Duration, err error)) error {
	attempts := max(p.Attempts, 1)
	var err error
	for attempt := 1; ; attempt++ {
		if err = fn(ctx); err == nil {
			return nil
		}
		if attempt >= attempts {
			return err
		}
		wait := p.Backoff(attempt)
		if onRetry != nil {
			onRetry(attempt, wait, err)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
