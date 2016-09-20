package worker

import (
	"github.com/valyala/fasthttp"
	"bufio"
	"net"
	"log"
)

type worker struct{
	host string
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
}

type Sender interface {
	SendRequest(req *fasthttp.Request, resp *fasthttp.Response) error
	CloseConnection()
}

type HostWorker struct {
	worker
}

func NewHostWorker(host string) Sender {
	worker := HostWorker{}
	worker.host = host
	worker.OpenConnection()

	return &worker
}

//TODO: add redirect support for 301,302,303 headers
func (hw *HostWorker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) error {
	err := hw.send(req, resp)
	if err != nil || resp.ConnectionClose() {
		hw.restartConnection()
	}

	return err
}

func (hw *HostWorker) send(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
	if err = req.Write(hw.bw); err != nil {
		return
	}
	if err = hw.bw.Flush(); err != nil {
		return
	}
	if err = resp.Read(hw.br); err != nil {
		return
	}

	return nil
}

func (hw *HostWorker) OpenConnection() {
	conn, err := fasthttp.Dial(hw.host)
	if err != nil {
		log.Fatalf("conn error: %s; adress: %s\n", err, hw.host)
	}
	hw.conn = conn

	hw.br = bufio.NewReaderSize(hw.conn, 16*1024)
	hw.bw = bufio.NewWriter(hw.conn)
}

func (hw *HostWorker) CloseConnection() {
	hw.conn.Close()
}

func (hw *HostWorker) restartConnection() {
	hw.CloseConnection()
	hw.OpenConnection()
}


type PipelineWorker struct {
	worker
	pc *fasthttp.PipelineClient
}

func NewPipelineWorker(host string) Sender {
	pc := &fasthttp.PipelineClient{
		Addr:               host,
	}
	pw := &PipelineWorker{
		pc: pc,
	}
	pw.host = host
	return pw

}

func (pw *PipelineWorker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
	return pw.pc.Do(req, resp)
}

func (pw *PipelineWorker) CloseConnection() {
}