# fasthttploader

fasthttploader is a simple http loader, like apache benchamrk, based on [fasthttp](https://github.com/valyala/fasthttp). Originally its forked from github.com/rakyll/boom but later, forced to new project

This loader was written with purpose to get practice in golang

## Usage

~~~
Usage: boom [options...] <url>

Options:
  -n  Number of requests to run.
  -c  Number of requests to run concurrently. Total number of requests cannot
      be smaller than the concurency level.
  -q  Rate limit, in seconds (QPS).
  -o  Output type. If none provided, a summary is printed.
      "csv" is the only supported alternative. Dumps the response
      metrics in comma-seperated values format.

  -m  HTTP method, one of GET, POST, PUT, DELETE, HEAD, OPTIONS.
  -h  Custom HTTP headers, name1:value1;name2:value2.
  -t  Timeout in ms.
  -A  HTTP Accept header.
  -d  HTTP request body.
  -T  Content-type, defaults to "text/html".

  -disable-compression  Disable compression.
~~~
