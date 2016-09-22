package main

import (
	"flag"
	"github.com/valyala/fasthttp"
	"fmt"
)

var (
	gatewayAddr = flag.String("gatewayAddr", "localhost:9091", "Address of PushGateway service")
	jobName = flag.String("jobName", "pushGateway", "Name of the job for PushGateway")

	pushGatewayClient *fasthttp.HostClient
)

func init() {
	pushGatewayClient = &fasthttp.HostClient{
		Addr: *gatewayAddr,
	}
}

func push() error {
	var req fasthttp.Request
	var resp fasthttp.Response

	r.Lock()
	r2 := r
	r.Unlock()

	req.Header.SetMethod("POST")
	req.SetBodyString(r2.Prometheus())
	req.SetRequestURI(fmt.Sprintf("/metrics/job/%s", *jobName))
	req.Header.SetHost(*gatewayAddr)
	err := pushGatewayClient.Do(&req, &resp)
	if err != nil {
		return fmt.Errorf("error when pushing metrics: %s", err)
	}

	return nil
}