package debounce

import (
	"sync"
	"time"
)

type Coalescer struct {
	delay time.Duration
	fn    func(key string)

	mu      sync.Mutex
	pending map[string]bool
	running map[string]*sync.Mutex
	closed  bool
	wg      sync.WaitGroup
}

func New(delay time.Duration, fn func(key string)) *Coalescer {
	return &Coalescer{
		delay:   delay,
		fn:      fn,
		pending: map[string]bool{},
		running: map[string]*sync.Mutex{},
	}
}

func (c *Coalescer) Trigger(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.pending[key] {
		return
	}
	c.pending[key] = true
	c.wg.Add(1)
	time.AfterFunc(c.delay, func() {
		defer c.wg.Done()
		c.run(key)
	})
}

func (c *Coalescer) run(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	lock, ok := c.running[key]
	if !ok {
		lock = &sync.Mutex{}
		c.running[key] = lock
	}
	c.mu.Unlock()

	lock.Lock()
	defer lock.Unlock()
	c.fn(key)
}

func (c *Coalescer) Now(key string) {
	c.run(key)
}

func (c *Coalescer) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.wg.Wait()
}
