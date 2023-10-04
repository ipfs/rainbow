package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func registerVersionMetric(version string) {
	m := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "ipfs",
		Subsystem:   "rainbow",
		Name:        "info",
		Help:        "Information about rainbow instance.",
		ConstLabels: prometheus.Labels{"version": version},
	})
	prometheus.MustRegister(m)
	m.Set(1)
}

// httpMetricsObjectives Objectives map defines the quantile objectives for a
// summary metric in Prometheus. Each key-value pair in the map represents a
// quantile level and the desired maximum error allowed for that quantile.
//
// Adjusting the objectives control the trade-off between
// accuracy and resource usage for the summary metric.
//
// Example: 0.95: 0.005 means that the 95th percentile (P95) should have a
// maximum error of 0.005, which represents a 0.5% error margin.
var httpMetricsObjectives = map[float64]float64{
	0.5:  0.05,
	0.75: 0.025,
	0.9:  0.01,
	0.95: 0.005,
	0.99: 0.001,
}

// withHTTPMetrics collects metrics around HTTP request/response count, duration, and size
// per specific handler. Allows us to track them separately for /ipns and /ipfs.
func withHTTPMetrics(handler http.Handler, handlerName string) http.Handler {

	// HTTP metric template names match Kubo:
	// https://github.com/ipfs/kubo/blob/e550d9e4761ea394357c413c02ade142c0dea88c/core/corehttp/metrics.go#L79-L152
	// This allows Kubo users to migrate to rainbow and compare global totals.
	opts := prometheus.SummaryOpts{
		Namespace:   "ipfs",
		Subsystem:   "http",
		Objectives:  httpMetricsObjectives,
		ConstLabels: prometheus.Labels{"handler": handlerName},
	}
	// Dynamic labels 'method or 'code' are auto-filled
	// by https://pkg.go.dev/github.com/prometheus/client_golang/prometheus/promhttp#InstrumentHandlerResponseSize
	labels := []string{"method", "code"}

	reqWip := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   opts.Namespace,
			Subsystem:   opts.Subsystem,
			Name:        "requests_inflight",
			Help:        "Tracks the number of HTTP client requests currently in progress.",
			ConstLabels: opts.ConstLabels,
		},
	)
	prometheus.MustRegister(reqWip)

	reqCnt := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   opts.Namespace,
			Subsystem:   opts.Subsystem,
			Name:        "requests_total",
			Help:        "Total number of HTTP requests made.",
			ConstLabels: opts.ConstLabels,
		},
		labels,
	)
	prometheus.MustRegister(reqCnt)

	opts.Name = "request_duration_seconds"
	opts.Help = "The HTTP request latencies in seconds."
	reqDur := prometheus.NewSummaryVec(opts, labels)
	prometheus.MustRegister(reqDur)

	opts.Name = "request_size_bytes"
	opts.Help = "The HTTP request sizes in bytes."
	reqSz := prometheus.NewSummaryVec(opts, labels)
	prometheus.MustRegister(reqSz)

	opts.Name = "response_size_bytes"
	opts.Help = "The HTTP response sizes in bytes."
	resSz := prometheus.NewSummaryVec(opts, labels)
	prometheus.MustRegister(resSz)

	handler = promhttp.InstrumentHandlerInFlight(reqWip, handler)
	handler = promhttp.InstrumentHandlerCounter(reqCnt, handler)
	handler = promhttp.InstrumentHandlerDuration(reqDur, handler)
	handler = promhttp.InstrumentHandlerRequestSize(reqSz, handler)
	handler = promhttp.InstrumentHandlerResponseSize(resSz, handler)

	return handler
}
