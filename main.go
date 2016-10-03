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
	method      = flag.String("m", "GET", "Set HTTP method")
	headers     = flag.String("h", "", "Set headers")
	body        = flag.String("b", "", "Set body")
	accept      = flag.String("A", "", "Set Accept headers")
	contentType = flag.String("T", "text/html", "Set content-type headers")

	fileName = flag.String("r", "report.html", "Set filename to store final report")

	d = flag.Duration("d", 20*time.Second, "Cant be less than 20sec")
	t = flag.Duration("t", 5*time.Second, "Request timeout")
	q = flag.Int("q", 0, "Request per second limit. Detect automatically, if not setted")
	cl = flag.Int("c", 500, "Number of supposed clients")

	debug  = flag.Bool("debug", false, "Print debug messages if true")
	disableKeepAlive  = flag.Bool("k", false, "Disable keepalive if true")
	disableCompression = flag.Bool("disable-compression", false, "Disables compression if true")

	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to this file")

)

var usage = `Usage: fasthttploader [options...] <url>
Notice: fasthttploader would force agressive burst stages before testing to detect max qps and number for clients.
To avoid this you need to set -c and -q parameters.
Options:
`

func main(){
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
		os.Exit(1)
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