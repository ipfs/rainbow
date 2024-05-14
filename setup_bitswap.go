package main

import (
	"context"
	"time"

	"github.com/ipfs/boxo/bitswap"
	bsclient "github.com/ipfs/boxo/bitswap/client"
	wl "github.com/ipfs/boxo/bitswap/client/wantlist"
	bsmspb "github.com/ipfs/boxo/bitswap/message/pb"
	bsnet "github.com/ipfs/boxo/bitswap/network"
	bsserver "github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	metri "github.com/ipfs/go-metrics-interface"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
)

func setupBitswapExchange(ctx context.Context, cfg Config, h host.Host, cr routing.ContentRouting, bstore blockstore.Blockstore) exchange.Interface {
	bsctx := metri.CtxScope(ctx, "ipfs_bitswap")
	bn := bsnet.NewFromIpfsHost(h, cr)

	// --- Client Options
	// bitswap.RebroadcastDelay: default is 1 minute to search for a random
	// live-want (1 CID).  I think we want to search for random live-wants more
	// often although probably it overlaps with general rebroadcasts.
	rebroadcastDelay := delay.Fixed(10 * time.Second)
	// bitswap.ProviderSearchDelay: default is 1 second.
	providerSearchDelay := 1 * time.Second

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

		// Initialize client+server
		bswap := bitswap.New(bsctx, bn, bstore,
			// --- Client Options
			bitswap.RebroadcastDelay(rebroadcastDelay),
			bitswap.ProviderSearchDelay(providerSearchDelay),
			bitswap.WithoutDuplicatedBlockStats(),

			// ---- Server Options
			bitswap.WithPeerBlockRequestFilter(peerBlockRequestFilter),
			bitswap.ProvideEnabled(false),
			// Do not keep track of other peer's wantlists, we only want to reply if we
			// have a block. If we get it later, it's no longer relevant.
			bitswap.WithPeerLedger(&noopPeerLedger{}),
			// When we don't have a block, don't reply. This reduces processment.
			bitswap.SetSendDontHaves(false),
		)
		bn.Start(bswap)
		return &noNotifyExchange{bswap}
	}

	// By default, rainbow runs with bitswap client alone
	bswap := bsclient.New(bsctx, bn, bstore,
		// --- Client Options
		bsclient.RebroadcastDelay(rebroadcastDelay),
		bsclient.ProviderSearchDelay(providerSearchDelay),
		bsclient.WithoutDuplicatedBlockStats(),
	)
	bn.Start(bswap)
	return bswap
}

type noopPeerLedger struct{}

func (*noopPeerLedger) Wants(p peer.ID, e wl.Entry) {}

func (*noopPeerLedger) CancelWant(p peer.ID, k cid.Cid) bool {
	return false
}

func (*noopPeerLedger) CancelWantWithType(p peer.ID, k cid.Cid, typ bsmspb.Message_Wantlist_WantType) {
}

func (*noopPeerLedger) Peers(k cid.Cid) []bsserver.PeerEntry {
	return nil
}

func (*noopPeerLedger) CollectPeerIDs() []peer.ID {
	return nil
}

func (*noopPeerLedger) WantlistSizeForPeer(p peer.ID) int {
	return 0
}

func (*noopPeerLedger) WantlistForPeer(p peer.ID) []wl.Entry {
	return nil
}

func (*noopPeerLedger) ClearPeerWantlist(p peer.ID) {}

func (*noopPeerLedger) PeerDisconnected(p peer.ID) {}

type noNotifyExchange struct {
	exchange.Interface
}

func (e *noNotifyExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	// Rainbow does not notify when we get new blocks in our Blockservice.
	return nil
}
