package worker

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"bufio"
	"net"
	"sync/atomic"
	"log"
)

type worker struct{
	host string
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
}

type sender interface {
	sendRequest()
}


func NewHostWorker(host string) *worker {
	worker := HostWorker{}
	worker.host = host
	worker.OpenConnection()

	return &worker
}

type HostWorker struct {
	worker
}


//TODO: add redirect support for 301,302,303 headers
func (hw *HostWorker) sendRequest(req *fasthttp.Request, resp *fasthttp.Response) error {
	err := hw.send(req, resp)
	if err != nil || resp.ConnectionClose() {
		hw.restartConnection()
	}

	return err
}

func (hw *HostWorker) send(req *fasthttp.Request, resp *fasthttp.Response) error {
	if err := req.Write(hw.bw); err != nil {
		fmt.Printf("Write - unexpected error: %s\n", err)
		return err
	}
	if err := hw.bw.Flush(); err != nil {
		fmt.Printf("Flush - unexpected error: %s\n", err)
		return err
	}
	if err := resp.Read(hw.br); err != nil {
		fmt.Printf("Read - unexpected error: %s\n", err)
		return err
	}

	return nil
}

func (hw *HostWorker) OpenConnection() {
	conn, err := fasthttp.Dial(hw.host)
	if err != nil {
		log.Fatalf("conn error: %s\n", err)
	}
	hw.conn = &countConn{Conn: conn}

	hw.br = bufio.NewReaderSize(hw.conn, 16*1024)
	hw.bw = bufio.NewWriter(hw.conn)
}

func (hw *HostWorker) CloseConnection() {
	hw.conn.Close()
}

func (hw *HostWorker) restartConnection() {
	hw.CloseConnection()
	hw.OpenConnection()
	atomic.AddUint32(&connectionRestarts, 1)
}

type countConn struct {
	net.Conn
	writeCalls int
	readCalls  int
	bytesRead  int64
}

func (c *countConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.writeCalls++
	return n, err
}

func (c *countConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.readCalls++
	c.bytesRead += int64(n)
	return n, err
}

var (
	writeCalls         uint32
	readCalls          uint32
	bytesRead          uint64
	connectionRestarts uint32
)

func (c *countConn) Close() error {
	err := c.Conn.Close()
	atomic.AddUint32(&writeCalls, uint32(c.writeCalls))
	atomic.AddUint32(&readCalls, uint32(c.readCalls))
	atomic.AddUint64(&bytesRead, uint64(c.bytesRead))
	return err
}

