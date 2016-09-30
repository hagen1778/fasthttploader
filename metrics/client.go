package metrics

import (
	"time"
	"sync"
	"log"
	"strings"
	"strconv"
	"net"
	"sync/atomic"
	"flag"
	"io"

	"github.com/valyala/fasthttp"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpClientRequestTimeout  = flag.Duration("httpClientRequestTimeout", time.Second*10, "Maximum time to wait for http response")
	httpClientKeepAlivePeriod = flag.Duration("httpClientKeepAlivePeriod", time.Second*5, "Interval for sending keep-alive messages on keepalive connections. Zero disables keep-alive messages")
	httpClientReadBufferSize  = flag.Int("httpClientReadBufferSize", 8*1024, "Per-connection read buffer size for httpclient")
	httpClientWriteBufferSize = flag.Int("httpClientWriteBufferSize", 8*1024, "Per-connection write buffer size for httpclient")
)

const jobCapacity = 10000

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
	register()
	request = r
	host = convertHost(request)
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

	flushMetrics()
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
				timeouts.Inc()
			}
			//fmt.Printf("Err while sending req: %s", err)
			//atomic.AddUint64(&m.Errors, 1)
			errors.Inc()
		}
		requestDuration.Observe(float64(time.Since(s).Seconds()))
		requestSum.Inc()
	}
}

type hostConn struct {
	net.Conn
	addr   string
	closed uint32
	connOpen prometheus.Gauge
	readError prometheus.Counter
	writeError prometheus.Counter
	bytesWritten prometheus.Counter
	bytesRead prometheus.Counter
}

func dial(addr string) (net.Conn, error) {
	conn, err := fasthttp.DialTimeout(addr, *httpClientRequestTimeout)
	if err != nil {
		return nil, err
	}
	if err = setupTCPConn(conn); err != nil {
		connError.Inc()
		conn.Close()
		return nil, err
	}

	connOpen.Inc()
	return &hostConn{
		Conn: conn,
		addr: addr,
		connOpen: connOpen,
		readError: readError,
		writeError: writeError,
		bytesWritten: bytesWritten,
		bytesRead: bytesRead,
	}, nil
}

func setupTCPConn(conn net.Conn) error {
	c, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	var err error
	if *httpClientReadBufferSize > 0 {
		if err = c.SetReadBuffer(*httpClientReadBufferSize); err != nil {
			return err
		}
	}
	if *httpClientWriteBufferSize > 0 {
		if err = c.SetWriteBuffer(*httpClientWriteBufferSize); err != nil {
			return err
		}
	}
	if *httpClientKeepAlivePeriod > 0 {
		if err = c.SetKeepAlive(true); err != nil {
			return err
		}
		if err = c.SetKeepAlivePeriod(*httpClientKeepAlivePeriod); err != nil {
			return err
		}
	}
	return nil
}

func (hc *hostConn) Close() error {
	if atomic.AddUint32(&hc.closed, 1) == 1 {
		hc.connOpen.Dec()
	}

	return hc.Conn.Close()
}

func (hc *hostConn) Write(p []byte) (int, error) {
	n, err := hc.Conn.Write(p)
	hc.bytesWritten.Add(float64(n))
	if err != nil {
		hc.writeError.Inc()
	}
	return n, err
}

func (hc *hostConn) Read(p []byte) (int, error) {
	n, err := hc.Conn.Read(p)
	hc.bytesRead.Add(float64(n))
	if err != nil && err != io.EOF {
		hc.readError.Inc()
	}
	return n, err
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