package ratelimiter

import (
	"sync"
	"time"
)

type Limiter struct {
	ch     chan struct{}
	ticker *time.Ticker
	wg     sync.WaitGroup

	mu        sync.Mutex
	limit     float64
	lastEvent time.Time
}

const bufferSize = 1e6

func NewLimiter() *Limiter {
	l := &Limiter{
		ch:     make(chan struct{}, bufferSize),
		ticker: time.NewTicker(5 * time.Millisecond),
	}
	func() {
		l.wg.Add(1)
		go l.start()
	}()

	return l
}

func (l *Limiter) QPS() chan struct{} {
	return l.ch
}

func (l *Limiter) Stop() {
	l.ticker.Stop()
	l.wg.Done()
	drainChan(l.ch)
}

func (l *Limiter) Limit() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.limit
}

func (l *Limiter) start() {
	var surplus float64
	for {
		select {
		case <-l.ticker.C:
			now := time.Now()
			l.mu.Lock()
			tokens := (now.Sub(l.lastEvent).Seconds() * l.limit) + surplus
			l.mu.Unlock()

			n := int(tokens)
			if n < len(l.ch) || n == 0 {
				continue
			}

			n = n - len(l.ch)
			surplus = tokens - float64(n)
			for i := 0; i < n; i++ {
				l.ch <- struct{}{}
			}

			l.mu.Lock()
			l.lastEvent = now
			l.mu.Unlock()
		}
	}
	l.wg.Wait()
}

func (l *Limiter) SetLimit(n float64) {
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
