package main

import (
	"fmt"
	"time"
	"strings"
	"strconv"
	"log"

	"github.com/valyala/fasthttp"
	"github.com/hagen1778/fasthttploader/metrics"
	"github.com/hagen1778/fasthttploader/pushgateway"
	"sync/atomic"
	"sync"
	//"math"
	"os"
	"runtime/pprof"
)

func (l *Loader) startProgress() {
	l.t = time.Now()
	fmt.Println("Start loading")
}

func (l *Loader) finalizeProgress() {
	fmt.Println("Done")
}

func (l *Loader) inc() {

}

type Loader struct {
	// Request is the request to be made.
	Request *fasthttp.Request

	// Qps is the rate limit.
	Qps int

	// Duration is the duration of test running.
	Duration time.Duration

	prevQps	int
	host    string
	workers []*metrics.Worker
	jobsch chan struct{}
	t time.Time
	er uint64

	sync.Mutex
}
var stopCh = make(chan struct{})
var throttle = make(<-chan time.Time)
var m *metrics.Metrics
func (l *Loader) Run() {
	l.host = convertHost(l.Request)
	multiplier = 0.1
	l.prevQps = l.Qps
	fmt.Printf("Begin with:\n host: %s\nQPS:%d\n", l.host, l.Qps)
	pushgateway.Init()
	m = &metrics.Metrics{}
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

func (l *Loader) startCountdown(){
	l.startProgress()
	timeout := time.After(l.Duration)
	tick := time.Tick(500*time.Millisecond)
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
	since := time.Since(l.t).Seconds()
	overflow := len(l.jobsch)
	workersNum := len(l.workers)

	//if !l.isFlawed() {
	//	if len(l.jobsch) > 1000 || overflow == jobCapacity {
	//		if workersNum < 600 {
	//			//l.runWorkers(10)
	//		}
	//	} else {
	//		//multiplier += (1-(multiplier/100)) * 0.1
	//		multiplier = math.Abs(multiplier)
	//		l.prevQps = l.Qps
	//		l.adjustQPS()
	//	}
	//} else {
	//	fmt.Printf("Step back from %d to %d\n", l.Qps, l.prevQps)
	//	l.Qps = l.prevQps
	//	multiplier = math.Abs(multiplier) / -1.1
	//	l.adjustQPS()
	//	l.prevQps = l.Qps
	//}
	fmt.Println("------------")
	fmt.Printf("[ Multiplier = %f ]\n", multiplier)
	fmt.Printf("QPS was increased to: %d\nWorkers: %d\nJobsch len: %d\n", l.Qps, workersNum, overflow)
	fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", m.OpenConns, m.RequestSum, m.Errors, m.Timeouts)
	fmt.Printf(" >> Real Req/s: %f; Transfer/s: %f kb; overflow: %d\n", float64(m.RequestSum)/since, float64(m.BytesWritten)/(since*1024),len(l.jobsch))
	fmt.Println("------------")

}

func (l *Loader) adjustQPS() {
	return
	if l.Qps < 100000 {
		l.Qps = int(float64(l.Qps) * (1+multiplier))
		throttle = time.Tick(time.Duration(1e6/l.Qps) * time.Microsecond)
	}
}

func (l *Loader) isFlawed() bool {
	fmt.Printf("Check for errs - l.err: %d; m.err: %d\n", l.er, m.Errors)
	if m.Errors > 0 && l.er != m.Errors {
		l.er = m.Errors
		return true
	}

	return false
}

func convertHost(req *fasthttp.Request) string {
	addr := string(req.URI().Host())
	if len(addr) == 0 {
		log.Fatalf("address cannot be empty")
	}
	tmp := strings.SplitN(addr, ":", 2)
	if len(tmp) != 2 {
		return tmp[0]+":80"
	}
	port := tmp[1]
	portInt, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("cannot parse port %q of addr %q: %s", port, addr, err)
	}
	if portInt < 0 {
		log.Fatalf("upstreamHosts port %d cannot be negative: %q", portInt, addr)
	}

	return addr
}

func (l *Loader) runWorkers(n int) {
	for i := 0; i<n; i++ {
		go l.runWorker()
	}
}

func (l *Loader) runWorker() {
	w := metrics.NewWorker(l.host, m)

	l.Lock()
	l.workers = append(l.workers, w)
	l.Unlock()

	var resp fasthttp.Response
	req := cloneRequest(l.Request)
	for range l.jobsch {
		s := time.Now()
		err := w.SendRequest(req, &resp, *t)
		if err != nil {
			if err == fasthttp.ErrTimeout {
				atomic.AddUint64(&m.Timeouts, 1)
			}
			fmt.Printf("Err while sending req: %s", err)
			atomic.AddUint64(&m.Errors, 1)
		}
		//fmt.Println(resp.String())
		//os.Exit(1)
		atomic.AddUint64(&m.RequestDuration, uint64(time.Since(s)))
		atomic.AddUint64(&m.RequestSum, 1)
	}
}

const jobCapacity = 10000
func (l *Loader) load() {
	throttle = time.Tick(time.Duration(1e6/(l.Qps)) * time.Microsecond)
	l.jobsch = make(chan struct{}, jobCapacity)
	l.runWorkers(140)
	for {
		select {
		case <-stopCh:
			close(l.jobsch)
			return
		default:
			//<-throttle
			l.jobsch <- struct{}{}
		}
	}
	fmt.Println(">>>>>>>>>>>>>>>>>>>>>")
	close(l.jobsch)
}

func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
}
