package cache

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock, safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestGetRespectsTTL(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c := New(5*time.Minute, 0, WithClock(clock.Now))
	defer c.Stop()

	c.Add("goblin", []byte("stats"))

	tests := []struct {
		name    string
		advance time.Duration
		wantHit bool
	}{
		{"fresh", 0, true},
		{"just before expiry", 5*time.Minute - time.Second, true},
		{"at expiry", time.Second, false},
	}
	for _, tt := range tests {
		clock.Advance(tt.advance)
		got, ok := c.Get("goblin")
		if ok != tt.wantHit {
			t.Fatalf("%s: hit = %v, want %v", tt.name, ok, tt.wantHit)
		}
		if ok && string(got) != "stats" {
			t.Fatalf("%s: got %q", tt.name, got)
		}
	}
}

func TestReapRemovesOnlyExpired(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	c := New(time.Minute, 0, WithClock(clock.Now))
	defer c.Stop()

	c.Add("old", []byte("1"))
	clock.Advance(40 * time.Second)
	c.Add("new", []byte("2"))
	clock.Advance(30 * time.Second) // old is 70s, new is 30s

	if n := c.Reap(); n != 1 {
		t.Fatalf("Reap removed %d entries, want 1", n)
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
	if _, ok := c.Get("new"); !ok {
		t.Fatal("fresh entry was reaped")
	}
}

func TestReaperGoroutinePurges(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	c := New(time.Second, 5*time.Millisecond, WithClock(clock.Now))
	defer c.Stop()

	c.Add("k", []byte("v"))
	clock.Advance(2 * time.Second)

	deadline := time.Now().Add(time.Second)
	for c.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("reaper never purged the expired entry")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(time.Minute, time.Millisecond)
	defer c.Stop()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			c.Add(key, []byte{byte(i)})
			c.Get(key)
		}(i)
	}
	wg.Wait()
	c.Stop() // double Stop must be safe
}
