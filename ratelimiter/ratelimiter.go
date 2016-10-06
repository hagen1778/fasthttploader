package ratelimiter

import (
	"sync"
	"time"
)

type Limiter struct {
	ch     chan struct{}
	done   chan struct{}
	ticker *time.Ticker

	mu        sync.Mutex
	limit     float64
	lastEvent time.Time
}

const bufferSize = 1e9

func NewLimiter() *Limiter {
	l := &Limiter{
		ch:     make(chan struct{}, bufferSize),
		done:   make(chan struct{}),
		ticker: time.NewTicker(10 * time.Millisecond),
	}
	go l.start()
	return l
}

func (l *Limiter) QPS() chan struct{} {
	return l.ch
}

func (l *Limiter) Stop() {
	l.ticker.Stop()
	l.done <- struct{}{}
}

func (l *Limiter) Limit() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.limit
}

func (l *Limiter) start() {
	for {
		select {
		case <-l.done:
			return
		case <-l.ticker.C:
			l.mu.Lock()
			limit := l.limit
			l.mu.Unlock()

			n := int(time.Now().Sub(l.lastEvent).Seconds() * limit)
			if n < len(l.ch) {
				continue
			}
			n = n - len(l.ch)
			for i := 0; i < n; i++ {
				l.ch <- struct{}{}
			}

			l.mu.Lock()
			l.lastEvent = time.Now()
			l.mu.Unlock()
		}
	}
}

func (l *Limiter) drain() {
	for {
		select {
		case <-l.ch:
			continue
		default:
			return
		}
	}
}

func (l *Limiter) SetLimit(n float64) {
	l.setLimit(0)
	l.drain()
	l.setLimit(n)
}

func (l *Limiter) setLimit(n float64) {
	l.mu.Lock()
	l.limit = n
	l.lastEvent = time.Now()
	l.mu.Unlock()
}
