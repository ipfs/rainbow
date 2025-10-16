package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble/v2"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	nopfs "github.com/ipfs-shipyard/nopfs"
	nopfsipfs "github.com/ipfs-shipyard/nopfs/ipfs"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	bsfetcher "github.com/ipfs/boxo/fetcher/impl/blockservice"
	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/namesys"
	"github.com/ipfs/boxo/path/resolver"
	"github.com/ipfs/boxo/peering"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	badger4 "github.com/ipfs/go-ds-badger4"
	flatfs "github.com/ipfs/go-ds-flatfs"
	pebbleds "github.com/ipfs/go-ds-pebble"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/ipfs/go-unixfsnode"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}
}

const httpRouterGatewayProtocol = "transport-ipfs-gateway-http"

var httpRoutersFilterProtocols = []string{"unknown", "transport-bitswap"} // IPIP-484

type DHTRouting string

const (
	DHTAccelerated DHTRouting = "accelerated"
	DHTStandard    DHTRouting = "standard"
	DHTOff         DHTRouting = "off"
)

type RemoteBackendMode string

const (
	RemoteBackendBlock RemoteBackendMode = "block"
	RemoteBackendCAR   RemoteBackendMode = "car"
)

func init() {
	// Lets us discover our own public address with a single observation
	identify.ActivationThresh = 1
}

type Node struct {
	ns           namesys.NameSystem
	vs           routing.ValueStore
	dataDir      string
	bsrv         blockservice.BlockService
	denylistSubs []*nopfs.HTTPSubscriber

	// Maybe not be set depending on the configuration:
	host       host.Host
	datastore  datastore.Batching
	blockstore blockstore.Blockstore
	resolver   resolver.Resolver
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

	GatewayDomains           []string
	SubdomainGatewayDomains  []string
	TrustlessGatewayDomains  []string
	RoutingV1Endpoints       []string
	RoutingV1FilterProtocols []string
	HTTPRoutersTimeout       time.Duration
	RoutingTimeout           time.Duration
	RoutingIgnoreProviders   []peer.ID
	DHTRouting               DHTRouting
	DHTSharedHost            bool
	IpnsMaxCacheTTL          time.Duration
	Bitswap                  bool

	DNSLinkResolver madns.BasicResolver

	// BitswapWantHaveReplaceSize tells the bitswap server to replace WantHave
	// with WantBlock responses when the block size less then or equal to this
	// value. Set to zero to disable replacement and avoid block size lookup
	// when processing HaveWant requests.
	BitswapWantHaveReplaceSize int
	BitswapEnableDuplicateBlockStats bool

	DenylistSubs []string

	Peering            []peer.AddrInfo
	PeeringSharedCache bool

	Seed                string
	SeedIndex           int
	SeedPeering         bool
	SeedPeeringMaxIndex int

	RemoteBackends     []string
	RemoteBackendsIPNS bool
	RemoteBackendMode  RemoteBackendMode

	GCInterval  time.Duration
	GCThreshold float64

	TracingAuthToken string

	disableMetrics bool // only meant to be used during testing

	// Pebble config values
	BytesPerSync                int
	DisableWAL                  bool
	L0CompactionThreshold       int
	L0StopWritesThreshold       int
	LBaseMaxBytes               int64
	MemTableSize                uint64
	MemTableStopWritesThreshold int
	WALBytesPerSync             int
	MaxConcurrentCompactions    int
	WALMinSyncInterval          time.Duration

	// ProviderQueryManager configuration.
	RoutingMaxRequests  int
	RoutingMaxProviders int
	RoutingMaxTimeout   time.Duration

	// HTTP Retrieval configuration
	HTTPRetrievalEnable                    bool
	HTTPRetrievalAllowlist                 []string
	HTTPRetrievalDenylist                  []string
	HTTPRetrievalWorkers                   int
	HTTPRetrievalMaxDontHaveErrors         int
	HTTPRetrievalMetricsLabelsForEndpoints []string

	// AutoConf configuration
	AutoConf AutoConfConfig

	// Bootstrap peers configuration (with "auto" support)
	Bootstrap []string

	// Gateway limits
	MaxConcurrentRequests   int
	RetrievalTimeout        time.Duration
	MaxRangeRequestFileSize int64
	DiagnosticServiceURL    string
}

func SetupNoLibp2p(ctx context.Context, cfg Config, dnsCache *cachedDNS) (*Node, error) {
	var err error

	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	denylists, blocker, err := setupDenylists(cfg)
	if err != nil {
		return nil, err
	}

	// The stars aligned and Libp2p does not need to be turned on at all.
	if len(cfg.RemoteBackends) == 0 {
		return nil, errors.New("RAINBOW_REMOTE_BACKENDS must be set if RAINBOW_LIBP2P is disabled")
	}

	// Setup a Value Store composed of both the remote backends and the delegated
	// routers, if they exist. This vs is only used for resolving IPNS Records.
	vs, err := setupRoutingNoLibp2p(cfg, dnsCache)
	if err != nil {
		return nil, err
	}

	// Setup the remote blockstore if that's the mode we're using.
	var bsrv blockservice.BlockService
	if cfg.RemoteBackendMode == RemoteBackendBlock {
		blkst, err := gateway.NewRemoteBlockstore(cfg.RemoteBackends, nil)
		if err != nil {
			return nil, err
		}

		bsrv = blockservice.New(blkst, offline.Exchange(blkst))
		bsrv = nopfsipfs.WrapBlockService(bsrv, blocker)
	}

	ns, err := setupNamesys(cfg, vs, blocker, cfg.DNSLinkResolver)
	if err != nil {
		return nil, err
	}

	return &Node{
		vs:           vs,
		ns:           ns,
		dataDir:      cfg.DataDir,
		denylistSubs: denylists,
		bsrv:         bsrv,
	}, nil
}

func SetupWithLibp2p(ctx context.Context, cfg Config, key crypto.PrivKey, dnsCache *cachedDNS) (*Node, error) {
	if !cfg.Bitswap && cfg.DHTRouting == DHTOff {
		return nil, errors.New("libp2p is enabled, but not used: bitswap and dht are disabled")
	}

	var err error

	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	denylists, blocker, err := setupDenylists(cfg)
	if err != nil {
		return nil, err
	}

	n := &Node{
		dataDir:      cfg.DataDir,
		denylistSubs: denylists,
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

	ds, err := setupDatastore(cfg)
	if err != nil {
		return nil, err
	}

	var (
		vs routing.ValueStore
		cr routing.ContentRouting
		pr routing.PeerRouting
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

	var bsrv blockservice.BlockService
	if cfg.Bitswap {
		blkst := blockstore.NewBlockstore(ds,
			blockstore.NoPrefix(),
			// Every Has() for every written block is a transaction with a
			// seek onto LSM. If not in memory it will be a pain.
			// We opt to write every block Put into the blockstore.
			// See also comment in blockservice.
			blockstore.WriteThrough(true),
		)
		blkst = &switchingBlockstore{
			baseBlockstore:      blkst,
			contextSwitchingKey: NoBlockcache{},
		}
		blkst = blockstore.NewIdStore(blkst)
		n.blockstore = blkst

		bsrv = blockservice.New(blkst, setupBitswapExchange(ctx, cfg, h, cr, blkst),
			// if we are doing things right, our bitswap wantlists should
			// not have blocks that we already have (see
			// https://github.com/ipfs/boxo/blob/e0d4b3e9b91e9904066a10278e366c9a6d9645c7/blockservice/blockservice.go#L272). Thus
			// we should not be writing many blocks that we already
			// have. Thus, no point in checking whether we have a block
			// before writing new blocks.
			blockservice.WriteThrough(true),
		)
	} else {
		if len(cfg.RemoteBackends) == 0 || cfg.RemoteBackendMode != RemoteBackendBlock {
			return nil, errors.New("remote backends in block mode must be set when disabling bitswap")
		}

		if cfg.PeeringSharedCache {
			return nil, errors.New("disabling bitswap is incompatible with peering cache")
		}

		blkst, err := gateway.NewRemoteBlockstore(cfg.RemoteBackends, nil)
		if err != nil {
			return nil, err
		}

		bsrv = blockservice.New(blkst, offline.Exchange(blkst))
	}

	bsrv = nopfsipfs.WrapBlockService(bsrv, blocker)

	fetcherCfg := bsfetcher.NewFetcherConfig(bsrv)
	fetcherCfg.PrototypeChooser = dagpb.AddSupportToChooser(bsfetcher.DefaultPrototypeChooser)
	fetcher := fetcherCfg.WithReifier(unixfsnode.Reify)
	r := resolver.NewBasicResolver(fetcher)
	r = nopfsipfs.WrapResolver(r, blocker)

	n.host = h
	n.datastore = ds
	n.bsrv = bsrv
	n.resolver = r

	ns, err := setupNamesys(cfg, vs, blocker, cfg.DNSLinkResolver)
	if err != nil {
		return nil, err
	}

	n.vs = vs
	n.ns = ns

	return n, nil
}

func setupDatastore(cfg Config) (datastore.Batching, error) {
	switch cfg.BlockstoreType {
	case "flatfs":
		return flatfs.CreateOrOpen(filepath.Join(cfg.DataDir, "flatfs"), flatfs.NextToLast(3), false)
	case "pebble":
		return pebbleds.NewDatastore(filepath.Join(cfg.DataDir, "pebbleds"),
			pebbleds.WithCacheSize(cfg.InMemBlockCache),
			pebbleds.WithPebbleOpts(getPebbleOpts(cfg)),
		)
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

func setupDenylists(cfg Config) ([]*nopfs.HTTPSubscriber, *nopfs.Blocker, error) {
	err := os.Mkdir(filepath.Join(cfg.DataDir, "denylists"), 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, nil, err
	}

	var denylists []*nopfs.HTTPSubscriber
	for _, dl := range cfg.DenylistSubs {
		s, err := nopfs.NewHTTPSubscriber(dl, filepath.Join(cfg.DataDir, "denylists", filepath.Base(dl)), time.Minute)
		if err != nil {
			return nil, nil, err
		}
		denylists = append(denylists, s)
	}

	files, err := nopfs.GetDenylistFilesInDir(filepath.Join(cfg.DataDir, "denylists"))
	if err != nil {
		return nil, nil, err
	}
	blocker, err := nopfs.NewBlocker(files)
	if err != nil {
		return nil, nil, err
	}

	return denylists, blocker, nil
}

func setupNamesys(cfg Config, vs routing.ValueStore, blocker *nopfs.Blocker, dnslinkResolver madns.BasicResolver) (namesys.NameSystem, error) {
	nsOptions := []namesys.Option{namesys.WithDNSResolver(dnslinkResolver)}
	if cfg.IpnsMaxCacheTTL > 0 {
		nsOptions = append(nsOptions, namesys.WithMaxCacheTTL(cfg.IpnsMaxCacheTTL))
	}
	ns, err := namesys.NewNameSystem(vs, nsOptions...)
	if err != nil {
		return nil, err
	}
	ns = nopfsipfs.WrapNameSystem(ns, blocker)
	return ns, nil
}

type switchingBlockstore struct {
	baseBlockstore      blockstore.Blockstore
	contextSwitchingKey any
}

func (s *switchingBlockstore) getBlockstore(ctx context.Context) blockstore.Blockstore {
	alternativeBlockstore, ok := ctx.Value(s.contextSwitchingKey).(blockstore.Blockstore)
	if ok {
		return alternativeBlockstore
	}
	return s.baseBlockstore
}

func (s *switchingBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return s.getBlockstore(ctx).DeleteBlock(ctx, c)
}

func (s *switchingBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	return s.getBlockstore(ctx).Has(ctx, c)
}

func (s *switchingBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	return s.getBlockstore(ctx).Get(ctx, c)
}

func (s *switchingBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	return s.getBlockstore(ctx).GetSize(ctx, c)
}

func (s *switchingBlockstore) Put(ctx context.Context, block blocks.Block) error {
	return s.getBlockstore(ctx).Put(ctx, block)
}

func (s *switchingBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	return s.getBlockstore(ctx).PutMany(ctx, blocks)
}

func (s *switchingBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return s.getBlockstore(ctx).AllKeysChan(ctx)
}

var _ blockstore.Blockstore = (*switchingBlockstore)(nil)

func getPebbleOpts(cfg Config) *pebble.Options {
	opts := &pebble.Options{
		BytesPerSync:                cfg.BytesPerSync,
		DisableWAL:                  cfg.DisableWAL,
		FormatMajorVersion:          pebble.FormatNewest,
		L0CompactionThreshold:       cfg.L0CompactionThreshold,
		L0StopWritesThreshold:       cfg.L0StopWritesThreshold,
		LBaseMaxBytes:               cfg.LBaseMaxBytes,
		MemTableSize:                cfg.MemTableSize,
		MemTableStopWritesThreshold: cfg.MemTableStopWritesThreshold,
		WALBytesPerSync:             cfg.WALBytesPerSync,
	}
	if cfg.MaxConcurrentCompactions != 0 {
		opts.CompactionConcurrencyRange = func() (int, int) { return 1, cfg.MaxConcurrentCompactions }
	}
	if cfg.WALMinSyncInterval != 0 {
		opts.WALMinSyncInterval = func() time.Duration { return cfg.WALMinSyncInterval }
	}

	return opts
}
