package fastclient

import (
	"flag"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/fasthttp"
)

var (
	httpClientRequestTimeout  = flag.Duration("httpClientRequestTimeout", time.Second*10, "Maximum time to wait for http response")
	httpClientKeepAlivePeriod = flag.Duration("httpClientKeepAlivePeriod", time.Second*5, "Interval for sending keep-alive messages"+
		"on keepalive connections. Zero disables keep-alive messages")
	httpClientReadBufferSize  = flag.Int("httpClientReadBufferSize", 8*1024, "Per-connection read buffer size for httpclient")
	httpClientWriteBufferSize = flag.Int("httpClientWriteBufferSize", 8*1024, "Per-connection write buffer size for httpclient")
)

const (
	jobCapacity         = 10000
	maxIdleConnDuration = time.Second
	maxConns            = 1<<31 - 1
)

func init() {
	registerMetrics()
}

// Client is a wrapper for fasthttp.HostClient
// It allows to send requests and collect metrics while sending
type Client struct {
	// Jobsch is a channel of tasks(requests) which should be done
	Jobsch chan struct{}

	*fasthttp.HostClient
	wg                sync.WaitGroup
	request           *fasthttp.Request
	successStatusCode int

	sync.Mutex
	workers          int
	statusCodeLabels map[int]prometheus.Labels
	errorMessages    map[string]prometheus.Labels
}

// New creates new client
func New(request *fasthttp.Request, timeout time.Duration, sc int) *Client {
	flushMetrics()
	addr, isTLS := acquireAddr(request)
	return &Client{
		Jobsch:            make(chan struct{}, jobCapacity),
		request:           request,
		statusCodeLabels:  make(map[int]prometheus.Labels),
		errorMessages:     make(map[string]prometheus.Labels),
		successStatusCode: sc,
		HostClient: &fasthttp.HostClient{
			Addr:                addr,
			IsTLS:               isTLS,
			Dial:                dial,
			MaxIdleConnDuration: maxIdleConnDuration,
			MaxConns:            maxConns,
			ReadTimeout:         timeout,
			WriteTimeout:        timeout,
		},
	}
}

// Amount return number of created workers
// after Flush() workers would flushed too
func (c *Client) Amount() int {
	c.Lock()
	defer c.Unlock()

	return c.workers
}

// Overflow return length of job-channel
// after calling Flush(), channel would flushed too
func (c *Client) Overflow() int {
	c.Lock()
	defer c.Unlock()

	return len(c.Jobsch)
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

// Flush dropping out and re-init all counters and metrics
func (c *Client) Flush() {
	drainChan(c.Jobsch)
	close(c.Jobsch)
	c.wg.Wait()
	flushMetrics()
	c.workers = 0
	c.Jobsch = make(chan struct{}, jobCapacity)
}

// RunWorkers runs n goroutines to serve jobs from Jobsch
func (c *Client) RunWorkers(n int) {
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		c.wg.Add(1)
		go func() {
			c.Lock()
			c.workers++
			c.Unlock()

			c.run()
			c.wg.Done()
		}()
	}
}

func (c *Client) run() {
	var resp fasthttp.Response
	r := new(fasthttp.Request)
	c.request.CopyTo(r)
	for range c.Jobsch {
		s := time.Now()
		err := c.Do(r, &resp)
		if err != nil {
			if err == fasthttp.ErrTimeout {
				timeouts.Inc()
			}
			errors.Inc()
			c.withErrorMessage(err.Error()).Inc()
		}

		sc := resp.StatusCode()
		if c.successStatusCode == sc {
			requestSuccess.Inc()
		}

		c.withStatusCode(sc).Inc()
		requestDuration.Observe(float64(time.Since(s).Seconds()))
		requestSum.Inc()
	}
}

func (c *Client) withStatusCode(code int) prometheus.Counter {
	var label prometheus.Labels
	var ok bool
	c.Lock()
	if label, ok = c.statusCodeLabels[code]; !ok {
		label = prometheus.Labels{"code": strconv.Itoa(code)}
		c.statusCodeLabels[code] = label
	}
	c.Unlock()
	return statusCodes.With(label)
}

func (c *Client) withErrorMessage(msg string) prometheus.Counter {
	var label prometheus.Labels
	var ok bool
	c.Lock()
	if label, ok = c.errorMessages[msg]; !ok {
		label = prometheus.Labels{"message": msg}
		c.errorMessages[msg] = label
	}
	c.Unlock()
	return errorMessages.With(label)
}

type hostConn struct {
	net.Conn
	addr         string
	closed       uint32
	connOpen     prometheus.Gauge
	readError    prometheus.Counter
	writeError   prometheus.Counter
	bytesWritten prometheus.Counter
	bytesRead    prometheus.Counter
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
		Conn:         conn,
		addr:         addr,
		connOpen:     connOpen,
		readError:    readError,
		writeError:   writeError,
		bytesWritten: bytesWritten,
		bytesRead:    bytesRead,
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

func acquireAddr(req *fasthttp.Request) (string, bool) {
	addr := string(req.URI().Host())
	if len(addr) == 0 {
		log.Fatalf("address cannot be empty")
	}
	isTLS := string(req.URI().Scheme()) == "https"
	tmp := strings.SplitN(addr, ":", 2)
	if len(tmp) != 2 {
		port := ":80"
		if isTLS {
			port = ":443"
		}
		return tmp[0] + port, isTLS
	}
	port := tmp[1]
	portInt, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("cannot parse port %q of addr %q: %s", port, addr, err)
	}
	if portInt < 0 {
		log.Fatalf("upstreamHosts port %d cannot be negative: %q", portInt, addr)
	}

	return addr, isTLS
}
