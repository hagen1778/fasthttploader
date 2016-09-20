package main

import (
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/rakyll/pb"
	"sync/atomic"

	"github.com/hagen1778/fasthttploader/worker"
	"strings"
	"strconv"
	"log"
)

type result struct {
	err           error
	statusCode    int
	duration      time.Duration
	contentLength int64
}

type Loader struct {
	// Request is the request to be made.
	Request *fasthttp.Request

	// N is the total number of requests to make.
	N int

	// C is the concurrency level, the number of concurrent workers to run.
	C int

	// Timeout in seconds.
	Timeout int

	// Qps is the rate limit.
	Qps int

	bar     *pb.ProgressBar
	host    string
	results []result
	idx     uint64
}

func (l *Loader) startProgress() {
	l.bar = pb.New(l.N)
	l.bar.Start()
}

func (l *Loader) finalizeProgress() {
	l.bar.Finish()
}

func (l *Loader) inc() {
	l.bar.Increment()
}

// Run makes all the requests, prints the summary. It blocks until
// all work is done.
func (l *Loader) Run() {
	start := time.Now()
	l.results = make([]result, l.N)
	l.host = convertHost(l.Request)
	l.startProgress()

	l.runWorkers()

	l.finalizeProgress()
	newReport(l.N, l.results, time.Now().Sub(start)).finalize()
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

func (l *Loader) runWorker(ch <-chan struct{}) {
	var w worker.Sender
	if *enablePipeline{
		w = worker.NewPipelineWorker(l.host)
	} else {
		w = worker.NewHostWorker(l.host)
	}

	defer w.CloseConnection()
	var resp fasthttp.Response
	req := cloneRequest(l.Request)

	for range ch {
		s := time.Now()

		err := w.SendRequest(req, &resp)
		idx := atomic.AddUint64(&l.idx, 1)
		l.inc()

		r := &l.results[idx-1]
		if err == nil {
			r.statusCode = resp.StatusCode()
			r.contentLength = int64(resp.Header.ContentLength())
		}
		r.duration = time.Since(s)
		r.err = err
	}
}

func (l *Loader) runWorkers() {
	var wg sync.WaitGroup
	wg.Add(l.C)

	var throttle <-chan time.Time
	if l.Qps > 0 {
		throttle = time.Tick(time.Duration(1e6/(l.Qps)) * time.Microsecond)
	}

	jobsch := make(chan struct{}, l.N)
	for i := 0; i < l.C; i++ {
		go func() {
			l.runWorker(jobsch)
			wg.Done()
		}()
	}

	for i := 0; i < l.N; i++ {
		if l.Qps > 0 {
			<-throttle
		}
		jobsch <- struct{}{}
	}
	close(jobsch)
	wg.Wait()
}

/*//TODO: add redirect support for 301,302,303 headers
func (w *worker) sendRequest(req *fasthttp.Request, resp *fasthttp.Response) error {
	err := w.send(req, resp)
	if err != nil || resp.ConnectionClose() {
		w.restartConnection()
	}

	return err
}

func (w *worker) send(req *fasthttp.Request, resp *fasthttp.Response) error {
	if err := req.Write(w.bw); err != nil {
		fmt.Printf("Write - unexpected error: %s\n", err)
		return err
	}
	if err := w.bw.Flush(); err != nil {
		fmt.Printf("Flush - unexpected error: %s\n", err)
		return err
	}
	if err := resp.Read(w.br); err != nil {
		fmt.Printf("Read - unexpected error: %s\n", err)
		return err
	}

	return nil
}*/


func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
}
