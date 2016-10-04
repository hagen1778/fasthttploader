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
	c = flag.Int("c", 500, "Number of supposed clients")

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

var req = new(fasthttp.Request)

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

	applyHeaders()
	req.AppendBodyString(*body)
	run()

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
		return
	}
}

var re = regexp.MustCompile("^([\\w-]+):\\s*(.+)")

func applyHeaders() {
	var url string
	req.Header.SetContentType(*contentType)
	if *headers != "" {
		headers := strings.Split(*headers, ";")
		for _, h := range headers {
			matches := re.FindStringSubmatch(h)
			if len(matches) < 1 {
				usageAndExit(fmt.Sprintf("could not parse the provided input; input = %v", h))
			}
			req.Header.Set(matches[1], matches[2])
		}
	}
	if *accept != "" {
		req.Header.Set("Accept", *accept)
	}
	url = flag.Args()[0]
	req.Header.SetMethod(strings.ToUpper(*method))
	req.Header.SetRequestURI(url)
	if !*disableCompression {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	if *disableKeepAlive {
		req.Header.Set("Connection", "close")
	} else {
		req.Header.Set("Connection", "keep-alive")
	}
}

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, msg)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
}