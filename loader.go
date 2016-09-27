package main

import (
	"fmt"
	"time"
	"log"
	"sync"
	"os"
	"runtime/pprof"
	"math"

	"github.com/valyala/fasthttp"
	"github.com/hagen1778/fasthttploader/metrics"
	"github.com/hagen1778/fasthttploader/pushgateway"
)

func (l *Loader) startProgress() {
	l.t = time.Now()
	fmt.Println("Start loading")
}

func (l *Loader) finalizeProgress() {
	fmt.Println("Done")
}

type Loader struct {
	// Request is the request to be made.
	Request *fasthttp.Request

	// Qps is the rate limit.
	Qps int

	// Duration is the duration of test running.
	Duration time.Duration
	er uint64
	t time.Time

	sync.Mutex
}
var (
	stopCh = make(chan struct{})
 	throttle = make(<-chan time.Time)
)

var c *metrics.Client

func (l *Loader) Run() {
	c = metrics.Init(l.Request, *t)
	pushgateway.Init()

	l.initialQPS()
	go l.startCountdown()
	l.load()

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
		return
	}
}

var avgQPS float64
var flawed bool
func (l *Loader) initialQPS() {
	timeout := time.After(time.Second*5)
	c.AddWorkers(500)
	for {
		select {
		case <-timeout:
			flawed = c.Metrics().Errors > 0
			avgQPS = float64(c.Metrics().RequestSum)/5
			fmt.Printf("Average QPS for 500 workers is: %f; Errs: %d; Req done: %d\n", avgQPS, c.Metrics().Errors, c.Metrics().RequestSum)
			c.Flush()
			return
		default:
			c.Jobsch <- struct{}{}
		}
	}
}

func (l *Loader) startCountdown(){
	l.startProgress()
	timeout := time.After(l.Duration)
	tick := time.Tick(*t)
	for {
		select {
		case <-timeout:
			fmt.Println("Timeout")
			stopCh <- struct{}{}
		case <-tick:
			l.Lock()
			l.calibrate()
			l.Unlock()

			//if err := pushgateway.Push(m); err != nil {
			//	fmt.Printf("%s\n", err)
			//}
		}
	}

	l.finalizeProgress()
}

var multiplier float64

func (l *Loader) calibrate(){
	if math.Abs(multiplier) < 0.0001 {
		fmt.Println("Zeroed")
		stopCh <- struct{}{}
		return
	}
	if !l.isFlawed() {
		if len(c.Jobsch) > 10 {
			//if workersNum < 600 {
				c.AddWorkers(10)
			//}
		} else {
			//multiplier += (1-(multiplier/100)) * 0.1
			multiplier = math.Abs(multiplier)
			l.adjustQPS()
		}
	} else {
		if multiplier < 0 {
			multiplier /= 1.2
		}
		multiplier = math.Abs(multiplier)*-1
		l.adjustQPS()
	}

	since := time.Since(l.t).Seconds()
	fmt.Println("------------")
	fmt.Printf("[ Multiplier = %f ]\n", multiplier)
	fmt.Printf("QPS was increased to: %d\nWorkers: %d\nJobsch len: %d\n", l.Qps, c.Amount(), len(c.Jobsch))
	fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", c.Metrics().OpenConns, c.Metrics().RequestSum, c.Metrics().Errors, c.Metrics().Timeouts)
	fmt.Printf(" >> Real Req/s: %f; Transfer/s: %f kb;\n", float64(c.Metrics().RequestSum)/since, float64(c.Metrics().BytesWritten)/(since*1024))
	fmt.Println("------------")

}

func (l *Loader) adjustQPS() {
	if l.Qps < 1000000 {
		l.Qps = int(float64(l.Qps) * (1+multiplier))
		throttle = time.Tick(time.Duration(1e6/l.Qps) * time.Microsecond)
	}
}

func (l *Loader) isFlawed() bool {
	fmt.Printf("Check for errs - l.err: %d; m.err: %d\n", l.er, c.Metrics().Errors)
	if c.Metrics().Errors > 0 && l.er != c.Metrics().Errors {
		l.er = c.Metrics().Errors
		return true
	}

	return false
}

func (l *Loader) load() {
	if flawed {
		c.AddWorkers(5*10)
		l.Qps = int(avgQPS/2)
	} else {
		c.AddWorkers(50*10)
		l.Qps = int(avgQPS)
	}


	throttle = time.Tick(time.Duration(1e6/l.Qps) * time.Microsecond)
	multiplier = 0.1
	for {
		select {
		case <-stopCh:
			c.Flush()
			return
		default:
			<-throttle
			c.Jobsch <- struct{}{}
		}
	}
}
