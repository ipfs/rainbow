package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ipfs/boxo/blockstore"
	leveldb "github.com/ipfs/go-ds-leveldb"
	"net/http"
	"os"
	"runtime"
	"strconv"

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

func GCHandler(gnd *Node) func(w http.ResponseWriter, r *http.Request) {
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

func setupGatewayHandler(cfg Config, nd *Node, tracingAuth string) (http.Handler, error) {
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

	var ipfsHandler, ipnsHandler http.Handler
	if cfg.disableMetrics {
		ipfsHandler = gwHandler
		ipnsHandler = gwHandler
	} else {
		ipfsHandler = withHTTPMetrics(gwHandler, "ipfs")
		ipnsHandler = withHTTPMetrics(gwHandler, "ipns")
	}

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
	handler = otelhttp.NewHandler(handler, "Gateway")

	// Remove tracing and cache skipping headers if not authorized
	prevHandler := handler
	handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != tracingAuth {
			if request.Header.Get("Traceparent") != "" {
				request.Header.Del("Traceparent")
			}
			if request.Header.Get("Tracestate") != "" {
				request.Header.Del("Tracestate")
			}
			if request.Header.Get(NoBlockcacheHeader) != "" {
				request.Header.Del(NoBlockcacheHeader)
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

		prevHandler.ServeHTTP(writer, request)
	})

	return handler, nil
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
