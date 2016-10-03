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

const (
	// Duration of burst-testing, without qps-limit. Used to estimate start test conditions
	calibrateDuration = 10 * time.Second

	// Duration of adjustable testing, while trying to reach max qps with minimal lvl of errors
	adjustmentDuration = 40 * time.Second

	// Period of sample taking, while testing
	samplePeriod = 500 * time.Millisecond
)

type Loader struct {
	// Request is the request to be made.
	Request *fasthttp.Request

	// Qps is the rate limit.
	Qps rate.Limit

	// C is number of workers (clients)
	C int

	// Duration is the duration of test running.
	Duration time.Duration

	er uint64
	t time.Time
	throttle *rate.Limiter

	sync.Mutex
}

func (l *Loader) startProgress() {
	l.t = time.Now()
	fmt.Println("Loading start!")
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
	testTime time.Time
	prev *prevState
	stopCh = make(chan struct{})

	c *metrics.Client
	r *report.Page
)

func (l *Loader) Run() {
	testTime = time.Now()
	prev = &prevState{}
	r = &report.Page{
		Title: string(l.Request.URI().Host()),
		RequestDuration: make(map[float64][]float64),
		Interval: samplePeriod.Seconds(),
	}
	c = metrics.Init(l.Request, *t)
	pushgateway.Init()
	l.throttle = rate.NewLimiter(1, 1)

	if l.Qps == 0 {
		l.makeAdjustment()
	} else {
		prev.qps = l.Qps
		prev.workers = l.C
	}
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

func (l *Loader) makeAdjustment() {
	l.calibrateQPS()

	go func(){
		l.startProgress()
		timeout := time.After(adjustmentDuration)
		tick := time.Tick(samplePeriod)
		for {
			select {
			case <-timeout:
				stopCh <- struct{}{}
				printSummary("Adjustment test")
				return
			case <-tick:
				l.calibrate()
			}
		}
	}()

	if prev.flawed {
		c.AddWorkers(prev.workers/2)
		l.setQPS(prev.qps/2)
	} else {
		c.AddWorkers(prev.workers)
		l.setQPS(prev.qps)
	}

	multiplier = 0.1
	l.load()

	fmt.Println("Adjustment Finished!")
}

func (l *Loader) calibrateQPS() {
	fmt.Println("Run initial callibrate QPS phase")
	timeout := time.After(calibrateDuration)
	c.AddWorkers(*cl)
	for {
		select {
		case <-timeout:
			prev.flawed = (metrics.Errors()/metrics.RequestSum())*100 > 2 // just more than 3% of errors
			prev.qps = rate.Limit(float64(metrics.RequestSum())/calibrateDuration.Seconds())
			prev.workers = c.Amount()
			fmt.Printf("Average QPS for %d workers is: %f; Errs: %d; Req done: %d\n", c.Amount(), prev.qps, metrics.Errors(), metrics.RequestSum())
			c.Flush()
			return
		default:
			c.Jobsch <- struct{}{}
		}
	}
}

func (l *Loader) makeTest() {
	s := time.Duration(l.Duration.Seconds()/2/10)
	stepTick := time.Tick(time.Second * s) // half of the time, 10 steps in first half
	stateTick := time.Tick(samplePeriod)
	workerStep := prev.workers/10
	qpsStep := prev.qps/10
	l.setQPS(qpsStep)
	c.AddWorkers(workerStep)
	go func(){
		l.startProgress()
		timeout := time.After(l.Duration)
		steps := 0
		for {
			select {
			case <-timeout:
				stopCh <- struct{}{}
				printSummary("Loading test")
				return
			case <-stepTick:
				if steps >= 10-1 {
					continue
				}
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

var multiplier float64
var await = 0
func (l *Loader) calibrate(){
	l.printState()
	if await > 0 {
		await -=1
		return
	}

	if math.Abs(multiplier) < 0.0001 {
		fmt.Println("Multiplier is negligible now. Stoping adjustment")
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
	if *debug {
		since := time.Since(l.t).Seconds()
		fmt.Println("------------")
		fmt.Printf("[ Multiplier = %f ]\n", multiplier)
		fmt.Printf("QPS was increased to: %f\nWorkers: %d\nJobsch len: %d\n", l.Qps, c.Amount(), len(c.Jobsch))
		fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", metrics.ConnOpen(), metrics.RequestSum(), metrics.Errors(), metrics.Timeouts())
		fmt.Printf(" >> Real Req/s: %f; Transfer/s: %f kb;\n", float64(metrics.RequestSum())/since, float64(metrics.BytesWritten())/(since*1024))
		fmt.Println("------------")
	}

	r.Lock()
	r.Connections = append(r.Connections, metrics.ConnOpen())
	r.Errors = append(r.Errors, metrics.Errors())
	r.Timeouts = append(r.Timeouts, metrics.Timeouts())
	r.RequestSum = append(r.RequestSum, metrics.RequestSum())
	r.RequestSuccess = append(r.RequestSuccess, metrics.RequestSuccess())
	r.BytesWritten = append(r.BytesWritten, metrics.BytesWritten())
	r.BytesRead = append(r.BytesRead, metrics.BytesRead())
	r.Qps = append(r.Qps, uint64(l.Qps))
	r.UpdateRequestDuration(metrics.RequestDuration())
	r.Unlock()
}

func (l *Loader) adjustQPS() {
	l.setQPS(l.Qps * rate.Limit(1+multiplier))
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
			return
		default:
			if l.throttle.Allow(){
				c.Jobsch <- struct{}{}
			}
		}
	}
}

func (l *Loader) makeReport() {
	f, err := os.Create(*fileName)
	if err != nil {
		log.Fatalf("Error while trying to create file: %s", err)
	}
	defer f.Close()

	f.WriteString(report.PrintPage(r))
	fmt.Printf("Check test results at %s\n", *fileName)
}

func printSummary(stage string) {
	since := time.Since(testTime).Seconds()
	fmt.Printf("\n------ %s ------\n", stage)
	fmt.Printf("Elapsed time: %fs\n", since)
	fmt.Printf("Req done: %d; Success: %f %%\n", metrics.RequestSum(), (float64(metrics.RequestSuccess())/float64(metrics.RequestSum()))*100)
	fmt.Printf("Rps: %f; Connections: %d\n", float64(metrics.RequestSum())/since, metrics.ConnOpen())
	fmt.Printf("Errors: %d; Timeouts: %d\n\n", metrics.Errors(), metrics.Timeouts())
	testTime = time.Now()
}
