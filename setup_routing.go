package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ocprom "contrib.go.opencensus.io/exporter/prometheus"
	autoconf "github.com/ipfs/boxo/autoconf"
	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/ipns"
	routingv1client "github.com/ipfs/boxo/routing/http/client"
	httpcontentrouter "github.com/ipfs/boxo/routing/http/contentrouter"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/fullrt"
	record "github.com/libp2p/go-libp2p-record"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	prometheus "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func init() {
	promRegistry, ok := prometheus.DefaultRegisterer.(*prometheus.Registry)
	if !ok {
		goLog.Error("routing metrics: error casting DefaultRegisterer")
		return
	}
	pe, err := ocprom.NewExporter(ocprom.Options{
		Namespace: "ipfs",
		Registry:  promRegistry,
		OnError: func(err error) {
			goLog.Errorf("ocprom error: %w", err)
		},
	})
	if err != nil {
		goLog.Errorf("routing metrics: error creating exporter: %w", err)
		return
	}
	view.RegisterExporter(pe)
	view.SetReportingPeriod(2 * time.Second)
}

func setupDelegatedRouting(cfg Config, dnsCache *cachedDNS) ([]routing.Routing, error) {
	// Set configurable timeout with 30s default
	timeout := cfg.RoutingV1HTTPClientTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	// Increase per-host connection pool since we are making lots of concurrent requests.
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: otelhttp.NewTransport(
			&routingv1client.ResponseBodyLimitedTransport{
				RoundTripper: &customTransport{
					// Roundtripper with increased defaults than http.Transport such that retrieving
					// multiple lookups concurrently is fast.
					RoundTripper: &http.Transport{
						MaxIdleConns:        1000,
						MaxConnsPerHost:     100,
						MaxIdleConnsPerHost: 100,
						IdleConnTimeout:     90 * time.Second,
						DialContext:         dnsCache.dialWithCachedDNS,
						ForceAttemptHTTP2:   true,
					},
				},
				LimitBytes: 1 << 20,
			}),
	}

	var delegatedRouters []routing.Routing

	// Group endpoints by base URL and merge capabilities
	// Rainbow only supports read operations (no IPNS publishing)
	goLog.Debugf("Input endpoints: %v", cfg.RoutingV1Endpoints)
	groupedEndpoints, err := autoconf.GroupByKnownCapabilities(cfg.RoutingV1Endpoints, true, false)
	if err != nil {
		return nil, err
	}
	goLog.Debugf("Grouped endpoints: %v", groupedEndpoints)

	// Create a routing client for each unique base URL with appropriate capabilities
	for baseURL, capabilities := range groupedEndpoints {
		if capabilities.IsEmpty() {
			goLog.Warnf("Skipping endpoint %s with no capabilities", baseURL)
			continue
		}

		goLog.Debugf("Creating routing client for base URL %q with capabilities: Providers=%v, Peers=%v, IPNSGet=%v",
			baseURL, capabilities.Providers, capabilities.Peers, capabilities.IPNSGet)

		// Create HTTP routing client with base URL
		delegatedRouter, err := delegatedHTTPContentRouterWithCapabilities(baseURL, capabilities,
			routingv1client.WithHTTPClient(httpClient),
			routingv1client.WithProtocolFilter(cfg.RoutingV1FilterProtocols), // IPIP-484
			routingv1client.WithStreamResultsRequired(),                      // https://specs.ipfs.tech/routing/http-routing-v1/#streaming
			routingv1client.WithDisabledLocalFiltering(false),                // force local filtering in case remote server does not support IPIP-484
		)
		if err != nil {
			return nil, err
		}
		delegatedRouters = append(delegatedRouters, delegatedRouter)
	}

	return delegatedRouters, nil
}

// parseBootstrapPeers parses a list of bootstrap peer strings into AddrInfo structs
// It skips empty strings and "auto" placeholders, and logs warnings for invalid peers
func parseBootstrapPeers(peers []string, warnOnAuto bool) ([]peer.AddrInfo, error) {
	bootstrapPeers := make([]peer.AddrInfo, 0, len(peers))
	for _, peerStr := range peers {
		if peerStr == "" {
			continue
		}
		if peerStr == autoconf.AutoPlaceholder {
			if warnOnAuto {
				goLog.Warnf("Bootstrap peer 'auto' placeholder not expanded - is autoconf enabled?")
			}
			continue
		}
		ai, err := peer.AddrInfoFromString(peerStr)
		if err != nil {
			goLog.Warnf("Failed to parse bootstrap peer %q: %v", peerStr, err)
			continue
		}
		bootstrapPeers = append(bootstrapPeers, *ai)
	}
	return bootstrapPeers, nil
}

func setupDHTRouting(ctx context.Context, cfg Config, h host.Host, ds datastore.Batching, dhtRcMgr network.ResourceManager, bwc metrics.Reporter) (routing.Routing, error) {
	if cfg.DHTRouting == DHTOff {
		return nil, nil
	}

	// Parse bootstrap peers
	bootstrapPeers, err := parseBootstrapPeers(cfg.Bootstrap, true)
	if err != nil {
		return nil, err
	}

	// If no bootstrap peers provided, use defaults for seed peering or error otherwise
	if len(bootstrapPeers) == 0 {
		if !cfg.SeedPeering {
			return nil, fmt.Errorf("no valid bootstrap peers configured - provide bootstrap peers or enable autoconf")
		}
		// Use default bootstrap peers for seed peering
		bootstrapPeers = dht.GetDefaultBootstrapPeerAddrInfos()
	}

	var dhtHost host.Host
	if cfg.DHTSharedHost {
		dhtHost = h
	} else {
		dhtHost, err = libp2p.New(
			libp2p.UserAgent("rainbow/"+buildVersion()),
			libp2p.NoListenAddrs,
			libp2p.BandwidthReporter(bwc),
			libp2p.DefaultTransports,
			libp2p.DefaultMuxers,
			libp2p.ResourceManager(dhtRcMgr),
		)
		if err != nil {
			return nil, err
		}
	}

	standardClient, err := dht.New(ctx, dhtHost,
		dht.Datastore(ds),
		dht.BootstrapPeers(bootstrapPeers...),
		dht.Mode(dht.ModeClient),
	)
	if err != nil {
		return nil, err
	}

	if cfg.DHTRouting == DHTStandard {
		return standardClient, nil
	}

	if cfg.DHTRouting == DHTAccelerated {
		fullRTClient, err := fullrt.NewFullRT(dhtHost, dht.DefaultPrefix,
			fullrt.DHTOption(
				dht.Validator(record.NamespacedValidator{
					"pk":   record.PublicKeyValidator{},
					"ipns": ipns.Validator{KeyBook: h.Peerstore()},
				}),
				dht.Datastore(ds),
				dht.BootstrapPeers(bootstrapPeers...),
				dht.BucketSize(20),
			))
		if err != nil {
			return nil, err
		}
		return &bundledDHT{
			standard: standardClient,
			fullRT:   fullRTClient,
		}, nil
	}

	return nil, fmt.Errorf("unknown DHTRouting option: %q", cfg.DHTRouting)
}

func setupCompositeRouting(delegatedRouters []routing.Routing, dht routing.Routing, cfg Config) routing.Routing {
	// Default router is no routing at all: can be especially useful during tests.
	var router routing.Routing
	router = &routinghelpers.Null{}

	if len(delegatedRouters) == 0 && dht != nil {
		router = dht
	} else {
		var routers []*routinghelpers.ParallelRouter

		if dht != nil {
			routers = append(routers, &routinghelpers.ParallelRouter{
				Router:                  dht,
				ExecuteAfter:            0,
				DoNotWaitForSearchValue: true,
				IgnoreError:             false,
			})
		}

		timeout := cfg.RoutingV1HTTPClientTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		for _, routingV1Router := range delegatedRouters {
			routers = append(routers, &routinghelpers.ParallelRouter{
				Timeout:                 timeout,
				Router:                  routingV1Router,
				ExecuteAfter:            0,
				DoNotWaitForSearchValue: true,
				IgnoreError:             true,
			})
		}

		if len(routers) > 0 {
			router = routinghelpers.NewComposableParallel(routers)
		}
	}

	return router
}

func setupRouting(ctx context.Context, cfg Config, h host.Host, ds datastore.Batching, dhtRcMgr network.ResourceManager, bwc metrics.Reporter, dnsCache *cachedDNS) (routing.ContentRouting, routing.PeerRouting, routing.ValueStore, error) {
	delegatedRouters, err := setupDelegatedRouting(cfg, dnsCache)
	if err != nil {
		return nil, nil, nil, err
	}

	dhtRouter, err := setupDHTRouting(ctx, cfg, h, ds, dhtRcMgr, bwc)
	if err != nil {
		return nil, nil, nil, err
	}

	router := setupCompositeRouting(delegatedRouters, dhtRouter, cfg)

	var (
		cr routing.ContentRouting = router
		pr routing.PeerRouting    = router
		vs routing.ValueStore     = router
	)

	// If we're using a remote backend, but we also have libp2p enabled (e.g. for
	// seed peering), we can still leverage the remote backend here.
	if len(cfg.RemoteBackends) > 0 && cfg.RemoteBackendsIPNS {
		remoteValueStore, err := gateway.NewRemoteValueStore(cfg.RemoteBackends, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		vs = setupCompositeRouting(append(delegatedRouters, &routinghelpers.Compose{
			ValueStore: remoteValueStore,
		}), dhtRouter, cfg)
	}

	// If we're using seed peering, we need to run a lighter Amino DHT for the
	// peering routing. We need to run a separate DHT with the main host if
	//  the shared host is disabled, or if we're not running any DHT at all.
	if cfg.SeedPeering && (!cfg.DHTSharedHost || dhtRouter == nil) {
		// Parse bootstrap peers for seed peering DHT (don't warn on auto since it's expected)
		seedBootstrapPeers, err := parseBootstrapPeers(cfg.Bootstrap, false)
		if err != nil {
			return nil, nil, nil, err
		}

		// Use provided bootstrap peers or fall back to defaults
		dhtOpts := []dht.Option{
			dht.Datastore(ds),
			dht.Mode(dht.ModeClient),
		}
		if len(seedBootstrapPeers) > 0 {
			dhtOpts = append(dhtOpts, dht.BootstrapPeers(seedBootstrapPeers...))
		} else {
			// Use default bootstrap peers if none provided
			dhtOpts = append(dhtOpts, dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...))
		}

		pr, err = dht.New(ctx, h, dhtOpts...)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return cr, pr, vs, nil
}

func setupRoutingNoLibp2p(cfg Config, dnsCache *cachedDNS) (routing.ValueStore, error) {
	delegatedRouters, err := setupDelegatedRouting(cfg, dnsCache)
	if err != nil {
		return nil, err
	}

	if len(cfg.RemoteBackends) > 0 && cfg.RemoteBackendsIPNS {
		remoteValueStore, err := gateway.NewRemoteValueStore(cfg.RemoteBackends, nil)
		if err != nil {
			return nil, err
		}
		delegatedRouters = append(delegatedRouters, &routinghelpers.Compose{
			ValueStore: remoteValueStore,
		})
	}

	return setupCompositeRouting(delegatedRouters, nil, cfg), nil
}

type bundledDHT struct {
	standard *dht.IpfsDHT
	fullRT   *fullrt.FullRT
}

func (b *bundledDHT) getDHT() routing.Routing {
	if b.fullRT.Ready() {
		return b.fullRT
	}
	return b.standard
}

func (b *bundledDHT) Provide(ctx context.Context, c cid.Cid, brdcst bool) error {
	return b.getDHT().Provide(ctx, c, brdcst)
}

func (b *bundledDHT) FindProvidersAsync(ctx context.Context, c cid.Cid, i int) <-chan peer.AddrInfo {
	return b.getDHT().FindProvidersAsync(ctx, c, i)
}

func (b *bundledDHT) FindPeer(ctx context.Context, id peer.ID) (peer.AddrInfo, error) {
	return b.getDHT().FindPeer(ctx, id)
}

func (b *bundledDHT) PutValue(ctx context.Context, k string, v []byte, option ...routing.Option) error {
	return b.getDHT().PutValue(ctx, k, v, option...)
}

func (b *bundledDHT) GetValue(ctx context.Context, s string, option ...routing.Option) ([]byte, error) {
	return b.getDHT().GetValue(ctx, s, option...)
}

func (b *bundledDHT) SearchValue(ctx context.Context, s string, option ...routing.Option) (<-chan []byte, error) {
	return b.getDHT().SearchValue(ctx, s, option...)
}

func (b *bundledDHT) Bootstrap(ctx context.Context) error {
	return b.standard.Bootstrap(ctx)
}

var _ routing.Routing = (*bundledDHT)(nil)

// delegatedHTTPContentRouterWithCapabilities creates a routing client with selective capabilities
func delegatedHTTPContentRouterWithCapabilities(baseURL string, capabilities autoconf.EndpointCapabilities, rv1Opts ...routingv1client.Option) (routing.Routing, error) {
	// Create the HTTP routing client with base URL
	cli, err := routingv1client.New(
		baseURL,
		append([]routingv1client.Option{
			routingv1client.WithUserAgent("rainbow/" + buildVersion()),
		}, rv1Opts...)...,
	)
	if err != nil {
		return nil, err
	}

	cr := httpcontentrouter.NewContentRoutingClient(cli)

	err = view.Register(routingv1client.OpenCensusViews...)
	if err != nil {
		return nil, fmt.Errorf("registering HTTP delegated routing views: %w", err)
	}

	// Create a composed router with selective capabilities
	composer := &routinghelpers.Compose{}

	// Enable operations based on capabilities
	if capabilities.IPNSGet {
		composer.ValueStore = cr
	}

	if capabilities.Peers {
		composer.PeerRouting = cr
	}

	if capabilities.Providers {
		composer.ContentRouting = cr
	}

	// Note: Rainbow doesn't support IPNS publishing (IPNSPut)
	// so we don't need to handle that capability here

	return composer, nil
}
