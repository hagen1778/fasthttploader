# fasthttploader

Tool for http load-testing with auto-adjustment of load.

### Quickstart
Install:
```
go get github.com/hagen1778/fasthttploader
```

Run test:
```
fasthttploader http://google.com
```

Expected results:
```
------ Burst Throughput ------
Elapsed time: 5.000740s
Req done: 2955; Success: 100.00 %
Rps: 590.912523; Connections: 500
Errors: 0; Timeouts: 0

------ Adjustment test ------
Elapsed time: 30.000411s
Req done: 37684; Success: 100.00 %
Rps: 1256.116142; Connections: 1509
Errors: 0; Timeouts: 0

------ Loading test ------
Elapsed time: 20.000837s
Req done: 36433; Success: 100.00 %
Rps: 1821.573798; Connections: 1738
Errors: 0; Timeouts: 0
```

Besides console result you can check html-report:
![Charts with results of test](https://raw.githubusercontent.com/hagen1778/fasthttploader/master/charts.jpg "Result chart")

### Usage
```
Usage: fasthttploader [options...] <url>
Notice: fasthttploader would force agressive burst stages before testing 
to detect max qps and number for clients. To avoid this you need to 
set -c and -q parameters.

Options:
  -A string
        Set Accept headers
  -T string
        Set content-type headers (default "text/html")
  -b string
        Set body
  -c int
        Number of supposed clients (default 500)
  -cpuprofile string
        write cpu profile to file
  -d duration
        Cant be less than 20sec (default 20s)
  -debug
        Print debug messages if true
  -disable-compression
        Disables compression if true
  -gatewayAddr string
        Address of PushGateway service (default "localhost:9091")
  -h string
        Set headers
  -httpClientKeepAlivePeriod duration
        Interval for sending keep-alive messageson keepalive connections. 
        Zero disables keep-alive messages (default 5s)
  -httpClientReadBufferSize int
        Per-connection read buffer size for httpclient (default 8192)
  -httpClientRequestTimeout duration
        Maximum time to wait for http response (default 10s)
  -httpClientWriteBufferSize int
        Per-connection write buffer size for httpclient (default 8192)
  -jobName string
        Name of the job for PushGateway (default "pushGateway")
  -k    Disable keepalive if true
  -m string
        Set HTTP method (default "GET")
  -memprofile string
        write memory profile to this file
  -q int
        Request per second limit. Detect automatically, if not setted
  -r string
        Set filename to store final report (default "report.html")
  -t duration
        Request timeout (default 5s)

```

### Stages
Testing consist of 3 stages:
* Burst - 5sec test with no limits by QPS and number of clients equal (by default, but can be changed by -c passing) to 500. Burst stage helps to detect possible QPS rate for further stages
* Adjustment - 30sec test with smoothly QPS and clients tunning. Initial QPS and number of clients is taken from results of Burst stage. During 30s QPS and number of clients would be increased
till timeout or errors limit
* Testing - step by step load balancing (10 steps) and keeping it while time is left

