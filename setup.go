package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	nopfs "github.com/ipfs-shipyard/nopfs"
	nopfsipfs "github.com/ipfs-shipyard/nopfs/ipfs"
	"github.com/ipfs/boxo/bitswap"
	bsnet "github.com/ipfs/boxo/bitswap/network"
	bsserver "github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	bsfetcher "github.com/ipfs/boxo/fetcher/impl/blockservice"
	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/boxo/namesys"
	"github.com/ipfs/boxo/path/resolver"
	"github.com/ipfs/boxo/peering"
	routingv1client "github.com/ipfs/boxo/routing/http/client"
	httpcontentrouter "github.com/ipfs/boxo/routing/http/contentrouter"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	badger4 "github.com/ipfs/go-ds-badger4"
	flatfs "github.com/ipfs/go-ds-flatfs"
	delay "github.com/ipfs/go-ipfs-delay"
	metri "github.com/ipfs/go-metrics-interface"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/ipfs/go-unixfsnode"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/fullrt"
	record "github.com/libp2p/go-libp2p-record"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/multiformats/go-multiaddr"
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}
}

const cidContactEndpoint = "https://cid.contact"

type DHTRouting string

const (
	DHTAccelerated DHTRouting = "accelerated"
	DHTStandard    DHTRouting = "standard"
	DHTOff         DHTRouting = "off"
)

func init() {
	// Lets us discover our own public address with a single observation
	identify.ActivationThresh = 1
}

type Node struct {
	vs   routing.ValueStore
	host host.Host

	dataDir      string
	datastore    datastore.Batching
	blockstore   blockstore.Blockstore
	bs           *bitswap.Bitswap
	bsrv         blockservice.BlockService
	resolver     resolver.Resolver
	ns           namesys.NameSystem
	denylistSubs []*nopfs.HTTPSubscriber
}

type Config struct {
	DataDir        string
	BlockstoreType string

	ListenAddrs   []string
	AnnounceAddrs []string

	ConnMgrLow   int
	ConnMgrHi    int
	ConnMgrGrace time.Duration

	InMemBlockCache int64
	MaxMemory       uint64
	MaxFD           int

	GatewayDomains          []string
	SubdomainGatewayDomains []string
	TrustlessGatewayDomains []string
	RoutingV1Endpoints      []string
	DHTRouting              DHTRouting
	DHTSharedHost           bool
	IpnsMaxCacheTTL         time.Duration

	DenylistSubs []string
	Peering      []peer.AddrInfo
	PeeringCache bool

	Seed                string
	SeedIndex           int
	SeedPeering         bool
	SeedPeeringMaxIndex int

	GCInterval  time.Duration
	GCThreshold float64
}

func Setup(ctx context.Context, cfg Config, key crypto.PrivKey, dnsCache *cachedDNS) (*Node, error) {
	var err error

	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	ds, err := setupDatastore(cfg)
	if err != nil {
		return nil, err
	}

	bwc := metrics.NewBandwidthCounter()

	cmgr, err := connmgr.NewConnManager(cfg.ConnMgrLow, cfg.ConnMgrHi, connmgr.WithGracePeriod(cfg.ConnMgrGrace))
	if err != nil {
		return nil, err
	}

	bitswapRcMgr, dhtRcMgr, err := makeResourceMgrs(cfg.MaxMemory, cfg.MaxFD, cfg.ConnMgrHi, !cfg.DHTSharedHost)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(cmgr),
		libp2p.Identity(key),
		libp2p.UserAgent("rainbow/" + buildVersion()),
		libp2p.BandwidthReporter(bwc),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.ResourceManager(bitswapRcMgr),
		libp2p.EnableHolePunching(),
	}

	if len(cfg.ListenAddrs) == 0 {
		// Note: because the transports are set above we must also set the listen addresses
		// We need to set listen addresses in order for hole punching to work
		opts = append(opts, libp2p.DefaultListenAddrs)
	} else {
		opts = append(opts, libp2p.ListenAddrStrings(cfg.ListenAddrs...))
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

	blkst := blockstore.NewBlockstore(ds,
		blockstore.NoPrefix(),
		// Every Has() for every written block is a transaction with a
		// seek onto LSM. If not in memory it will be a pain.
		// We opt to write every block Put into the blockstore.
		// See also comment in blockservice.
		blockstore.WriteThrough(),
	)
	blkst = blockstore.NewIdStore(blkst)

	var (
		cr routing.ContentRouting
		pr routing.PeerRouting
		vs routing.ValueStore
	)

	opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		cr, pr, vs, err = setupRouting(ctx, cfg, h, ds, dhtRcMgr, bwc, dnsCache)
		return pr, err
	}))
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	err = setupPeering(cfg, h)
	if err != nil {
		return nil, err
	}

	bswap := setupBitswap(ctx, cfg, h, cr, blkst)

	err = os.Mkdir(filepath.Join(cfg.DataDir, "denylists"), 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	var denylists []*nopfs.HTTPSubscriber
	for _, dl := range cfg.DenylistSubs {
		s, err := nopfs.NewHTTPSubscriber(dl, filepath.Join(cfg.DataDir, "denylists", filepath.Base(dl)), time.Minute)
		if err != nil {
			return nil, err
		}
		denylists = append(denylists, s)
	}

	files, err := nopfs.GetDenylistFilesInDir(filepath.Join(cfg.DataDir, "denylists"))
	if err != nil {
		return nil, err
	}
	blocker, err := nopfs.NewBlocker(files)
	if err != nil {
		return nil, err
	}

	bsrv := blockservice.New(blkst, bswap,
		// if we are doing things right, our bitswap wantlists should
		// not have blocks that we already have (see
		// https://github.com/ipfs/boxo/blob/e0d4b3e9b91e9904066a10278e366c9a6d9645c7/blockservice/blockservice.go#L272). Thus
		// we should not be writing many blocks that we already
		// have. Thus, no point in checking whether we have a block
		// before writing new blocks.
		blockservice.WriteThrough(),
	)
	bsrv = nopfsipfs.WrapBlockService(bsrv, blocker)

	dns, err := gateway.NewDNSResolver(nil)
	if err != nil {
		return nil, err
	}
	nsOptions := []namesys.Option{namesys.WithDNSResolver(dns)}
	if cfg.IpnsMaxCacheTTL > 0 {
		nsOptions = append(nsOptions, namesys.WithMaxCacheTTL(cfg.IpnsMaxCacheTTL))
	}
	ns, err := namesys.NewNameSystem(vs, nsOptions...)
	if err != nil {
		return nil, err
	}
	ns = nopfsipfs.WrapNameSystem(ns, blocker)

	fetcherCfg := bsfetcher.NewFetcherConfig(bsrv)
	fetcherCfg.PrototypeChooser = dagpb.AddSupportToChooser(bsfetcher.DefaultPrototypeChooser)
	fetcher := fetcherCfg.WithReifier(unixfsnode.Reify)
	r := resolver.NewBasicResolver(fetcher)
	r = nopfsipfs.WrapResolver(r, blocker)

	return &Node{
		host:         h,
		blockstore:   blkst,
		dataDir:      cfg.DataDir,
		datastore:    ds,
		bs:           bswap,
		ns:           ns,
		vs:           vs,
		bsrv:         bsrv,
		resolver:     r,
		denylistSubs: denylists,
	}, nil
}

func setupDatastore(cfg Config) (datastore.Batching, error) {
	switch cfg.BlockstoreType {
	case "flatfs":
		return flatfs.CreateOrOpen(filepath.Join(cfg.DataDir, "flatfs"), flatfs.NextToLast(3), false)
	case "badger":
		badgerOpts := badger.DefaultOptions("")
		badgerOpts.CompactL0OnClose = false
		// ValueThreshold: defaults to 1MB! For us that means everything goes
		// into the LSM tree and that means more stuff in memory.  We only
		// put very small things on the LSM tree by default (i.e. a single
		// CID).
		badgerOpts.ValueThreshold = 256

		// BlockCacheSize: instead of using blockstore, we cache things
		// here. This only makes sense if using compression, according to
		// docs.
		badgerOpts.BlockCacheSize = cfg.InMemBlockCache // default 1 GiB.

		// Compression: default. Trades reading less from disk for using more
		// CPU. Given gateways are usually IO bound, I think we can make this
		// trade.
		if badgerOpts.BlockCacheSize == 0 {
			badgerOpts.Compression = options.None
		} else {
			badgerOpts.Compression = options.Snappy
		}

		// If we write something twice, we do it with the same values so
		// *shrugh*.
		badgerOpts.DetectConflicts = false

		// MemTableSize: Defaults to 64MiB which seems an ok amount to flush
		// to disk from time to time.
		badgerOpts.MemTableSize = 64 << 20
		// NumMemtables: more means more memory, faster writes, but more to
		// commit to disk if they get full. Default is 5.
		badgerOpts.NumMemtables = 5

		// IndexCacheSize: 0 means all in memory (default). All means indexes,
		// bloom filters etc. Usually not huge amount of memory usage from
		// this.
		badgerOpts.IndexCacheSize = 0

		opts := badger4.Options{
			GcDiscardRatio: 0.3,
			GcInterval:     20 * time.Minute,
			GcSleep:        10 * time.Second,
			Options:        badgerOpts,
		}

		return badger4.NewDatastore(filepath.Join(cfg.DataDir, "badger4"), &opts)
	default:
		return nil, fmt.Errorf("unsupported blockstore type: %s", cfg.BlockstoreType)
	}
}

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

func setupPeering(cfg Config, h host.Host) error {
	if len(cfg.Peering) == 0 && !cfg.SeedPeering {
		return nil
	}

	ps := peering.NewPeeringService(h)
	if err := ps.Start(); err != nil {
		return err
	}
	for _, a := range cfg.Peering {
		ps.AddPeer(a)
	}

	if !cfg.SeedPeering {
		return nil
	}

	if cfg.SeedIndex < 0 {
		return fmt.Errorf("seed index must be equal or greater than 0, it is %d", cfg.SeedIndex)
	}

	if cfg.SeedPeeringMaxIndex < 0 {
		return fmt.Errorf("seed peering max index must be a positive number, it is %d", cfg.SeedPeeringMaxIndex)
	}

	pids, err := derivePeerIDs(cfg.Seed, cfg.SeedIndex, cfg.SeedPeeringMaxIndex)
	if err != nil {
		return err
	}

	for _, pid := range pids {
		// The peering module will automatically perform lookups to find the
		// addresses of the given peers.
		ps.AddPeer(peer.AddrInfo{ID: pid})
	}

	return nil
}

func setupBitswap(ctx context.Context, cfg Config, h host.Host, cr routing.ContentRouting, bstore blockstore.Blockstore) *bitswap.Bitswap {
	var (
		provideEnabled         bool
		peerBlockRequestFilter bsserver.PeerBlockRequestFilter
	)
	if cfg.PeeringCache && len(cfg.Peering) > 0 {
		peers := make(map[peer.ID]struct{}, len(cfg.Peering))
		for _, a := range cfg.Peering {
			peers[a.ID] = struct{}{}
		}

		provideEnabled = true
		peerBlockRequestFilter = func(p peer.ID, c cid.Cid) bool {
			_, ok := peers[p]
			return ok
		}
	} else {
		provideEnabled = false
		peerBlockRequestFilter = func(p peer.ID, c cid.Cid) bool {
			return false
		}
	}

	bsctx := metri.CtxScope(ctx, "ipfs_bitswap")
	bn := bsnet.NewFromIpfsHost(h, cr)
	bswap := bitswap.New(bsctx, bn, bstore,
		// --- Client Options
		// default is 1 minute to search for a random live-want (1
		// CID).  I think we want to search for random live-wants more
		// often although probably it overlaps with general
		// rebroadcasts.
		bitswap.RebroadcastDelay(delay.Fixed(10*time.Second)),
		// ProviderSearchDelay: default is 1 second.
		bitswap.ProviderSearchDelay(time.Second),
		bitswap.WithoutDuplicatedBlockStats(),

		// ---- Server Options
		bitswap.WithPeerBlockRequestFilter(peerBlockRequestFilter),
		bitswap.ProvideEnabled(provideEnabled),
	)
	bn.Start(bswap)

	return bswap
}
