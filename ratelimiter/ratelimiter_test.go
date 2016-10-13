package ratelimiter

import (
	"testing"
	"time"
)

func TestLimiterQPS(t *testing.T) {
	testLimiterQPS(t, 10)
	testLimiterQPS(t, 100)
	testLimiterQPS(t, 100000)
}

func testLimiterQPS(t *testing.T, rate int) {
	limiter := NewLimiter()
	limiter.SetLimit(float64(rate))
	timer := time.After(time.Millisecond * 1000)
	i := 0
	for {
		select {
		case <-timer:
			limiter.Stop()
			expEvents := rate
			if i > expEvents {
				t.Errorf("Received number of events is bigger than expected. Got: %d; Expected: %d", i, expEvents)
			}

			expEventsPercent := (float64(i) / float64(expEvents)) * 100
			if expEventsPercent < 90 {
				t.Errorf("Received number of events is lesser than expected. Got: %d (%.2f%%); Expected: %d", i, expEventsPercent, expEvents)
			}
			return
		case <-limiter.QPS():
			i++

		}
	}
}
func TestLimiterStop(t *testing.T) {
	limiter := NewLimiter()
	limiter.SetLimit(1000)
	time.Sleep(50 * time.Millisecond)
	limiter.Stop()
	if len(limiter.ch) > 0 {
		t.Errorf("Limiter is not empty after Stop. Got: %d; Expected: %d", len(limiter.ch), 0)
	}
}

func TestLimiterSetLimit(t *testing.T) {
	limiter := NewLimiter()
	limiter.SetLimit(1000)
	time.Sleep(50 * time.Millisecond)
	limiter.SetLimit(1)
	if len(limiter.ch) > 0 {
		t.Errorf("Limiter is not empty after SetLimit. Got: %d; Expected: %d", len(limiter.ch), 0)
	}
}
