package metrics

import (
	"time"
	"sync/atomic"
	"net"
	"flag"
	"io"

	"github.com/valyala/fasthttp"
)

var (
	httpClientRequestTimeout  = flag.Duration("httpClientRequestTimeout", time.Second*10, "Maximum time to wait for http response")
	httpClientKeepAlivePeriod = flag.Duration("httpClientKeepAlivePeriod", time.Second*5, "Interval for sending keep-alive messages on keepalive connections. Zero disables keep-alive messages")
	httpClientReadBufferSize  = flag.Int("httpClientReadBufferSize", 8*1024, "Per-connection read buffer size for httpclient")
	httpClientWriteBufferSize = flag.Int("httpClientWriteBufferSize", 8*1024, "Per-connection write buffer size for httpclient")
)

type Metrics struct {
	Timeouts	uint64
	Errors		uint64
	RequestSum	uint64
	RequestDuration	uint64

	connStats
}

type connStats struct {
	ConnectError uint64
	OpenConns    uint64
	BytesWritten uint64
	BytesRead    uint64
	WriteError   uint64
	ReadError    uint64
}

type hostConn struct {
	net.Conn
	addr   string
	closed uint32
	m *Metrics
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
		m: m,
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
		atomic.AddUint64(&hc.m.OpenConns, ^uint64(0))
	}

	return hc.Conn.Close()
}

func (hc *hostConn) Write(p []byte) (int, error) {
	n, err := hc.Conn.Write(p)
	atomic.AddUint64(&hc.m.BytesWritten, uint64(n))
	if err != nil {
		atomic.AddUint64(&hc.m.WriteError, 1)
	}
	return n, err
}

func (hc *hostConn) Read(p []byte) (int, error) {
	n, err := hc.Conn.Read(p)
	atomic.AddUint64(&hc.m.BytesRead, uint64(n))
	if err != nil && err != io.EOF {
		atomic.AddUint64(&hc.m.ReadError, 1)
	}
	return n, err
}
