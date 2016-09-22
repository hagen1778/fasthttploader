package main

import (
	"github.com/valyala/fasthttp"
)

type worker struct {
	host string
	hc *fasthttp.HostClient
}

func Worker(host string) *worker {
	hc := &fasthttp.HostClient{
		Addr:         host,
	}
	w := &worker{
		hc: hc,
	}
	return w
}

func (w *worker) SendRequest(req *fasthttp.Request, resp *fasthttp.Response) (err error) {
	return w.hc.Do(req, resp)
}