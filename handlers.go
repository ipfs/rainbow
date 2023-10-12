package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	_ "net/http/pprof"

	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/path"
	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const DefaultKuboRPC = "http://127.0.0.1:5001"

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
		goLog.Infow(r.Method, "url", r.URL, "host", r.Host)
		// TODO: if debug is enabled, show more? goLog.Infow("request received", "url", r.URL, "host", r.Host, "method", r.Method, "ua", r.UserAgent(), "referer", r.Referer())
		next.ServeHTTP(w, r)
	})
}

func setupGatewayHandler(nd *Node) (http.Handler, error) {
	backend, err := gateway.NewBlocksBackend(nd.bsrv, gateway.WithValueStore(nd.vs), gateway.WithNameSystem(nd.ns))
	if err != nil {
		return nil, err
	}

	headers := map[string][]string{}
	gateway.AddAccessControlHeaders(headers)

	// Note: in the future we may want to make this more configurable.
	noDNSLink := false

	// TODO: allow appending hostnames to this list via ENV variable (separate PATH_GATEWAY_HOSTS & SUBDOMAIN_GATEWAY_HOSTS)
	publicGateways := map[string]*gateway.PublicGateway{
		"localhost": {
			Paths:                 []string{"/ipfs", "/ipns", "/version", "/api/v0"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         false,
			DeserializedResponses: true,
			UseSubdomains:         true,
		},
		"dweb.link": {
			Paths:                 []string{"/ipfs", "/ipns", "/version", "/api/v0"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         true,
			DeserializedResponses: true,
			UseSubdomains:         true,
		},
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
		Headers:               headers,
		PublicGateways:        publicGateways,
		NoDNSLink:             noDNSLink,
	}
	gwHandler := gateway.NewHandler(gwConf, backend)
	ipfsHandler := withHTTPMetrics(gwHandler, "ipfs")
	ipnsHandler := withHTTPMetrics(gwHandler, "ipns")

	topMux := http.NewServeMux()
	topMux.Handle("/ipfs/", ipfsHandler)
	topMux.Handle("/ipns/", ipnsHandler)
	topMux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Client: %s\n", name)
		fmt.Fprintf(w, "Version: %s\n", version)
	})
	// TODO: below is legacy which we want to remove, measuring this separately
	// allows us to decide when is the time to do it.
	legacyKuboRPCHandler := withHTTPMetrics(newKuboRPCHandler(nd.kuboRPCs), "legacyKuboRpc")
	topMux.Handle("/api/v0/", legacyKuboRPCHandler)

	// Construct the HTTP handler for the gateway.
	handler := withConnect(topMux)
	handler = http.Handler(gateway.NewHostnameHandler(gwConf, backend, handler))
	handler = servertiming.Middleware(handler, nil)

	// Add logging.
	handler = withRequestLogger(handler)

	// Add tracing.
	handler = otelhttp.NewHandler(handler, "Gateway")

	return handler, nil
}

func newKuboRPCHandler(endpoints []string) http.Handler {
	mux := http.NewServeMux()

	// Endpoints that can be redirected to the gateway itself as they can be handled
	// by the path gateway. We use 303 See Other here to ensure that the API requests
	// are transformed to GET requests to the gateway.
	// - https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/303
	redirectToGateway := func(format string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			path, err := path.NewPath(r.URL.Query().Get("arg"))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
				return
			}
			url := path.String()
			if format != "" {
				url += "?format=" + format
			}

			goLog.Debugw("api request redirected to gateway", "url", r.URL, "redirect", url)
			http.Redirect(w, r, url, http.StatusSeeOther)
		}
	}

	mux.HandleFunc("/api/v0/cat", redirectToGateway(""))
	mux.HandleFunc("/api/v0/dag/export", redirectToGateway("car"))
	mux.HandleFunc("/api/v0/block/get", redirectToGateway("raw"))
	mux.HandleFunc("/api/v0/dag/get", func(w http.ResponseWriter, r *http.Request) {
		path, err := path.NewPath(r.URL.Query().Get("arg"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		codec := r.URL.Query().Get("output-codec")
		if codec == "" {
			codec = "dag-json"
		}
		url := fmt.Sprintf("%s?format=%s", path.String(), codec)
		goLog.Debugw("api request redirected to gateway", "url", r.URL, "redirect", url)
		http.Redirect(w, r, url, http.StatusSeeOther)
	})

	// Endpoints that have high traffic volume. We will keep redirecting these
	// for now to Kubo endpoints that are able to handle these requests. We use
	// 307 Temporary Redirect in order to preserve the original HTTP Method.
	// - https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/307
	s := rand.NewSource(time.Now().Unix())
	rand := rand.New(s)
	redirectToKubo := func(w http.ResponseWriter, r *http.Request) {
		// Naively choose one of the Kubo RPC clients.
		endpoint := endpoints[rand.Intn(len(endpoints))]
		url := endpoint + r.URL.Path + "?" + r.URL.RawQuery
		goLog.Debugw("api request redirected to kubo", "url", r.URL, "redirect", url)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}

	mux.HandleFunc("/api/v0/name/resolve", redirectToKubo)
	mux.HandleFunc("/api/v0/resolve", redirectToKubo)
	mux.HandleFunc("/api/v0/dag/resolve", redirectToKubo)
	mux.HandleFunc("/api/v0/dns", redirectToKubo)

	// Remaining requests to the API receive a 501, as well as an explanation.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		goLog.Debugw("api request returned 501", "url", r.URL)
		w.Write([]byte("The /api/v0 Kubo RPC is now discontinued on this server as it is not part of the gateway specification. If you need this API, please self-host a Kubo instance yourself: https://docs.ipfs.tech/install/command-line/"))
	})

	return mux
}

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
