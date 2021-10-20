package main

import (
	"fmt"
	"net/http"
	"sort"

	id "github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

var CurrentCommit = "wip-commit"

var ClientVersion = "rainbow-gateway"

type GatewayConfig struct {
	Headers      map[string][]string
	Writable     bool
	PathPrefixes []string
}

// A helper function to clean up a set of headers:
// 1. Canonicalizes.
// 2. Deduplicates.
// 3. Sorts.
func cleanHeaderSet(headers []string) []string {
	// Deduplicate and canonicalize.
	m := make(map[string]struct{}, len(headers))
	for _, h := range headers {
		m[http.CanonicalHeaderKey(h)] = struct{}{}
	}
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}

	// Sort
	sort.Strings(result)
	return result
}

func setupHandlerMux(paths []string, cfgheaders map[string][]string, pathPrefixes []string, nd *Node) (*http.ServeMux, error) {
	headers := make(map[string][]string, len(cfgheaders))
	for h, v := range cfgheaders {
		headers[http.CanonicalHeaderKey(h)] = v
	}

	// Hard-coded headers.
	const ACAHeadersName = "Access-Control-Allow-Headers"
	const ACEHeadersName = "Access-Control-Expose-Headers"
	const ACAOriginName = "Access-Control-Allow-Origin"
	const ACAMethodsName = "Access-Control-Allow-Methods"

	if _, ok := headers[ACAOriginName]; !ok {
		// Default to *all*
		headers[ACAOriginName] = []string{"*"}
	}
	if _, ok := headers[ACAMethodsName]; !ok {
		// Default to GET
		headers[ACAMethodsName] = []string{http.MethodGet}
	}

	headers[ACAHeadersName] = cleanHeaderSet(
		append([]string{
			"Content-Type",
			"User-Agent",
			"Range",
			"X-Requested-With",
		}, headers[ACAHeadersName]...))

	headers[ACEHeadersName] = cleanHeaderSet(
		append([]string{
			"Content-Range",
			"X-Chunked-Output",
			"X-Stream-Output",
		}, headers[ACEHeadersName]...))

	gateway := newGatewayHandler(GatewayConfig{
		Headers:      headers,
		PathPrefixes: pathPrefixes,
	}, nd)

	mux := http.NewServeMux()

	for _, p := range paths {
		mux.Handle(p+"/", gateway)
	}

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Commit: %s\n", CurrentCommit)
		fmt.Fprintf(w, "Client Version: %s\n", ClientVersion)
		fmt.Fprintf(w, "Protocol Version: %s\n", id.LibP2PVersion)
	})
	return mux, nil
}

/*
func VersionOption() ServeOption {
	return func(_ *core.IpfsNode, _ net.Listener, mux *http.ServeMux) (*http.ServeMux, error) {
		return mux, nil
	}
}
*/
