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
)

func (l *Loader) startProgress() {
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
	throttle <-chan time.Time

	sync.Mutex
}

var stopCh = make(chan struct{})
var m *metrics.Metrics
func (l *Loader) Run() {
	l.host = convertHost(l.Request)
	multiplier = 1.2
	fmt.Printf("Begin with:\n host: %s\nQPS:%d\nWorkers: %d\n", l.host, l.Qps, len(l.workers))
	pushgateway.Init()
	m = &metrics.Metrics{}
	go l.startCountdown()
	l.load()
}

func (l *Loader) startCountdown(){
	l.startProgress()
	timeout := time.After(l.Duration)
	tick := time.Tick(l.Duration/100)
	for {
		select {
		case <-timeout:
			fmt.Println("Timeout")
			stopCh <- struct{}{}
		case <-tick:
			l.Lock()
			l.calibrate()
			fmt.Printf(" >> Num of cons: %d; Req done: %d\n", m.OpenConns, m.RequestSum)
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
	if !isFlawed() && l.Qps < 10000000 {
		if len(l.jobsch) > 0 {
			go l.runWorker()
			multiplier = 1
			l.Qps = l.prevQps
			//if l.prevQps == l.Qps {
				//fmt.Printf("Eq: %f", math.Abs(1-(multiplier/100)) * 0.1)
				//multiplier -= math.Abs(1-(multiplier/100)) * 0.1

			//}
		} else {
			multiplier += (1-(multiplier/100)) * 0.1
			l.prevQps = l.Qps
		}

		fmt.Printf("[ Multiplier = %f ]", multiplier)
		l.adjustQPS()
		fmt.Println("------------")
		fmt.Printf("QPS was increased to: %d\nWorkers: %d\nJobsch len: %d\n", l.Qps, len(l.workers), len(l.jobsch))
		fmt.Println("------------")
	}
}

func (l *Loader) adjustQPS() {
	l.Qps = int(float64(l.Qps) * multiplier)
	l.throttle = time.Tick(time.Duration(1e6/(l.Qps)) * time.Microsecond)
}

const flawLimit = 10
func isFlawed() bool {
	return m.RequestSum > 0 && (int((m.Errors/m.RequestSum) * 100) > flawLimit)
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
func (l *Loader) runWorker() {
	//TODO: add support of N workers createion
	w := metrics.NewWorker(l.host, m)

	l.Lock()
	l.workers = append(l.workers, w)
	l.Unlock()

	var resp fasthttp.Response
	req := cloneRequest(l.Request)
	for range l.jobsch {
		s := time.Now()
		err := w.SendRequest(req, &resp)
		if err != nil {
			if err == fasthttp.ErrTimeout {
				atomic.AddUint64(&m.Timeouts, 1)
			}
			fmt.Printf("Err while sending req: %s", err)
			atomic.AddUint64(&m.Errors, 1)
		}
		atomic.AddUint64(&m.RequestDuration, uint64(time.Since(s)))
		atomic.AddUint64(&m.RequestSum, 1)
	}
}

const jobCapacity = 10000
func (l *Loader) load() {
	if l.Qps > 0 {
		l.throttle = time.Tick(time.Duration(1e6/(l.Qps)) * time.Microsecond)
	}

	l.jobsch = make(chan struct{}, jobCapacity)
	go l.runWorker()
	for {
		select {
		case <-stopCh:
			close(l.jobsch)
			return
		default:
			if l.Qps > 0 {
				<-l.throttle
			}
			l.jobsch <- struct{}{}
		}
	}
	close(l.jobsch)
}

func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
}
