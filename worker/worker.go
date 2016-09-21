package worker

import (
	"bufio"
	"net"
	"log"

	"github.com/valyala/fasthttp"
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

type hostWorker struct {
	worker
}

func NewHostWorker(host string) Sender {
	worker := hostWorker{}
	worker.host = host
	worker.OpenConnection()

	return &worker
}

//TODO: add redirect support for 301,302,303 headers
func (hw *hostWorker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) error {
	err := hw.send(req, resp)
	if err != nil || resp.ConnectionClose() {
		hw.restartConnection()
	}

	return err
}

func (hw *hostWorker) send(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
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

func (hw *hostWorker) OpenConnection() {
	conn, err := fasthttp.Dial(hw.host)
	if err != nil {
		log.Fatalf("conn error: %s; adress: %s\n", err, hw.host)
	}
	hw.conn = conn

	hw.br = bufio.NewReaderSize(hw.conn, 16*1024)
	hw.bw = bufio.NewWriter(hw.conn)
}

func (hw *hostWorker) CloseConnection() {
	hw.conn.Close()
}

func (hw *hostWorker) restartConnection() {
	hw.CloseConnection()
	hw.OpenConnection()
}


type pipelineWorker struct {
	worker
	pc *fasthttp.PipelineClient
}

func NewPipelineWorker(host string) Sender {
	pc := &fasthttp.PipelineClient{
		Addr:               host,
	}
	pw := &pipelineWorker{
		pc: pc,
	}
	pw.host = host
	return pw

}

func (pw *pipelineWorker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
	return pw.pc.Do(req, resp)
}

func (pw *pipelineWorker) CloseConnection() {
}