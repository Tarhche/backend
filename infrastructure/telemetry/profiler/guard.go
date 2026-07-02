package profiler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// errRateLimited is returned by admit when the per-minute profile budget
	// is exhausted.
	errRateLimited = errors.New("profiler: profiles-per-minute limit reached")

	// errBufferFull is returned by reserve when holding another payload would
	// exceed the configured memory budget.
	errBufferFull = errors.New("profiler: profile buffer limit reached")
)

// resourceGuard enforces the hard resource limits: how many profiles may be
// collected per minute and how much memory in-flight profile payloads may
// hold. It protects the service from the profiler itself misbehaving.
type resourceGuard struct {
	maxPerMinute   int
	maxBufferBytes int64

	mu          sync.Mutex
	windowStart time.Time
	count       int

	buffered atomic.Int64

	now func() time.Time
}

func newResourceGuard(maxPerMinute int, maxBufferBytes int64) *resourceGuard {
	return &resourceGuard{
		maxPerMinute:   maxPerMinute,
		maxBufferBytes: maxBufferBytes,
		now:            time.Now,
	}
}

// admit reports whether another profile may be collected in the current
// one-minute window.
func (g *resourceGuard) admit() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if now.Sub(g.windowStart) >= time.Minute {
		g.windowStart = now
		g.count = 0
	}

	if g.count >= g.maxPerMinute {
		return errRateLimited
	}
	g.count++

	return nil
}

// reserve accounts n bytes of profile payload; the caller must release the
// same amount once the payload left the process (or was dropped).
func (g *resourceGuard) reserve(n int64) error {
	if g.buffered.Add(n) > g.maxBufferBytes {
		g.buffered.Add(-n)
		return errBufferFull
	}

	return nil
}

func (g *resourceGuard) release(n int64) {
	g.buffered.Add(-n)
}

// bufferedBytes returns the memory currently held by in-flight payloads.
func (g *resourceGuard) bufferedBytes() int64 {
	return g.buffered.Load()
}
