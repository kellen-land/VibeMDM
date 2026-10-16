package agentapi

import (
	"sync"
	"time"
)

// deviceLimiter is a per-computer token bucket for endpoints a Mac calls in bursts (log
// shipping). A refused request leaves the agent's buffer intact, so it simply retries later.
type deviceLimiter struct {
	mu      sync.Mutex
	burst   float64
	perSec  float64
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newDeviceLimiter(burst int, every time.Duration) *deviceLimiter {
	return &deviceLimiter{burst: float64(burst), perSec: 1 / every.Seconds(), buckets: map[string]*bucket{}}
}

func (l *deviceLimiter) allow(id string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[id]
	if !ok {
		if len(l.buckets) > 100000 { // forget idle devices rather than grow without bound
			l.buckets = map[string]*bucket{}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[id] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.perSec)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
