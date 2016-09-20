package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/valyala/fasthttp"
)

const headerRegexp = "^([\\w-]+):\\s*(.+)"

var (
	m           = flag.String("m", "GET", "")
	headers     = flag.String("h", "", "")
	body        = flag.String("d", "", "")
	accept      = flag.String("A", "", "")
	contentType = flag.String("T", "text/html", "")

	c = flag.Int("c", 50, "")
	n = flag.Int("n", 200, "")
	q = flag.Int("q", 0, "")

	disableKeepAlive  = flag.Bool("k", false, "")
	enablePipeline  = flag.Bool("p", false, "")
	disableCompression = flag.Bool("disable-compression", false, "")
)

var usage = `Usage: boom [options...] <url>

Options:
  -n  Number of requests to run.
  -c  Number of requests to run concurrently. Total number of requests cannot
      be smaller than the concurency level.
  -q  Rate limit, in seconds (QPS).
  -k  disable keepalive.
  -p  use pipeline

  -m  HTTP method, one of GET, POST, PUT, DELETE, HEAD, OPTIONS.
  -h  Custom HTTP headers, name1:value1;name2:value2.
  -A  HTTP Accept header.
  -d  HTTP request body.
  -T  Content-type, defaults to "text/html".

  -disable-compression  Disable compression.
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	num := *n
	conc := *c
	q := *q

	if num <= 0 || conc <= 0 {
		usageAndExit("n and c cannot be smaller than 1.")
	}

	var (
		url, method string
		header      fasthttp.RequestHeader
	)

	url = flag.Args()[0]
	method = strings.ToUpper(*m)

	// set content-type
	header.SetContentType(*contentType)
	// set any other additional headers
	if *headers != "" {
		headers := strings.Split(*headers, ";")
		for _, h := range headers {
			match, err := parseInputWithRegexp(h, headerRegexp)
			if err != nil {
				usageAndExit(err.Error())
			}
			header.Set(match[1], match[2])
		}
	}
	if *accept != "" {
		header.Set("Accept", *accept)
	}
	header.SetMethod(method)
	header.SetRequestURI(url)
	if !*disableCompression {
		header.Set("Accept-Encoding", "gzip")
	}

	if *disableKeepAlive && !*enablePipeline {
		header.Set("Connection", "close")
	} else {
		header.Set("Connection", "keep-alive")
	}

	var req fasthttp.Request
	header.CopyTo(&req.Header)
	req.AppendBodyString(*body)


	(&Loader{
		Request: &req,
		N:       num,
		C:       conc,
		Qps:     q,
	}).Run()
}

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, msg)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}

func parseInputWithRegexp(input, regx string) ([]string, error) {
	re := regexp.MustCompile(regx)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 1 {
		return nil, fmt.Errorf("could not parse the provided input; input = %v", input)
	}
	return matches, nil
}
