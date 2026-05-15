package main

import (
	"context"
	"time"

	"github.com/ipfs/boxo/routing/providerquerymanager"

	"github.com/ipfs/boxo/bitswap"
	bsclient "github.com/ipfs/boxo/bitswap/client"
	"github.com/ipfs/boxo/bitswap/network"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/bitswap/network/httpnet"
	bsserver "github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	metri "github.com/ipfs/go-metrics-interface"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/routing"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// peerstoreMergingHost wraps the bitswap host so Connect sees addresses
// the DHT host has learned for a peer.
//
// BasicHost.Connect runs AddAddrs(pi.Addrs, TempAddrTTL) and then dials
// every address in the host's peerstore for that peer. Sharing a libp2p
// host (kubo, ipfs-check) means identify exchanges, DHT response messages,
// and DCUtR coordination all enrich the same peerstore that bitswap reads
// at dial time. Rainbow's --dht-shared-host=false default runs the DHT on
// a separate libp2p host, so that enrichment lands on a peerstore the
// bitswap host never reads. The wrapper bridges that gap.
//
// Only public addresses are copied. The DHT host's peerstore can hold
// loopback or RFC1918 entries that misconfigured peers stored in their
// routing tables; forwarding those would just waste resource-manager
// budget on dials that can never connect.
type peerstoreMergingHost struct {
	host.Host
	dhtAddrs peerstore.AddrBook
}

// Connect copies public DHT-known addresses for pi.ID into the main host's
// peerstore at TempAddrTTL (the same TTL BasicHost.Connect uses for the
// AddrInfo it receives), then delegates to the embedded host. Identify on
// the resulting connection refreshes the durable set on its own.
func (h *peerstoreMergingHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	if known := h.dhtAddrs.Addrs(pi.ID); len(known) > 0 {
		public := make([]ma.Multiaddr, 0, len(known))
		for _, a := range known {
			if manet.IsPublicAddr(a) {
				public = append(public, a)
			}
		}
		if len(public) > 0 {
			h.Host.Peerstore().AddAddrs(pi.ID, public, peerstore.TempAddrTTL)
		}
	}
	return h.Host.Connect(ctx, pi)
}

// setupBitswapExchange wires bitswap onto h, the main libp2p host. In the
// split-host setup (dhtAddrs non-nil), h is wrapped so each bitswap Connect
// copies DHT-known public addresses into the peerstore before dialing.
func setupBitswapExchange(ctx context.Context, cfg Config, h host.Host, dhtAddrs peerstore.AddrBook, cr routing.ContentRouting, bstore blockstore.Blockstore) exchange.Interface {
	bsctx := metri.CtxScope(ctx, "ipfs_bitswap")

	connEvtMgr := network.NewConnectEventManager()

	bitswapHost := h
	if dhtAddrs != nil {
		bitswapHost = &peerstoreMergingHost{Host: h, dhtAddrs: dhtAddrs}
	}

	var exnet network.BitSwapNetwork
	bn := bsnet.NewFromIpfsHost(bitswapHost, bsnet.WithConnectEventManager(connEvtMgr))

	if cfg.HTTPRetrievalEnable {

		htnet := httpnet.New(h,
			httpnet.WithHTTPWorkers(cfg.HTTPRetrievalWorkers),
			httpnet.WithMaxDontHaveErrors(cfg.HTTPRetrievalMaxDontHaveErrors),
			httpnet.WithAllowlist(cfg.HTTPRetrievalAllowlist),
			httpnet.WithDenylist(cfg.HTTPRetrievalDenylist),
			httpnet.WithUserAgent("rainbow/"+buildVersion()),
			httpnet.WithMetricsLabelsForEndpoints(cfg.HTTPRetrievalMetricsLabelsForEndpoints),
			httpnet.WithConnectEventManager(connEvtMgr),
		)
		exnet = network.New(h.Peerstore(), bn, htnet)
	} else {
		exnet = bn
	}

	// Custom query manager with the content router and the host
	// and our custom options to overwrite the default.
	pqm, err := providerquerymanager.New(exnet, cr,
		providerquerymanager.WithMaxInProcessRequests(cfg.RoutingMaxRequests),
		providerquerymanager.WithMaxProviders(cfg.RoutingMaxProviders),
		providerquerymanager.WithMaxTimeout(cfg.RoutingMaxTimeout),
		providerquerymanager.WithIgnoreProviders(cfg.RoutingIgnoreProviders...),
	)
	if err != nil {
		panic(err)
	}
	context.AfterFunc(ctx, func() {
		pqm.Close()
	})

	// --- Client Options
	// bitswap.RebroadcastDelay: default is 1 minute to search for a random
	// live-want (1 CID).  I think we want to search for random live-wants more
	// often although probably it overlaps with general rebroadcasts.
	const rebroadcastDelay = 10 * time.Second
	// bitswap.ProviderSearchDelay: default is 1 second.
	const providerSearchDelay = 1 * time.Second

	// --- Bitswap Client Options
	clientOpts := []bsclient.Option{
		bsclient.RebroadcastDelay(rebroadcastDelay),
		bsclient.ProviderSearchDelay(providerSearchDelay),
		bsclient.WithDefaultProviderQueryManager(false), // we pass it in manually
	}

	if !cfg.BitswapEnableDuplicateBlockStats {
		clientOpts = append(clientOpts, bsclient.WithoutDuplicatedBlockStats())
	}

	// If peering and shared cache are both enabled, we initialize both a
	// Client and a Server with custom request filter and custom options.
	// client+server is more expensive but necessary when deployment requires
	// serving cached blocks to safelisted peerids
	if cfg.PeeringSharedCache && len(cfg.Peering) > 0 {
		var peerBlockRequestFilter bsserver.PeerBlockRequestFilter

		// Set up request filter to only respond to request for safelisted (peered) nodes
		peers := make(map[peer.ID]struct{}, len(cfg.Peering))
		for _, a := range cfg.Peering {
			peers[a.ID] = struct{}{}
		}
		peerBlockRequestFilter = func(p peer.ID, c cid.Cid) bool {
			_, ok := peers[p]
			return ok
		}

		// turn bitswap clients option into bitswap options
		var opts []bitswap.Option
		for _, o := range clientOpts {
			opts = append(opts, bitswap.WithClientOption(o))
		}

		// ---- Server Options
		opts = append(opts,
			bitswap.WithPeerBlockRequestFilter(peerBlockRequestFilter),
			// When we don't have a block, don't reply. This reduces processment.
			bitswap.SetSendDontHaves(false),
			bitswap.WithWantHaveReplaceSize(cfg.BitswapWantHaveReplaceSize),
		)

		// Initialize client+server
		bswap := bitswap.New(bsctx, exnet, pqm, bstore, opts...)
		exnet.Start(bswap)
		return &noNotifyExchange{bswap}
	}

	// By default, rainbow runs with bitswap client alone
	bswap := bsclient.New(bsctx, exnet, pqm, bstore, clientOpts...)
	exnet.Start(bswap)
	return bswap
}

type noNotifyExchange struct {
	exchange.Interface
}

func (e *noNotifyExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	// Rainbow does not notify when we get new blocks in our Blockservice.
	return nil
}
