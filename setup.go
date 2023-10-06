package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"net/http"
	"os"
	"time"

	bsclient "github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/boxo/namesys"
	routingv1client "github.com/ipfs/boxo/routing/http/client"
	httpcontentrouter "github.com/ipfs/boxo/routing/http/contentrouter"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	levelds "github.com/ipfs/go-ds-leveldb"
	metri "github.com/ipfs/go-metrics-interface"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/fullrt"
	record "github.com/libp2p/go-libp2p-record"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"go.opencensus.io/stats/view"
)

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}
}

const ipniFallbackEndpoint = "https://cid.contact"

type Node struct {
	vs   routing.ValueStore
	host host.Host

	datastore  datastore.Batching
	blockstore blockstore.Blockstore
	bsClient   *bsclient.Client
	bsrv       blockservice.BlockService

	ns       namesys.NameSystem
	kuboRPCs []string

	bwc *metrics.BandwidthCounter
}

type DHTType int

const (
	Combined DHTType = iota
	Standard
	Accelerated
)

type Config struct {
	ListenAddrs   []string
	AnnounceAddrs []string

	Blockstore string

	Libp2pKeyFile string

	Datastore string

	ConnMgrLow   int
	ConnMgrHi    int
	ConnMgrGrace time.Duration

	RoutingV1     string
	KuboRPCURLs   []string
	DHTSharedHost bool
	DHTType       DHTType
}

func Setup(ctx context.Context, cfg *Config) (*Node, error) {
	peerkey, err := loadOrInitPeerKey(cfg.Libp2pKeyFile)
	if err != nil {
		return nil, err
	}

	ds, err := levelds.NewDatastore(cfg.Datastore, nil)
	if err != nil {
		return nil, err
	}

	bwc := metrics.NewBandwidthCounter()

	cmgr, err := connmgr.NewConnManager(cfg.ConnMgrLow, cfg.ConnMgrHi, connmgr.WithGracePeriod(cfg.ConnMgrGrace))
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(cmgr),
		libp2p.Identity(peerkey),
		libp2p.BandwidthReporter(bwc),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
	}

	if len(cfg.AnnounceAddrs) > 0 {
		var addrs []multiaddr.Multiaddr
		for _, anna := range cfg.AnnounceAddrs {
			a, err := multiaddr.NewMultiaddr(anna)
			if err != nil {
				return nil, fmt.Errorf("failed to parse announce addr: %w", err)
			}
			addrs = append(addrs, a)
		}
		opts = append(opts, libp2p.AddrsFactory(func([]multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return addrs
		}))
	}

	blkst, err := loadBlockstore("flatfs", cfg.Blockstore)
	if err != nil {
		return nil, err
	}
	blkst = blockstore.NewIdStore(blkst)

	bsctx := metri.CtxScope(ctx, "rainbow")

	var pr routing.PeerRouting
	var vs routing.ValueStore
	var cr routing.ContentRouting

	opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		if cfg.RoutingV1 != "" {
			routingClient, err := delegatedHTTPContentRouter(cfg.RoutingV1, routingv1client.WithStreamResultsRequired())
			if err != nil {
				return nil, err
			}
			pr = routingClient
			vs = routingClient
			cr = routingClient
		} else {
			// If there are no delegated routing endpoints run an accelerated Amino DHT client and send IPNI requests to cid.contact

			// TODO: This datastore shouldn't end up containing anything anyway so this could potentially just be a null datastore
			memDS, err := levelds.NewDatastore("", nil)
			if err != nil {
				return nil, err
			}

			var dhtHost host.Host
			if cfg.DHTSharedHost {
				dhtHost = h
			} else {
				dhtHost, err = libp2p.New(
					libp2p.NoListenAddrs,
					libp2p.BandwidthReporter(bwc),
					libp2p.DefaultTransports,
					libp2p.DefaultMuxers,
				)
				if err != nil {
					return nil, err
				}
			}

			var standardClient *dht.IpfsDHT
			var fullRTClient *fullrt.FullRT

			if cfg.DHTType == Combined || cfg.DHTType == Standard {
				standardClient, err = dht.New(ctx, dhtHost,
					dht.Datastore(memDS),
					dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
					dht.Mode(dht.ModeClient),
				)
				if err != nil {
					return nil, err
				}
			}

			if cfg.DHTType == Combined || cfg.DHTType == Accelerated {
				fullRTClient, err = fullrt.NewFullRT(dhtHost, dht.DefaultPrefix,
					fullrt.DHTOption(
						dht.Validator(record.NamespacedValidator{
							"pk":   record.PublicKeyValidator{},
							"ipns": ipns.Validator{KeyBook: h.Peerstore()},
						}),
						dht.Datastore(memDS),
						dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
						dht.BucketSize(20),
					))
				if err != nil {
					return nil, err
				}
			}

			var dhtRouter routing.Routing
			switch cfg.DHTType {
			case Combined:
				dhtRouter = &bundledDHT{
					standard: standardClient,
					fullRT:   fullRTClient,
				}
			case Standard:
				dhtRouter = standardClient
			case Accelerated:
				dhtRouter = fullRTClient
			default:
				return nil, fmt.Errorf("unsupported DHT type")
			}

			// we want to also use the default HTTP routers, so wrap the FullRT client
			// in a parallel router that calls them in parallel
			httpRouters, err := delegatedHTTPContentRouter(ipniFallbackEndpoint)
			if err != nil {
				return nil, err
			}
			routers := []*routinghelpers.ParallelRouter{
				{
					Router:                  dhtRouter,
					ExecuteAfter:            0,
					DoNotWaitForSearchValue: true,
					IgnoreError:             false,
				},
				{
					Timeout:                 15 * time.Second,
					Router:                  httpRouters,
					ExecuteAfter:            0,
					DoNotWaitForSearchValue: true,
					IgnoreError:             true,
				},
			}
			router := routinghelpers.NewComposableParallel(routers)

			pr = router
			vs = router
			cr = router
		}

		return pr, nil
	}))
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	bn := bsnet.NewFromIpfsHost(h, cr)
	bswap := bsclient.New(bsctx, bn, blkst)
	bn.Start(bswap)

	bsrv := blockservice.New(blkst, bswap)

	dns, err := gateway.NewDNSResolver(nil)
	if err != nil {
		return nil, err
	}
	ns, err := namesys.NewNameSystem(vs, namesys.WithDNSResolver(dns))
	if err != nil {
		return nil, err
	}

	return &Node{
		host:       h,
		blockstore: blkst,
		datastore:  ds,
		bsClient:   bswap,
		ns:         ns,
		vs:         vs,
		bsrv:       bsrv,
		bwc:        bwc,
		kuboRPCs:   cfg.KuboRPCURLs,
	}, nil
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
	// Increase per-host connection pool since we are making lots of concurrent requests.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 500
	transport.MaxIdleConnsPerHost = 100

	delegateHTTPClient := &http.Client{
		Transport: &routingv1client.ResponseBodyLimitedTransport{
			RoundTripper: transport,
			LimitBytes:   1 << 20,
		},
	}

	cli, err := routingv1client.New(
		endpoint,
		append([]routingv1client.Option{
			routingv1client.WithHTTPClient(delegateHTTPClient),
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

func loadBlockstore(spec string, path string) (blockstore.Blockstore, error) {
	switch spec {
	case "flatfs":
		sf, err := flatfs.ParseShardFunc("/repo/flatfs/shard/v1/next-to-last/3")
		if err != nil {
			return nil, err
		}

		ds, err := flatfs.CreateOrOpen(path, sf, false)
		if err != nil {
			return nil, err
		}

		return blockstore.NewBlockstore(ds, blockstore.NoPrefix()), nil
	default:
		return nil, fmt.Errorf("unsupported blockstore type: %s", spec)
	}
}

func loadOrInitPeerKey(kf string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(kf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		k, _, err := crypto.GenerateEd25519Key(crand.Reader)
		if err != nil {
			return nil, err
		}

		data, err := crypto.MarshalPrivateKey(k)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(kf, data, 0600); err != nil {
			return nil, err
		}

		return k, nil
	}
	return crypto.UnmarshalPrivateKey(data)
}
