package main

import (
	"fmt"
	"sync"
	"time"
	"strings"
	"strconv"
	"log"

	"github.com/valyala/fasthttp"
	"github.com/hagen1778/fasthttploader/metrics"
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

	host    string
}

var stopCh = make(chan struct{})
var m *metrics.M
func (l *Loader) Run() {
	l.host = convertHost(l.Request)
	pushgateway.Init()

	m = &metrics.M{}
	go l.startCountdown()
	l.runWorkers()
}

func (l *Loader) startCountdown(){
	l.startProgress()
	timeout := time.After(l.Duration)
	tick := time.Tick(l.Duration/10)
	for {
		select {
		case <-timeout:
			fmt.Println("Timeout")
			stopCh <- struct{}{}
		case <-tick:
			if err := pushgateway.Push(m); err != nil {
				fmt.Printf("%s\n", err)
			}
		}
	}

	l.finalizeProgress()
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

func (l *Loader) runWorker(ch chan struct{}) {
	w := metrics.Worker(l.host)
	var resp fasthttp.Response
	req := cloneRequest(l.Request)

	for range ch {
		s := time.Now()
		err := w.SendRequest(req, &resp)

		m.Lock()
		if err != nil {
			if err == fasthttp.ErrTimeout {
				m.Timeouts++
			}
			m.Errors++
		}
		m.RequestDuration += time.Since(s)
		m.RequestSum++
		m.Unlock()
	}
}

const jobCapacity = 10000
func (l *Loader) runWorkers() {
	var wg sync.WaitGroup
	wg.Add(10)

	var throttle <-chan time.Time
	if l.Qps > 0 {
		throttle = time.Tick(time.Duration(1e6/(l.Qps)) * time.Microsecond)
	}

	jobsch := make(chan struct{}, jobCapacity)
	for i := 0; i < 10; i++ {
		go func() {
			l.runWorker(jobsch)
			wg.Done()
		}()
	}

	for {
		select {
		case <-stopCh:
			close(jobsch)
			return
		default:
			if l.Qps > 0 {
				<-throttle
			}
			jobsch <- struct{}{}
		}
	}
	close(jobsch)
	wg.Wait()
}

func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
}
