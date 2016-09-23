package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"regexp"
	"time"

	"github.com/valyala/fasthttp"
)

var (
	method      = flag.String("m", "GET", "")
	headers     = flag.String("h", "", "")
	body        = flag.String("b", "", "")
	accept      = flag.String("A", "", "")
	contentType = flag.String("T", "text/html", "")

	q = flag.Int("q", 100, "")
	d = flag.Duration("d", 5*time.Second, "")

	disableKeepAlive  = flag.Bool("k", false, "")
	disableCompression = flag.Bool("disable-compression", false, "")
)

var usage = `Usage: boom [options...] <url>

Options:
  -q  Rate limit, in seconds (QPS).
  -k  disable keepalive.
`

func main(){
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	var req fasthttp.Request
	header := formRequestHeader()
	header.CopyTo(&req.Header)
	req.AppendBodyString(*body)

	(&Loader{
		Request: 	&req,
		Qps:     	*q,
		Duration:     	*d,
	}).Run()
}

const headerRegexp = "^([\\w-]+):\\s*(.+)"

func formRequestHeader() fasthttp.RequestHeader {
	var header fasthttp.RequestHeader
	var url string
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
	url = flag.Args()[0]
	header.SetMethod(strings.ToUpper(*method))
	header.SetRequestURI(url)
	if !*disableCompression {
		header.Set("Accept-Encoding", "gzip")
	}
	if *disableKeepAlive {
		header.Set("Connection", "close")
	} else {
		header.Set("Connection", "keep-alive")
	}

	return header
}

func parseInputWithRegexp(input, regx string) ([]string, error) {
	re := regexp.MustCompile(regx)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 1 {
		return nil, fmt.Errorf("could not parse the provided input; input = %v", input)
	}
	return matches, nil
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