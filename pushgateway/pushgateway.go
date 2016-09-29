package pushgateway

import (
	"flag"
	"fmt"

	"github.com/valyala/fasthttp"
)

var (
	gatewayAddr = flag.String("gatewayAddr", "localhost:9091", "Address of PushGateway service")
	jobName = flag.String("jobName", "pushGateway", "Name of the job for PushGateway")

	pushGatewayClient *fasthttp.HostClient
)

var requestURI string

func Init() {
	requestURI = fmt.Sprintf("/metrics/job/%s", *jobName)
	pushGatewayClient = &fasthttp.HostClient{
		Addr: *gatewayAddr,
	}
}

func Push() error {
	var req fasthttp.Request
	var resp fasthttp.Response

	//metrics := m.Prometheus()

	req.Header.SetMethod("POST")
	//req.SetBodyString(metrics)
	req.SetRequestURI(requestURI)
	req.Header.SetHost(*gatewayAddr)
	err := pushGatewayClient.Do(&req, &resp)
	if err != nil {
		return fmt.Errorf("error when pushing metrics: %s", err)
	}

	return nil
}