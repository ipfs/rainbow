package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func setupDelegatedRouting(cfg Config, dnsCache *cachedDNS) ([]routing.Routing, error) {
	// Increase per-host connection pool since we are making lots of concurrent requests.
	httpClient := &http.Client{
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

	var (
		delegatedRouters []routing.Routing
	)

	for _, endpoint := range cfg.RoutingV1Endpoints {
		rv1Opts := []routingv1client.Option{routingv1client.WithHTTPClient(httpClient)}
		if endpoint != cidContactEndpoint {
			rv1Opts = append(rv1Opts, routingv1client.WithStreamResultsRequired())
		}
		delegatedRouter, err := delegatedHTTPContentRouter(endpoint, rv1Opts...)
		if err != nil {
			return nil, err
		}
		delegatedRouters = append(delegatedRouters, delegatedRouter)
	}

	return delegatedRouters, nil
}

func setupDHTRouting(ctx context.Context, cfg Config, h host.Host, ds datastore.Batching, dhtRcMgr network.ResourceManager, bwc metrics.Reporter) (routing.Routing, error) {
	if cfg.DHTRouting == DHTOff {
		return nil, nil
	}

	var err error

	var dhtHost host.Host
	if cfg.DHTSharedHost {
		dhtHost = h
	} else {
		dhtHost, err = libp2p.New(
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
		dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
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
				dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
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

func setupCompositeRouting(delegatedRouters []routing.Routing, dht routing.Routing) routing.Routing {
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

		for _, routingV1Router := range delegatedRouters {
			routers = append(routers, &routinghelpers.ParallelRouter{
				Timeout:                 15 * time.Second,
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

	router := setupCompositeRouting(delegatedRouters, dhtRouter)

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
		}), dhtRouter)
	}

	// If we're using seed peering, we need to run a lighter Amino DHT for the
	// peering routing. We need to run a separate DHT with the main host if
	//  the shared host is disabled, or if we're not running any DHT at all.
	if cfg.SeedPeering && (!cfg.DHTSharedHost || dhtRouter == nil) {
		pr, err = dht.New(ctx, h,
			dht.Datastore(ds),
			dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
			dht.Mode(dht.ModeClient),
		)
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

	return setupCompositeRouting(delegatedRouters, nil), nil
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

func delegatedHTTPContentRouter(endpoint string, rv1Opts ...routingv1client.Option) (routing.Routing, error) {
	cli, err := routingv1client.New(
		endpoint,
		append([]routingv1client.Option{
			routingv1client.WithUserAgent(buildVersion()),
		}, rv1Opts...)...,
	)
	if err != nil {
		return nil, err
	}

	cr := httpcontentrouter.NewContentRoutingClient(
		cli,
	)

	err = view.Register(routingv1client.OpenCensusViews...)
	if err != nil {
		return nil, fmt.Errorf("registering HTTP delegated routing views: %w", err)
	}

	return &routinghelpers.Compose{
		ValueStore:     cr,
		PeerRouting:    cr,
		ContentRouting: cr,
	}, nil
}
