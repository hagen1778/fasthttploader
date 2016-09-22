package metrics

import (
	"time"
	"sync"
)

type M struct {
	Connections	int
	Timeouts	int
	Errors		int
	RequestSum	int
	RequestDuration	time.Duration

	sync.Mutex
}