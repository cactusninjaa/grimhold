// Package cache provides a small in-memory key/value cache whose entries
// expire after a fixed time-to-live. A background reaper goroutine purges
// expired entries periodically.
package cache

import (
	"sync"
	"time"
)

type entry struct {
	createdAt time.Time
	val       []byte
}

// Cache is safe for concurrent use.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	ttl     time.Duration
	now     func() time.Time

	stop     chan struct{}
	stopOnce sync.Once
}

// Option customises a Cache.
type Option func(*Cache)

// WithClock injects the clock used to timestamp and expire entries.
// Mostly useful in tests.
func WithClock(now func() time.Time) Option {
	return func(c *Cache) { c.now = now }
}

// New creates a cache whose entries live for ttl. If reapInterval is
// positive, a goroutine purges expired entries every reapInterval until
// Stop is called.
func New(ttl, reapInterval time.Duration, opts ...Option) *Cache {
	c := &Cache{
		entries: make(map[string]entry),
		ttl:     ttl,
		now:     time.Now,
		stop:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	if reapInterval > 0 {
		go c.reapLoop(reapInterval)
	}
	return c
}

// Add stores val under key, replacing any previous value.
func (c *Cache) Add(key string, val []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry{createdAt: c.now(), val: val}
}

// Get returns the value stored under key, if present and not expired.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.expired(e) {
		delete(c.entries, key)
		return nil, false
	}
	return e.val, true
}

// Len reports the number of entries currently held, expired or not.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Reap removes every expired entry and returns how many were removed.
func (c *Cache) Reap() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k, e := range c.entries {
		if c.expired(e) {
			delete(c.entries, k)
			n++
		}
	}
	return n
}

// Stop terminates the reaper goroutine. It is safe to call more than once.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
}

// expired must be called with c.mu held.
func (c *Cache) expired(e entry) bool {
	return c.now().Sub(e.createdAt) >= c.ttl
}

func (c *Cache) reapLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.Reap()
		case <-c.stop:
			return
		}
	}
}
