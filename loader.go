package main

import (
	"fmt"
	"time"
	"log"
	"sync"
	"os"
	"runtime/pprof"
	"math"

	"golang.org/x/time/rate"
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
	Qps rate.Limit

	// Duration is the duration of test running.
	Duration time.Duration
	er uint64
	t time.Time

	sync.Mutex
}
var (
	stopCh = make(chan struct{})
 	throttle *rate.Limiter
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
	timeout := time.After(time.Second*10)
	tick := time.Tick(time.Millisecond*1000)
	c.AddWorkers(100)
	for {
		select {
		case <-tick:
			if c.Amount() < 600 {
				c.AddWorkers(100)
			}
		case <-timeout:
			m := c.Metrics()
			flawed = (m.Errors/m.RequestSum)*100 > 3 // just more than 3% of errors
			avgQPS = float64(m.RequestSum)/10
			fmt.Printf("Average QPS for %d workers is: %f; Errs: %d; Req done: %d\n", c.Amount(), avgQPS, m.Errors, m.RequestSum)
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
	tick := time.Tick(time.Millisecond*500)
	for {
		select {
		case <-timeout:
			fmt.Println("Timeout")
			stopCh <- struct{}{}
		case <-tick:
			l.calibrate()

			//if err := pushgateway.Push(m); err != nil {
			//	fmt.Printf("%s\n", err)
			//}
		}
	}

	l.finalizeProgress()
}

var multiplier float64
var await = 0
func (l *Loader) calibrate(){
	since := time.Since(l.t).Seconds()
	fmt.Println("------------")
	fmt.Printf("[ Multiplier = %f ]\n", multiplier)
	fmt.Printf("QPS was increased to: %f\nWorkers: %d\nJobsch len: %d\n", l.Qps, c.Amount(), len(c.Jobsch))
	fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", c.Metrics().OpenConns, c.Metrics().RequestSum, c.Metrics().Errors, c.Metrics().Timeouts)
	fmt.Printf(" >> Real Req/s: %f; Transfer/s: %f kb;\n", float64(c.Metrics().RequestSum)/since, float64(c.Metrics().BytesWritten)/(since*1024))
	fmt.Println("------------")

	if await > 0 {
		fmt.Println("wait...")
		await -=1
		return
	}

	if math.Abs(multiplier) < 0.0001 {
		fmt.Println("Zeroed")
		stopCh <- struct{}{}
		return
	}
	if !l.isFlawed() {
		if len(c.Jobsch) > 0 {
			n := int(float64(c.Amount()) * multiplier)
			c.AddWorkers(n)
			await += 1
		} else {
			l.adjustQPS()
		}
	} else {
		multiplier /= 1.2
		await += 3
	}
}

func (l *Loader) adjustQPS() {
	if l.Qps < 1000000 {
		l.Qps = l.Qps * rate.Limit(1+multiplier)
		throttle.SetLimit(l.Qps)
	}
}

func (l *Loader) isFlawed() bool {
	if c.Metrics().Errors > 0 && l.er != c.Metrics().Errors {
		l.er = c.Metrics().Errors
		return true
	}

	return false
}

func (l *Loader) load() {
	if flawed {
		c.AddWorkers(25*10)
		l.Qps = rate.Limit(avgQPS/2)
	} else {
		c.AddWorkers(50*10)
		l.Qps = rate.Limit(avgQPS)
	}

	throttle = rate.NewLimiter(l.Qps, 1)
	multiplier = 0.1
	for {
		select {
		case <-stopCh:
			c.Flush()
			return
		default:
			if throttle.Allow(){
				c.Jobsch <- struct{}{}
			}
		}
	}
}
