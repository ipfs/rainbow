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

// Duration histograms  use fixed definition here, as we don't want to break existing buckets if we need to add more.
var defaultDurationHistogramBuckets = []float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 240, 480, 960, 1920}

// withHTTPMetrics collects metrics around HTTP request/response count, duration, and size
// per specific handler. Allows us to track them separately for /ipns and /ipfs.
func withHTTPMetrics(handler http.Handler, handlerName string) http.Handler {

	opts := prometheus.HistogramOpts{
		Namespace:   "ipfs",
		Subsystem:   "http",
		Buckets:     defaultDurationHistogramBuckets,
		ConstLabels: prometheus.Labels{"handler": handlerName},
	}

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
	opts.Help = "The HTTP request latencies in seconds. "
	reqDurHist := prometheus.NewHistogramVec(opts, labels)
	prometheus.MustRegister(reqDurHist)

	opts.Name = "request_size_bytes"
	opts.Help = "The HTTP request sizes in bytes."
	reqSzHist := prometheus.NewHistogramVec(opts, labels)
	prometheus.MustRegister(reqSzHist)

	opts.Name = "response_size_bytes"
	opts.Help = "The HTTP response sizes in bytes."
	resSzHist := prometheus.NewHistogramVec(opts, labels)
	prometheus.MustRegister(resSzHist)

	handler = promhttp.InstrumentHandlerInFlight(reqWip, handler)
	handler = promhttp.InstrumentHandlerCounter(reqCnt, handler)
	handler = promhttp.InstrumentHandlerDuration(reqDurHist, handler)
	handler = promhttp.InstrumentHandlerRequestSize(reqSzHist, handler)
	handler = promhttp.InstrumentHandlerResponseSize(resSzHist, handler)

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
