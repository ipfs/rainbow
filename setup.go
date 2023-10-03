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
	"github.com/ipfs/boxo/namesys"
	routingv1client "github.com/ipfs/boxo/routing/http/client"
	httpcontentrouter "github.com/ipfs/boxo/routing/http/contentrouter"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	levelds "github.com/ipfs/go-ds-leveldb"
	logging "github.com/ipfs/go-log/v2"
	metri "github.com/ipfs/go-metrics-interface"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

var log = logging.Logger("rainbow")

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}
}

type Node struct {
	vs   routing.ValueStore
	host host.Host

	datastore  datastore.Batching
	blockstore blockstore.Blockstore
	bsClient   *bsclient.Client
	bsrv       blockservice.BlockService

	ns namesys.NameSystem

	bwc *metrics.BandwidthCounter
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

	RoutingV1 string
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

	r1, err := routingv1client.New(cfg.RoutingV1, routingv1client.WithStreamResultsRequired())
	if err != nil {
		return nil, err
	}

	vs := httpcontentrouter.NewContentRoutingClient(r1)

	opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		return vs, nil
	}))
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	bn := bsnet.NewFromIpfsHost(h, vs)
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

func setupHandler(nd *Node) (http.Handler, error) {
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
			Paths:                 []string{"/ipfs", "/ipns", "/version"},
			NoDNSLink:             noDNSLink,
			InlineDNSLink:         false,
			DeserializedResponses: true,
			UseSubdomains:         true,
		},
	}

	gwConf := gateway.Config{
		DeserializedResponses: true,
		Headers:               headers,
		PublicGateways:        publicGateways,
		NoDNSLink:             noDNSLink,
	}
	gwHandler := gateway.NewHandler(gwConf, backend)

	topMux := http.NewServeMux()
	hostNameMux := http.NewServeMux()
	topMux.Handle("/", gateway.NewHostnameHandler(gwConf, backend, hostNameMux))
	hostNameMux.Handle("/ipfs/", gwHandler)
	hostNameMux.Handle("/ipns/", gwHandler)
	hostNameMux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Client: %s\n", name)
		fmt.Fprintf(w, "Version: %s\n", version)
	})

	handler := withConnect(topMux)

	return handler, nil
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
