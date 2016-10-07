package ratelimiter

import (
	"testing"
	"time"
)

func TestLimiter_QPS(t *testing.T) {
	testLimiter_QPS(t, 10)
	testLimiter_QPS(t, 300000)
}

func testLimiter_QPS(t *testing.T, rate int) {
	limiter := NewLimiter()
	limiter.SetLimit(float64(rate))
	timer := time.After(time.Second)
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
func TestLimiter_Stop(t *testing.T) {
	limiter := NewLimiter()
	limiter.SetLimit(1000)
	time.Sleep(50 * time.Millisecond)
	limiter.Stop()
	if len(limiter.ch) > 0 {
		t.Errorf("Limiter is not empty after Stop. Got: %d; Expected: %d", len(limiter.ch), 0)
	}
}

func TestLimiter_SetLimit(t *testing.T) {
	limiter := NewLimiter()
	limiter.SetLimit(1000)
	time.Sleep(50 * time.Millisecond)
	limiter.SetLimit(1)
	if len(limiter.ch) > 0 {
		t.Errorf("Limiter is not empty after SetLimit. Got: %d; Expected: %d", len(limiter.ch), 0)
	}
}
