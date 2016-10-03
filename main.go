package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"regexp"
	"time"
	"log"
	"runtime/pprof"

	"github.com/valyala/fasthttp"
	"golang.org/x/time/rate"
)

var (
	method      = flag.String("m", "GET", "")
	headers     = flag.String("h", "", "")
	body        = flag.String("b", "", "")
	accept      = flag.String("A", "", "")
	contentType = flag.String("T", "text/html", "")

	fileName = flag.String("r", "report.html", "")

	d = flag.Duration("d", 20*time.Second, "Cant be less than 20sec")
	t = flag.Duration("t", 5*time.Second, "Request timeout")
	q = flag.Int("q", 0, "Request per second limit. Detect automatically, if not setted")
	cl = flag.Int("c", 500, "Number of supposed clients")

	debug  = flag.Bool("debug", false, "Print debug messages if true")
	disableKeepAlive  = flag.Bool("k", false, "")
	disableCompression = flag.Bool("disable-compression", false, "")

	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to this file")

)

var usage = `Usage: fasthttploader [options...] <url>
Notice: fasthttploader would force agressive burst stages before testing to detect max qps and number for clients.
To avoid this you need to set -c and -q in
Options:
  -q  Request per second limit. Detect automatically, if not setted
  -d  Test duration. Cannot be less than 20s.
  -c  Supposed number of clients could be handled by tested service. Default is equal to 500
  -t  Request timeout. Default is equal to 5s.

  -k  Disable keepalive.
  -disable-compression Disables compression if true
  -debug Print debug messages if true
`
// TODO: add support of https
func main(){
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	if *d < time.Second*20 {
		usageAndExit("Duratiion cant be less than 20s")
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
		C:		*cl,
		Qps:		rate.Limit(*q),
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