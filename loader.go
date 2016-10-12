package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/hagen1778/fasthttploader/fastclient"
	"github.com/hagen1778/fasthttploader/pushgateway"
	"github.com/hagen1778/fasthttploader/ratelimiter"
	"github.com/hagen1778/fasthttploader/report"
)

const (
	// Duration of burst-testing, without qps-limit. Used to estimate start test conditions
	calibrateDuration = 5 * time.Second

	// Duration of adjustable testing, while trying to reach max qps with minimal lvl of errors
	adjustmentDuration = 30 * time.Second

	// Period of sample taking, while testing
	samplePeriod = 500 * time.Millisecond
)

var (
	// client do http requests, populate metrics
	client *fastclient.Client

	// report represent html-report
	r *report.Page

	// errors storage of errors amount in current step. Used to compare changes in errors-metric
	errors uint64

	// multiplier is a coefficient of qps multiplying during tests
	multiplier = float64(0.1)

	throttle = ratelimiter.NewLimiter()
)

type loadConfig struct {
	// qps is the rate limit.
	qps float64

	// c is a number of workers (clients)
	c int
}

func run() {
	client = fastclient.New(req, *t, *successStatusCode)
	pushgateway.Init()
	r = &report.Page{
		Title:           string(req.URI().Host()),
		RequestDuration: make(map[float64][]float64),
		Interval:        samplePeriod.Seconds(),
	}

	cfg := loadConfig{}
	if *q == 0 {
		fmt.Println("Run burst-load phase")
		burstThroughput(&cfg)

		fmt.Println("Run calibrate phase")
		calibrateThroughput(&cfg)
	} else {
		cfg.qps = float64(*q)
		cfg.c = *c
	}

	fmt.Println("Run load phase")
	makeLoad(&cfg)

	f, err := os.Create(*fileName)
	if err != nil {
		log.Fatalf("Error while trying to create file: %s", err)
	}
	defer f.Close()

	f.WriteString(report.PrintPage(r))
}

func burstThroughput(cfg *loadConfig) {
	startTime := time.Now()
	timeout := time.After(calibrateDuration)
	bar, progressTicker := acquireProgressBar(calibrateDuration)

	client.RunWorkers(*c)
	for {
		select {
		case <-timeout:
			finishProgressBar(bar)
			cfg.qps = float64(client.RequestSum()) / calibrateDuration.Seconds()
			cfg.c = client.Amount()
			if (client.Errors()/client.RequestSum())*100 > 2 { // just more than 2% of errors
				cfg.qps /= 2
				cfg.c /= 2
			}
			printSummary("Burst Throughput", startTime)
			client.Flush()
			return
		case <-progressTicker:
			bar.Increment()
		default:
			client.Jobsch <- struct{}{}
		}
	}
}

func calibrateThroughput(cfg *loadConfig) {
	t := time.Now()
	ctx, cancel := context.WithCancel(context.Background())

	throttle.SetLimit(cfg.qps)
	client.RunWorkers(cfg.c)
	go func() {
		timeout := time.After(adjustmentDuration)
		sampler := time.Tick(samplePeriod)
		bar, progressTicker := acquireProgressBar(adjustmentDuration)
		for {
			select {
			case <-timeout:
				cancel()
				finishProgressBar(bar)
				cfg.qps = throttle.Limit()
				cfg.c = client.Amount()
				printSummary("Adjustment test", t)
				return
			case <-progressTicker:
				bar.Increment()
			case <-sampler:
				printState()
				calibrate()
			}
		}
	}()

	load(ctx)
}

var await = 0

func calibrate() {
	if await > 0 {
		await -= 1
		return
	}

	if !isFlawed() {
		if client.Overflow() > 0 {
			n := int(float64(client.Amount()) * multiplier)
			client.RunWorkers(n)
			await += 1
		} else {
			throttle.SetLimit(throttle.Limit() * (1 + multiplier))
			await += 1
		}
	} else {
		multiplier /= 1.2
		await += 3
	}
}

func makeLoad(cfg *loadConfig) {
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())

	workerStep := cfg.c / 10
	qpsStep := cfg.qps / 10
	throttle.SetLimit(qpsStep)
	client.RunWorkers(workerStep)
	go func() {
		stepTick := time.Tick(time.Second)
		stateTick := time.Tick(samplePeriod)
		timeout := time.After(*d)
		steps := 0
		bar, progressTicker := acquireProgressBar(*d)
		for {
			select {
			case <-timeout:
				cancel()
				finishProgressBar(bar)
				printSummary("Loading test", startTime)
				throttle.Stop()
				return
			case <-stepTick:
				if steps >= 10-1 {
					continue
				}
				throttle.SetLimit(throttle.Limit() + qpsStep)
				client.RunWorkers(workerStep)
				steps++
			case <-progressTicker:
				bar.Increment()
			case <-stateTick:
				printState()

				//if err := pushgateway.Push(c.Metrics()); err != nil {
				//	fmt.Printf("%s\n", err)
				//}
			}
		}
	}()
	load(ctx)
}

func printState() {
	if *debug {
		fmt.Println("------------")
		fmt.Printf("[ Multiplier = %f ]\n", multiplier)
		fmt.Printf("QPS was increased to: %f\nWorkers: %d\nJobsch len: %d\n", throttle.Limit(), client.Amount(), client.Overflow())
		fmt.Printf(" >> Num of cons: %d; Req done: %d; Errors: %d; Timeouts: %d\n", client.ConnOpen(), client.RequestSum(), client.Errors(), client.Timeouts())
		fmt.Println("------------")
	}

	r.Lock()
	r.Connections = append(r.Connections, client.ConnOpen())
	r.Errors = append(r.Errors, client.Errors())
	r.Timeouts = append(r.Timeouts, client.Timeouts())
	r.RequestSum = append(r.RequestSum, client.RequestSum())
	r.RequestSuccess = append(r.RequestSuccess, client.RequestSuccess())
	r.BytesWritten = append(r.BytesWritten, client.BytesWritten())
	r.BytesRead = append(r.BytesRead, client.BytesRead())
	r.Qps = append(r.Qps, uint64(throttle.Limit()))
	r.StatusCodes = client.StatusCodes()
	r.ErrorMessages = client.ErrorMessages()
	r.UpdateRequestDuration(client.RequestDuration())
	r.Unlock()
}

func isFlawed() bool {
	if client.Errors() > 0 && errors != client.Errors() {
		errors = client.Errors()
		return true
	}

	return false
}

func load(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			client.Flush()
			return
		case <-throttle.QPS():
			client.Jobsch <- struct{}{}
		}
	}
}

func printSummary(stage string, t time.Time) {
	since := time.Since(t).Seconds()
	fmt.Printf("\n------ %s ------\n", stage)
	fmt.Printf("Elapsed time: %fs\n", since)
	fmt.Printf("Req done: %d; Success: %.2f %%\n", client.RequestSum(), (float64(client.RequestSuccess())/float64(client.RequestSum()))*100)
	fmt.Printf("QPS: %f; Connections: %d\n", float64(client.RequestSum())/since, client.ConnOpen())
	fmt.Printf("Errors: %d; Timeouts: %d\n\n", client.Errors(), client.Timeouts())
}

func acquireProgressBar(t time.Duration) (*pb.ProgressBar, <-chan time.Time) {
	pb := pb.New64(int64(t.Seconds()))
	pb.ShowCounters = false
	pb.ShowPercent = false
	pb.Start()
	return pb, time.Tick(time.Second)
}

func finishProgressBar(pb *pb.ProgressBar) {
	pb.Set64(pb.Total)
	pb.Finish()
}
