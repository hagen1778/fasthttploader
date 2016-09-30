package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var (
	requestDuration	prometheus.Summary
	connOpen	prometheus.Gauge

	timeouts 	prometheus.Counter
	errors		prometheus.Counter
	requestSum	prometheus.Counter
	connError 	prometheus.Counter
	bytesWritten 	prometheus.Counter
	bytesRead 	prometheus.Counter
	writeError 	prometheus.Counter
	readError 	prometheus.Counter
)

func initMetrics() {
	timeouts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "request_timeouts",
			Help: "Number of timeouts returned by server",
		},
	)

	errors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "request_errors",
			Help: "Number of errors returned by server. Including amount of timeouts",
		},
	)

	requestSum = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "request_sum",
			Help: "Total number of sent requests",
		},
	)

	requestDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "request_duration",
			Help: "Latency of sent requests",
			Objectives: map[float64]float64{0.5: 0.05, 0.75: 0.025, 0.8: 0.02, 0.9: 0.01, 0.99: 0.001},
		},
	)

	connOpen = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "conn_open",
			Help: "Number of open connections",
		},
	)

	connError = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "conn_errors",
			Help: "Number of connections ended with error",
		},
	)

	bytesWritten = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bytes_written",
			Help: "Amount of written bytes",
		},
	)

	bytesRead = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bytes_read",
			Help: "Amount of read bytes",
		},
	)

	writeError = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "request_write_errors",
			Help: "Number of errors while writing",
		},
	)

	readError = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "request_read_errors",
			Help: "Number of errors while reading",
		},
	)
}

func register() {
	initMetrics()
	prometheus.MustRegister(timeouts)
	prometheus.MustRegister(errors)
	prometheus.MustRegister(requestSum)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(connOpen)
	prometheus.MustRegister(connError)
	prometheus.MustRegister(bytesWritten)
	prometheus.MustRegister(bytesRead)
	prometheus.MustRegister(writeError)
	prometheus.MustRegister(readError)
}

func unregister() {
	prometheus.Unregister(timeouts)
	prometheus.Unregister(errors)
	prometheus.Unregister(requestSum)
	prometheus.Unregister(requestDuration)
	prometheus.Unregister(connOpen)
	prometheus.Unregister(connError)
	prometheus.Unregister(bytesWritten)
	prometheus.Unregister(bytesRead)
	prometheus.Unregister(writeError)
	prometheus.Unregister(readError)
}

func flushMetrics() {
	unregister()
	register()
}

var m = &dto.Metric{}

func Errors() uint64 {
	errors.Write(m)
	return uint64(*m.Counter.Value)
}

func Timeouts() uint64 {
	timeouts.Write(m)
	return uint64(*m.Counter.Value)
}

func RequestSum() uint64 {
	requestSum.Write(m)
	return uint64(*m.Counter.Value)
}

func BytesWritten() uint64 {
	bytesWritten.Write(m)
	return uint64(*m.Counter.Value)
}

func BytesRead() uint64 {
	bytesRead.Write(m)
	return uint64(*m.Counter.Value)
}

func ConnOpen() uint64 {
	connOpen.Write(m)
	return uint64(*m.Gauge.Value)
}

func RequestDuration() map[float64]float64 {
	requestDuration.Write(m)
	result := make(map[float64]float64, len(m.Summary.Quantile))
	for _, v := range m.Summary.Quantile {
		result[*v.Quantile] = *v.Value
	}

	return result
}
