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
	"github.com/hagen1778/fasthttploader/report"
)

func (l *Loader) startProgress() {
	l.t = time.Now()
	fmt.Println("Loading start!")
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
	throttle *rate.Limiter

	sync.Mutex
}
type prevState struct {
	// Qps is the rate limit.
	qps rate.Limit

	// Number of workers
	workers int

	// True if there was any errors while testing
	flawed bool
}

var (
	stopCh = make(chan struct{})
	c *metrics.Client
	r *report.Page
)
func (l *Loader) Run() {
	r = &report.Page{
		Title: string(l.Request.URI().Host()),
	}
	c = metrics.Init(l.Request, *t)
	pushgateway.Init()

	l.throttle = rate.NewLimiter(1, 1)
	l.makeAdjustment()
	fmt.Println("Adjustment Finished!")
	time.Sleep(time.Second*4)
	l.makeTest()
	l.makeReport()

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

var prev *prevState

func (l *Loader) makeAdjustment() {
	prev = &prevState{}
	l.calibrateQPS()
	// TODO: move duration for calibrate phase to some const or flag
	go l.startCountdown(time.Second*30)
	// TODO: move this coef to const or make some formula to calculate them
	// probably, num of workers should be moved to flags as "supposed number of workers"
	if prev.flawed {
		c.AddWorkers(prev.workers/2)
		l.setQPS(prev.qps/2)
	} else {
		c.AddWorkers(prev.workers)
		l.setQPS(prev.qps)
	}

	multiplier = 0.1
	l.load()
}

func (l *Loader) makeTest() {
	// TODO: make sure, that user passed testing time more than... 10 sec for example
	s := time.Duration(l.Duration.Seconds()/2/10)
	stepTick := time.Tick(time.Second * s) // half of the time, 10 steps in first half
	stateTick := time.Tick(time.Millisecond*500)
	workerStep := prev.workers/10
	qpsStep := prev.qps/10
	l.setQPS(qpsStep)
	c.AddWorkers(workerStep)
	fmt.Printf("\n\nStart test with - qps: %f; connections: %d\n", qpsStep, workerStep)
	go func(){
		l.startProgress()
		timeout := time.After(l.Duration)
		steps := 0
		for {
			select {
			case <-timeout:
				fmt.Println(" >> Timeout")
				stopCh <- struct{}{}
				l.finalizeProgress()
				return
			case <-stepTick:
				if steps >= 10-1 {
					continue
				}
				fmt.Println("!!! Step incr")
				l.setQPS(l.Qps + qpsStep)
				c.AddWorkers(workerStep)
				steps++
			case <-stateTick:
				l.printState()

			//if err := pushgateway.Push(c.Metrics()); err != nil {
			//	fmt.Printf("%s\n", err)
			//}
			}
		}
	}()
	l.load()
}

func (l *Loader) calibrateQPS() {
	fmt.Println("Run initial callibrate QPS phase")
	timeout := time.After(time.Second*10)
	tick := time.Tick(time.Millisecond*1000)
	c.AddWorkers(100)
	for {
		select {
		case <-tick:
			if c.Amount() < 600 {
				c.AddWorkers(100)
			}
			//if err := pushgateway.Push(c.Metrics()); err != nil {
			//	fmt.Printf("%s\n", err)
			//}
		case <-timeout:
			prev.flawed = (metrics.Errors()/metrics.RequestSum())*100 > 2 // just more than 3% of errors
			// TODO: move somewhere calibrate time
			prev.qps = rate.Limit(float64(metrics.RequestSum())/10)
			prev.workers = c.Amount()
			fmt.Printf("Average QPS for %d workers is: %f; Errs: %d; Req done: %d\n", c.Amount(), prev.qps, metrics.Errors(), metrics.RequestSum())
			c.Flush()
			return
		default:
			c.Jobsch <- struct{}{}
		}
	}
}

func (l *Loader) startCountdown(d time.Duration){
	l.startProgress()
	timeout := time.After(d)
	tick := time.Tick(time.Millisecond*500)
	for {
		select {
		case <-timeout:
			l.finalizeProgress()
			stopCh <- struct{}{}
			return
		case <-tick:
			l.calibrate()

			//if err := pushgateway.Push(c.Metrics()); err != nil {
			//	fmt.Printf("%s\n", err)
			//}
		}
	}
}

var multiplier float64
var await = 0
func (l *Loader) calibrate(){
	l.printState()

	if await > 0 {
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
			await += 1
		}
	} else {
		multiplier /= 1.2
		await += 3
	}
}

func (l *Loader) printState() {
	since := time.Since(l.t).Seconds()
	fmt.Println("------------")
	fmt.Printf("[ Multiplier = %f ]\n", multiplier)
	fmt.Printf("QPS was increased to: %f\nWorkers: %d\nJobsch len: %d\n", l.Qps, c.Amount(), len(c.Jobsch))
	fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", metrics.ConnOpen(), metrics.RequestSum(), metrics.Errors(), metrics.Timeouts())
	fmt.Printf(" >> Real Req/s: %f; Transfer/s: %f kb;\n", float64(metrics.RequestSum())/since, float64(metrics.BytesWritten())/(since*1024))
	fmt.Println("------------")

	r.Lock()
	r.Connections = append(r.Connections, metrics.ConnOpen())
	r.Errors = append(r.Errors, metrics.Errors())
	r.Timeouts = append(r.Timeouts, metrics.Timeouts())
	r.RequestSum = append(r.RequestSum, metrics.RequestSum())
	r.Qps = append(r.Qps, uint64(l.Qps))
	r.Unlock()
}

func (l *Loader) adjustQPS() {
	// TODO: make smthng with this unnatural limit
	if l.Qps < 1000000 {
		l.setQPS(l.Qps * rate.Limit(1+multiplier))
	}
}

func (l *Loader) setQPS(qps rate.Limit) {
	l.Qps = qps
	l.throttle.SetLimit(qps)
}

func (l *Loader) isFlawed() bool {
	if metrics.Errors() > 0 && l.er != metrics.Errors() {
		l.er = metrics.Errors()
		return true
	}

	return false
}

func (l *Loader) load() {
	for {
		select {
		case <-stopCh:
			prev.qps = l.Qps
			prev.workers = c.Amount()
			c.Flush()
			fmt.Println("Loading done. Data flushed")
			return
		default:
			if l.throttle.Allow(){
				c.Jobsch <- struct{}{}
			}
		}
	}
}

func (l *Loader) makeReport() {
	// TODO: took filename from flags
	f, err := os.Create("report2.html")
	if err != nil {
		log.Fatalf("Error while trying to create file: %s", err)
	}
	defer f.Close()
	f.WriteString(report.PrintPage(r))
}