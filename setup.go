package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
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
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	levelds "github.com/ipfs/go-ds-leveldb"
	metri "github.com/ipfs/go-metrics-interface"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/fullrt"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/multierr"
)

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}
}

type Node struct {
	router routing.Routing
	host   host.Host

	datastore  datastore.Batching
	blockstore blockstore.Blockstore
	bsClient   *bsclient.Client
	bsrv       blockservice.BlockService

	ns       namesys.NameSystem
	kuboRPCs []string
	dns      *cachedDNS

	bwc *metrics.BandwidthCounter
}

// Close closes services run by the Node.
func (nd *Node) Close() error {
	return multierr.Combine(
		nd.host.Close(),
		nd.dns.Close(),
	)
}

type Config struct {
	ListenAddrs   []string
	AnnounceAddrs []string

	Blockstore string

	Libp2pKeyFile string

	Datastore string

	ConnMgrLow   int
	ConnMgrHi    int
	ConnMgrGrace time.Duration

	KuboRPCURLs []string
	RoutingV1   string
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

	var finalRouter routing.Routing
	cdns := newCachedDNS(dnsCacheRefreshInterval)

	opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		finalRouter, err = newRouting(h, cfg, ds, cdns)
		if err != nil {
			return nil, err
		}
		return finalRouter, nil
	}))
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	bn := bsnet.NewFromIpfsHost(h, finalRouter)
	bswap := bsclient.New(bsctx, bn, blkst)
	bn.Start(bswap)

	bsrv := blockservice.New(blkst, bswap)

	dns, err := gateway.NewDNSResolver(nil)
	if err != nil {
		return nil, err
	}

	ns, err := namesys.NewNameSystem(finalRouter, namesys.WithDNSResolver(dns))
	if err != nil {
		return nil, err
	}

	return &Node{
		host:       h,
		blockstore: blkst,
		datastore:  ds,
		bsClient:   bswap,
		ns:         ns,
		router:     finalRouter,
		bsrv:       bsrv,
		bwc:        bwc,
		dns:        cdns,
	}, nil
}

func newRouting(h host.Host, cfg *Config, ds datastore.Batching, cdns *cachedDNS) (routing.Routing, error) {
	var baseRouting routing.Routing
	var err error

	if cfg.RoutingV1 != "" {
		r1, err := routingv1client.New(cfg.RoutingV1, routingv1client.WithStreamResultsRequired())
		if err != nil {
			return nil, err
		}
		v1cr := httpcontentrouter.NewContentRoutingClient(r1)
		baseRouting = &httpRoutingWrapper{
			ContentRouting:    v1cr,
			PeerRouting:       v1cr,
			ValueStore:        v1cr,
			ProvideManyRouter: v1cr,
		}
	} else {
		baseRouting, err = fullrt.NewFullRT(h,
			dht.DefaultPrefix,
			fullrt.DHTOption(
				dht.NamespacedValidator("pk", record.PublicKeyValidator{}),
				dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: h.Peerstore()}),
				dht.Datastore(ds),
				dht.BucketSize(20),
			),
		)
		if err != nil {
			return nil, err
		}
	}

	finalRouter := &Composer{
		GetValueRouter:      baseRouting,
		PutValueRouter:      baseRouting,
		FindPeersRouter:     baseRouting,
		FindProvidersRouter: baseRouting,
		ProvideRouter:       baseRouting,
	}

	if len(cfg.KuboRPCURLs) > 0 {
		proxyRouting := newProxyRouting(cfg.KuboRPCURLs, cdns)
		finalRouter.GetValueRouter = proxyRouting
		finalRouter.PutValueRouter = proxyRouting
	}
	return finalRouter, nil
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
