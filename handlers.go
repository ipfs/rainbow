package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/ipfs/boxo/blockstore"
	leveldb "github.com/ipfs/go-ds-leveldb"
	"github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	_ "embed"
	_ "net/http/pprof"

	"github.com/felixge/httpsnoop"
	"github.com/ipfs/boxo/gateway"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

//go:embed static/index.html
var indexHTML []byte

func makeMetricsAndDebuggingHandler() *http.ServeMux {
	mux := http.NewServeMux()

	gatherers := prometheus.Gatherers{
		prometheus.DefaultGatherer,
	}
	options := promhttp.HandlerOpts{}
	mux.Handle("/debug/metrics/prometheus", promhttp.HandlerFor(gatherers, options))
	mux.Handle("/debug/vars", http.DefaultServeMux)
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	mux.HandleFunc("/debug/stack", func(w http.ResponseWriter, r *http.Request) {
		if err := writeAllGoroutineStacks(w); err != nil {
			goLog.Error(err)
		}
	})
	MutexFractionOption("/debug/pprof-mutex/", mux)
	BlockProfileRateOption("/debug/pprof-block/", mux)

	return mux
}

func addLogHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/mgr/log/level", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		q := r.URL.Query()
		subsystem := q.Get("subsystem")
		level := q.Get("level")

		if subsystem == "" || level == "" {
			http.Error(w, "both subsystem and level must be passed", http.StatusBadRequest)
			return
		}

		if err := log.SetLogLevel(subsystem, level); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	mux.HandleFunc("/mgr/log/ls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Join(log.GetSubsystems(), ",")))
	})
}

func gcHandler(gnd *Node) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var body struct {
			BytesToFree int64
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := gnd.GC(r.Context(), body.BytesToFree); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func purgePeerHandler(p2pHost host.Host) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		q := r.URL.Query()
		peerIDStr := q.Get("peer")
		if peerIDStr == "" {
			http.Error(w, "missing peer id", http.StatusBadRequest)
			return
		}

		if peerIDStr == "all" {
			purgeCount, err := purgeAllConnections(p2pHost)
			if err != nil {
				goLog.Errorw("Error closing all libp2p connections", "err", err)
				http.Error(w, "error closing connections", http.StatusInternalServerError)
				return
			}
			goLog.Infow("Purged connections", "count", purgeCount)

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "Peer connections purged:", purgeCount)
			return
		}

		peerID, err := peer.Decode(peerIDStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = purgeConnection(p2pHost, peerID)
		if err != nil {
			goLog.Errorw("Error closing libp2p connection", "err", err, "peer", peerID)
			http.Error(w, "error closing connection", http.StatusInternalServerError)
			return
		}
		goLog.Infow("Purged connection", "peer", peerID)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Purged connection to peer", peerID)
	}
}

func purgeConnection(p2pHost host.Host, peerID peer.ID) error {
	peerStore := p2pHost.Peerstore()
	if peerStore != nil {
		peerStore.RemovePeer(peerID)
		peerStore.ClearAddrs(peerID)
	}
	return p2pHost.Network().ClosePeer(peerID)
}

func purgeAllConnections(p2pHost host.Host) (int, error) {
	net := p2pHost.Network()
	peers := net.Peers()

	peerStore := p2pHost.Peerstore()
	if peerStore != nil {
		for _, peerID := range peers {
			peerStore.RemovePeer(peerID)
			peerStore.ClearAddrs(peerID)
		}
	}

	var errCount, purgeCount int
	for _, peerID := range peers {
		err := net.ClosePeer(peerID)
		if err != nil {
			goLog.Errorw("Closing libp2p connection", "err", err, "peer", peerID)
			errCount++
		} else {
			purgeCount++
		}
	}

	if errCount != 0 {
		return 0, fmt.Errorf("error closing connections to %d peers", errCount)
	}

	return purgeCount, nil
}

func showPeersHandler(p2pHost host.Host) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		peers := p2pHost.Network().Peers()
		body := struct {
			Count int64
			Peers []string
		}{
			Count: int64(len(peers)),
		}

		if len(peers) != 0 {
			peerStrs := make([]string, len(peers))
			for i, peerID := range peers {
				peerStrs[i] = peerID.String()
			}
			body.Peers = peerStrs
		}

		enc := json.NewEncoder(w)
		if err := enc.Encode(body); err != nil {
			goLog.Errorw("cannot write response", "err", err)
			http.Error(w, "", http.StatusInternalServerError)
		}
	}
}

func withConnect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ServeMux does not support requests with CONNECT method,
		// so we need to handle them separately
		// https://golang.org/src/net/http/request.go#L111
		if r.Method == http.MethodConnect {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(next, w, r)
		goLog.Infow(r.Method, "url", r.URL, "host", r.Host, "code", m.Code, "duration", m.Duration, "written", m.Written, "ua", r.UserAgent(), "referer", r.Referer())
	})
}

func setupGatewayHandler(cfg Config, nd *Node) (http.Handler, error) {
	var (
		backend gateway.IPFSBackend
		err     error
	)

	options := []gateway.BackendOption{
		gateway.WithValueStore(nd.vs),
		gateway.WithNameSystem(nd.ns),
		gateway.WithResolver(nd.resolver), // May be nil, but that is fine.
	}

	if len(cfg.RemoteBackends) > 0 && cfg.RemoteBackendMode == RemoteBackendCAR {
		var fetcher gateway.CarFetcher
		fetcher, err = gateway.NewRemoteCarFetcher(cfg.RemoteBackends, nil)
		if err != nil {
			return nil, err
		}
		backend, err = gateway.NewCarBackend(fetcher, options...)
	} else {
		backend, err = gateway.NewBlocksBackend(nd.bsrv, options...)
	}
	if err != nil {
		return nil, err
	}

	headers := map[string][]string{}

	// Note: in the future we may want to make this more configurable.
	noDNSLink := false

	// TODO: allow appending hostnames to this list via ENV variable (separate PATH_GATEWAY_HOSTS & SUBDOMAIN_GATEWAY_HOSTS)
	publicGateways := map[string]*gateway.PublicGateway{
		"localhost": {
			Paths:                 []string{"/ipfs", "/ipns", "/version"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         false,
			DeserializedResponses: true,
			UseSubdomains:         true,
		},
	}
	for _, domain := range cfg.GatewayDomains {
		publicGateways[domain] = &gateway.PublicGateway{
			Paths:                 []string{"/ipfs", "/ipns", "/version"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         true,
			DeserializedResponses: true,
			UseSubdomains:         false,
		}
	}

	for _, domain := range cfg.SubdomainGatewayDomains {
		publicGateways[domain] = &gateway.PublicGateway{
			Paths:                 []string{"/ipfs", "/ipns", "/version"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         true,
			DeserializedResponses: true,
			UseSubdomains:         true,
		}
	}

	for _, domain := range cfg.TrustlessGatewayDomains {
		publicGateways[domain] = &gateway.PublicGateway{
			Paths:                 []string{"/ipfs", "/ipns", "/version"},
			NoDNSLink:             true,
			InlineDNSLink:         true,
			DeserializedResponses: false,
			UseSubdomains:         contains(cfg.SubdomainGatewayDomains, domain),
		}
	}

	// If we're doing tests, ensure the right public gateways are enabled.
	if os.Getenv("GATEWAY_CONFORMANCE_TEST") == "true" {
		publicGateways["example.com"] = &gateway.PublicGateway{
			Paths:                 []string{"/ipfs", "/ipns"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         true,
			DeserializedResponses: true,
			UseSubdomains:         true,
		}

		// TODO: revisit the below once we clarify desired behavior in https://specs.ipfs.tech/http-gateways/subdomain-gateway/
		publicGateways["localhost"].InlineDNSLink = true
	}

	gwConf := gateway.Config{
		DeserializedResponses: true,
		PublicGateways:        publicGateways,
		NoDNSLink:             noDNSLink,
	}
	gwHandler := gateway.NewHandler(gwConf, backend)

	ipfsHandler := withHTTPMetrics(gwHandler, "ipfs", cfg.disableMetrics)
	ipnsHandler := withHTTPMetrics(gwHandler, "ipns", cfg.disableMetrics)

	topMux := http.NewServeMux()
	topMux.Handle("/ipfs/", ipfsHandler)
	topMux.Handle("/ipns/", ipnsHandler)
	topMux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Client: %s\n", name)
		fmt.Fprintf(w, "Version: %s\n", version)
	})
	topMux.HandleFunc("/api/v0/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte("The /api/v0 Kubo RPC is not part of IPFS Gateway Specs (https://specs.ipfs.tech/http-gateways/). Consider refactoring your app. If you still need this Kubo endpoint, please self-host a Kubo instance yourself: https://docs.ipfs.tech/install/command-line/ with proper auth https://github.com/ipfs/kubo/blob/master/docs/config.md#apiauthorizations"))
	})
	topMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(indexHTML)
	})

	// Construct the HTTP handler for the gateway.
	handler := withConnect(topMux)
	handler = http.Handler(gateway.NewHostnameHandler(gwConf, backend, handler))

	// Add custom headers and liberal CORS.
	handler = gateway.NewHeaders(headers).ApplyCors().Wrap(handler)

	handler = servertiming.Middleware(handler, nil)

	// Add logging.
	handler = withRequestLogger(handler)

	// Add tracing.
	handler = withTracingAndDebug(handler, cfg.TracingAuthToken)

	return handler, nil
}

func withTracingAndDebug(next http.Handler, authToken string) http.Handler {
	next = otelhttp.NewHandler(next, "Gateway")

	// Remove tracing and cache skipping headers if not authorized
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Disable tracing/debug headers if auth token missing or invalid
		if authToken == "" || request.Header.Get("Authorization") != authToken {
			if request.Header.Get("Traceparent") != "" || request.Header.Get("Tracestate") != "" || request.Header.Get(NoBlockcacheHeader) != "" {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = writer.Write([]byte("The request is not accompanied by a valid authorization header"))
				return
			}
		}

		// Process cache skipping header
		if noBlockCache := request.Header.Get(NoBlockcacheHeader); noBlockCache == "true" {
			ds, err := leveldb.NewDatastore("", nil)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = writer.Write([]byte(err.Error()))
				return
			}
			newCtx := context.WithValue(request.Context(), NoBlockcache{}, blockstore.NewBlockstore(ds))
			request = request.WithContext(newCtx)
		}

		next.ServeHTTP(writer, request)
	})
}

const NoBlockcacheHeader = "Rainbow-No-Blockcache"

type NoBlockcache struct{}

// MutexFractionOption allows to set runtime.SetMutexProfileFraction via HTTP
// using POST request with parameter 'fraction'.
func MutexFractionOption(path string, mux *http.ServeMux) *http.ServeMux {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		asfr := r.Form.Get("fraction")
		if len(asfr) == 0 {
			http.Error(w, "parameter 'fraction' must be set", http.StatusBadRequest)
			return
		}

		fr, err := strconv.Atoi(asfr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runtime.SetMutexProfileFraction(fr)
	})

	return mux
}

// BlockProfileRateOption allows to set runtime.SetBlockProfileRate via HTTP
// using POST request with parameter 'rate'.
// The profiler tries to sample 1 event every <rate> nanoseconds.
// If rate == 1, then the profiler samples every blocking event.
// To disable, set rate = 0.
func BlockProfileRateOption(path string, mux *http.ServeMux) *http.ServeMux {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rateStr := r.Form.Get("rate")
		if len(rateStr) == 0 {
			http.Error(w, "parameter 'rate' must be set", http.StatusBadRequest)
			return
		}

		rate, err := strconv.Atoi(rateStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runtime.SetBlockProfileRate(rate)
	})
	return mux
}

func contains[T comparable](collection []T, element T) bool {
	for _, item := range collection {
		if item == element {
			return true
		}
	}

	return false
}
