package metrics

import (
	"sync/atomic"
	"time"
	"sync"
	"log"
	"strings"
	"strconv"

	"github.com/valyala/fasthttp"
)

const jobCapacity = 10000

var m *Metrics

type Client struct {
	Jobsch chan struct{}

	workers []*worker
	wg sync.WaitGroup

	sync.Mutex
}

var (
	request *fasthttp.Request
	host string
	t time.Duration
)

func Init(r *fasthttp.Request, timeout time.Duration) *Client {
	request = r
	host = convertHost(request)
	m = &Metrics{}
	t = timeout

	return &Client{
		Jobsch: make(chan struct{}, jobCapacity),
	}
}

func (c *Client) Amount() int {
	c.Lock()
	defer c.Unlock()

	return len(c.workers)
}

func (c *Client) Metrics() *Metrics {
	return m
}

func drainChan(ch chan struct{}) {
	for {
		select {
		case <-ch:
			continue
		default:
			return
		}
	}
}

func (c *Client) Flush() {
	c.Lock()
	defer c.Unlock()

	drainChan(c.Jobsch)
	close(c.Jobsch)

	c.wg.Wait()
	time.Sleep(2*t)

	m = &Metrics{}
	c.workers = c.workers[:0]
	c.Jobsch = make(chan struct{}, jobCapacity)
}

func (c *Client) AddWorkers(n int) {
	for i := 0; i < n; i++ {
		c.wg.Add(1)
		go c.AddWorker()
	}
}

func (c *Client) AddWorker() {
	hc := &fasthttp.HostClient{
		Addr:   host,
		Dial:	dial,
	}
	w := &worker{
		hc,
	}
	c.Lock()
	c.workers = append(c.workers, w)
	c.Unlock()

	w.run(c.Jobsch)
	c.wg.Done()
}

type worker struct{
	*fasthttp.HostClient
}

func (w *worker) run(ch chan struct{}) {
	var resp fasthttp.Response
	r := cloneRequest(request)
	for range ch {
		s := time.Now()
		err := w.DoTimeout(r, &resp, t)
		if err != nil {
			if err == fasthttp.ErrTimeout {
				atomic.AddUint64(&m.Timeouts, 1)
			}
			//fmt.Printf("Err while sending req: %s", err)
			atomic.AddUint64(&m.Errors, 1)
		}
		//fmt.Println(resp.String())
		//os.Exit(1)
		atomic.AddUint64(&m.RequestDuration, uint64(time.Since(s)))
		atomic.AddUint64(&m.RequestSum, 1)
	}
}

func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
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