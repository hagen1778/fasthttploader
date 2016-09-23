package metrics

import (
	"time"
	"sync"
	"sync/atomic"
	"net"
	"flag"
	"io"

	"github.com/valyala/fasthttp"
)

var (
	httpClientRequestTimeout  = flag.Duration("httpClientRequestTimeout", time.Second*10, "Maximum time to wait for http response")
	httpClientKeepAlivePeriod = flag.Duration("httpClientKeepAlivePeriod", 0, "Interval for sending keep-alive messages on keepalive connections. Zero disables keep-alive messages")
	httpClientReadBufferSize  = flag.Int("httpClientReadBufferSize", 8*1024, "Per-connection read buffer size for httpclient")
	httpClientWriteBufferSize = flag.Int("httpClientWriteBufferSize", 8*1024, "Per-connection write buffer size for httpclient")
)

type Metrics struct {
	Connections	int
	Timeouts	int
	Errors		int
	RequestSum	int
	RequestDuration	time.Duration

	connStats

	sync.Mutex
}

type connStats struct {
	ConnectError uint64
	OpenConns    uint64
	BytesWritten uint64
	BytesRead    uint64
	WriteError   uint64
	ReadError    uint64
}

type worker struct {
	host string
	hc *fasthttp.HostClient
}

var m *Metrics

func Worker(host string, metric *Metrics) *worker {
	if m == nil {
		m = metric
	}

	hc := &fasthttp.HostClient{
		Addr:   host,
		Dial:	dial,
	}
	w := &worker{
		hc: hc,
	}
	return w
}

func (w *worker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
	//TODO: rm, if nothing additional will not appear
	return w.hc.Do(req, resp)
}

type hostConn struct {
	net.Conn
	addr   string
	closed uint32
}

func dial(addr string) (net.Conn, error) {
	conn, err := fasthttp.DialTimeout(addr, *httpClientRequestTimeout)
	if err != nil {
		return nil, err
	}
	if err = setupTCPConn(conn); err != nil {
		atomic.AddUint64(&m.ConnectError, 1)
		conn.Close()
		return nil, err
	}
	atomic.AddUint64(&m.OpenConns, 1)
	return &hostConn{
		Conn: conn,
		addr: addr,
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

func (c *hostConn) Close() error {
	atomic.AddUint64(&m.OpenConns, ^uint64(0))
	return c.Conn.Close()
}

func (c *hostConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	atomic.AddUint64(&m.BytesWritten, uint64(n))
	if err != nil {
		atomic.AddUint64(&m.WriteError, 1)
	}
	return n, err
}

func (c *hostConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	atomic.AddUint64(&m.BytesRead, uint64(n))
	if err != nil && err != io.EOF {
		atomic.AddUint64(&m.ReadError, 1)
	}
	return n, err
}