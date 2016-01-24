// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package boomer provides commands to run load tests and display results.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"sync/atomic"
)

type result struct {
	err           error
	statusCode    int
	duration      time.Duration
	contentLength int64
}

type worker struct {
	host string
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
}

type Boomer struct {
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

	// Output represents the output type. If "csv" is provided, the
	// output will be dumped as a csv stream.
	Output string

	bar     *pBar
	host    string
	results []result
	idx     uint64
}

func (b *Boomer) startProgress() {
	if b.Output != "" {
		return
	}

	b.bar = newBar(b.N)
}

func (b *Boomer) finalizeProgress() {
	if b.Output != "" {
		return
	}
	b.bar.fin()
}

func (b *Boomer) incProgress(idx uint64) {
	if b.Output != "" {
		return
	}
	b.bar.inc()
}

// Run makes all the requests, prints the summary. It blocks until
// all work is done.
func (b *Boomer) Run() {
	start := time.Now()
	b.results = make([]result, b.N)
	//TODO: check possibility to achieve real port
	b.host = string(b.Request.URI().Host()) + ":80"
	b.startProgress()

	b.runWorkers()
	b.finalizeProgress()
	newReport(b.N, b.results, b.Output, time.Now().Sub(start)).finalize()
}

//TODO: add redirect support for 301,302,303 headers
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
}

func NewWorker(host string) *worker {
	if host == "" {
		return nil
	}

	worker := worker{host: host}
	worker.openConnection()

	return &worker
}

func (w *worker) openConnection() {
	conn, err := fasthttp.Dial(w.host)
	if err != nil {
		fmt.Printf("conn error: %s\n", err)
		os.Exit(1)
	}
	w.conn = &countConn{Conn: conn}

	w.br = bufio.NewReaderSize(w.conn, 16*1024)
	w.bw = bufio.NewWriter(w.conn)
}

func (w *worker) closeConnection() {
	w.conn.Close()
}

func (w *worker) restartConnection() {
	w.closeConnection()
	w.openConnection()
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

func (b *Boomer) runWorker(ch <-chan struct{}) {
	worker := NewWorker(b.host)
	defer worker.closeConnection()
	var resp fasthttp.Response
	req := cloneRequest(b.Request)

	for range ch {
		s := time.Now()
		var (
			code int
			size int64
		)

		err := worker.sendRequest(req, &resp)
		if err == nil {
			size = int64(resp.Header.ContentLength())
			code = resp.StatusCode()
		}

		idx := atomic.AddUint64(&b.idx, 1)
		if idx > b.bar.Grade {
			b.incProgress(idx)
		}

		r := &b.results[idx-1]
		r.statusCode = code
		r.duration = time.Since(s)
		r.err = err
		r.contentLength = size
	}
}

func (b *Boomer) runWorkers() {
	var wg sync.WaitGroup
	wg.Add(b.C)

	var throttle <-chan time.Time
	if b.Qps > 0 {
		throttle = time.Tick(time.Duration(1e6/(b.Qps)) * time.Microsecond)
	}

	jobsch := make(chan struct{}, b.N)
	for i := 0; i < b.C; i++ {
		go func() {
			b.runWorker(jobsch)
			wg.Done()
		}()
	}

	for i := 0; i < b.N; i++ {
		if b.Qps > 0 {
			<-throttle
		}
		jobsch <- struct{}{}
	}
	close(jobsch)
	wg.Wait()
}

func cloneRequest(r *fasthttp.Request) *fasthttp.Request {
	r2 := new(fasthttp.Request)
	r.Header.CopyTo(&r2.Header)
	r2.AppendBody(r.Body())
	return r2
}
