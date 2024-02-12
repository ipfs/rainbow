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

var peersTotalMetric = prometheus.NewDesc(
	prometheus.BuildFQName("ipfs", "p2p", "peers_total"),
	"Number of connected peers",
	[]string{"transport"},
	nil,
)

// IpfsNodeCollector collects peer metrics
type IpfsNodeCollector struct {
	Node *Node
}

func (IpfsNodeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- peersTotalMetric
}

func (c IpfsNodeCollector) Collect(ch chan<- prometheus.Metric) {
	for tr, val := range c.PeersTotalValues() {
		ch <- prometheus.MustNewConstMetric(
			peersTotalMetric,
			prometheus.GaugeValue,
			val,
			tr,
		)
	}
}

func (c IpfsNodeCollector) PeersTotalValues() map[string]float64 {
	vals := make(map[string]float64)
	if c.Node.host == nil {
		return vals
	}
	for _, peerID := range c.Node.host.Network().Peers() {
		// Each peer may have more than one connection (see for an explanation
		// https://github.com/libp2p/go-libp2p-swarm/commit/0538806), so we grab
		// only one, the first (an arbitrary and non-deterministic choice), which
		// according to ConnsToPeer is the oldest connection in the list
		// (https://github.com/libp2p/go-libp2p-swarm/blob/v0.2.6/swarm.go#L362-L364).
		conns := c.Node.host.Network().ConnsToPeer(peerID)
		if len(conns) == 0 {
			continue
		}
		tr := ""
		for _, proto := range conns[0].RemoteMultiaddr().Protocols() {
			tr = tr + "/" + proto.Name
		}
		vals[tr] = vals[tr] + 1
	}
	return vals
}

func registerIpfsNodeCollector(nd *Node) {
	prometheus.MustRegister(&IpfsNodeCollector{Node: nd})
}
