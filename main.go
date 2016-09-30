package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"regexp"
	"time"

	"github.com/valyala/fasthttp"
	"log"
	"runtime/pprof"
)

var (
	method      = flag.String("m", "GET", "")
	headers     = flag.String("h", "", "")
	body        = flag.String("b", "", "")
	accept      = flag.String("A", "", "")
	contentType = flag.String("T", "text/html", "")

	fileName = flag.String("r", "report.html", "")

	d = flag.Duration("d", 5*time.Second, "")
	t = flag.Duration("t", 5*time.Second, "")

	disableKeepAlive  = flag.Bool("k", false, "")
	disableCompression = flag.Bool("disable-compression", false, "")

	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to this file")

)

var usage = `Usage: boom [options...] <url>

Options:
  -k  disable keepalive.
  -t  request timeout. Default is equal to 5s.
`

func main(){
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var req fasthttp.Request
	header := formRequestHeader()
	header.CopyTo(&req.Header)
	req.AppendBodyString(*body)

	(&Loader{
		Request: 	&req,
		Duration:     	*d,
	}).Run()
}

const headerRegexp = "^([\\w-]+):\\s*(.+)"

func formRequestHeader() fasthttp.RequestHeader {
	var header fasthttp.RequestHeader
	var url string
	header.SetContentType(*contentType)
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
}