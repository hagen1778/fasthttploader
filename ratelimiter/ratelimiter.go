package ratelimiter

import (
	"sync"
	"time"
)

// Limiter represent object,
// which allows to set QueryPerSecond limit
// and provides a channel (via QPS()) which is
// filled by messages according to limit
type Limiter struct {
	ch     chan struct{}
	doneCh chan struct{}
	ticker *time.Ticker

	mu        sync.Mutex
	limit     float64
	lastEvent time.Time
}

const bufferSize = 1e6

// NewLimiter inits and returns new Limiter obj
func NewLimiter() *Limiter {
	l := &Limiter{
		ch:     make(chan struct{}, bufferSize),
		doneCh: make(chan struct{}),
		ticker: time.NewTicker(5 * time.Millisecond),
	}
	go l.start()

	return l
}

func (l *Limiter) start() {
	var surplus float64
	for {
		select {
		case <-l.doneCh:
			return
		case <-l.ticker.C:
			now := time.Now()
			l.mu.Lock()
			tokens := (now.Sub(l.lastEvent).Seconds() * l.limit) + surplus
			l.mu.Unlock()

			n := int(tokens) - len(l.ch)
			if n <= 0 {
				continue
			}
			surplus = tokens - float64(int(tokens))
			for i := 0; i < n; i++ {
				l.ch <- struct{}{}
			}

			l.mu.Lock()
			l.lastEvent = now
			l.mu.Unlock()
		}
	}
}

// QPS returns channel which would be populated with messages
// according to set limit
func (l *Limiter) QPS() chan struct{} {
	return l.ch
}

// Stop stops generating messages into QPS channel
// cant be used after Stop
func (l *Limiter) Stop() {
	l.ticker.Stop()
	l.doneCh <- struct{}{}
	drainChan(l.ch)
}

// Limit returns current QPS rate
// is thread-safe
func (l *Limiter) Limit() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.limit
}

// SetLimit updates current QPS value
// also clears current channel from messages
// is thread-safe
func (l *Limiter) SetLimit(n float64) {
	if n < 1 {
		n = 1
	}
	l.setLimit(0)
	drainChan(l.ch)
	l.setLimit(n)
}

func (l *Limiter) setLimit(n float64) {
	l.mu.Lock()
	l.limit = n
	l.lastEvent = time.Now()
	l.mu.Unlock()
}

func drainChan(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
			continue
		default:
			return
		}
	}
}
