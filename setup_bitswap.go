package main

import (
	"context"
	"github.com/ipfs/boxo/routing/providerquerymanager"
	"github.com/libp2p/go-libp2p/core/peerstore"
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
	n := &providerQueryNetwork{cr, h}
	pqm, err := providerquerymanager.New(ctx, n, providerquerymanager.WithMaxInProcessRequests(100))
	if err != nil {
		panic(err)
	}
	cr = &wrapProv{pqm: pqm}
	bn := bsnet.NewFromIpfsHost(h, cr)

	// --- Client Options
	// bitswap.RebroadcastDelay: default is 1 minute to search for a random
	// live-want (1 CID).  I think we want to search for random live-wants more
	// often although probably it overlaps with general rebroadcasts.
	rebroadcastDelay := delay.Fixed(10 * time.Second)
	// bitswap.ProviderSearchDelay: default is 1 second.
	providerSearchDelay := 1 * time.Second

	// --- Bitswap Client Options
	clientOpts := []bsclient.Option{
		bsclient.RebroadcastDelay(rebroadcastDelay),
		bsclient.ProviderSearchDelay(providerSearchDelay),
		bsclient.WithoutDuplicatedBlockStats(),
		bsclient.WithDefaultLookupManagement(false),
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
			bitswap.ProvideEnabled(false),
			// Do not keep track of other peer's wantlists, we only want to reply if we
			// have a block. If we get it later, it's no longer relevant.
			bitswap.WithPeerLedger(&noopPeerLedger{}),
			// When we don't have a block, don't reply. This reduces processment.
			bitswap.SetSendDontHaves(false))

		// Initialize client+server
		bswap := bitswap.New(bsctx, bn, bstore, opts...)
		bn.Start(bswap)
		return &noNotifyExchange{bswap}
	}

	// By default, rainbow runs with bitswap client alone
	bswap := bsclient.New(bsctx, bn, bstore, clientOpts...)
	bn.Start(bswap)
	return bswap
}

type providerQueryNetwork struct {
	routing.ContentRouting
	host.Host
}

func (p *providerQueryNetwork) ConnectTo(ctx context.Context, id peer.ID) error {
	return p.Host.Connect(ctx, peer.AddrInfo{ID: id})
}

func (p *providerQueryNetwork) FindProvidersAsync(ctx context.Context, c cid.Cid, i int) <-chan peer.ID {
	out := make(chan peer.ID, i)
	go func() {
		defer close(out)
		providers := p.ContentRouting.FindProvidersAsync(ctx, c, i)
		for info := range providers {
			if info.ID == p.Host.ID() {
				continue // ignore self as provider
			}
			p.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.TempAddrTTL)
			select {
			case <-ctx.Done():
				return
			case out <- info.ID:
			}
		}
	}()
	return out
}

type wrapProv struct {
	pqm *providerquerymanager.ProviderQueryManager
}

var _ routing.ContentRouting = (*wrapProv)(nil)

func (r *wrapProv) Provide(ctx context.Context, c cid.Cid, b bool) error {
	return routing.ErrNotSupported
}

func (r *wrapProv) FindProvidersAsync(ctx context.Context, c cid.Cid, _ int) <-chan peer.AddrInfo {
	retCh := make(chan peer.AddrInfo)
	go func() {
		defer close(retCh)
		provsCh := r.pqm.FindProvidersAsync(ctx, c)
		for p := range provsCh {
			select {
			case retCh <- peer.AddrInfo{ID: p}:
			case <-ctx.Done():
			}
		}
	}()
	return retCh
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
