package main

import (
	"context"
	"time"

	"github.com/ipfs/boxo/routing/providerquerymanager"

	"github.com/ipfs/boxo/bitswap"
	bsclient "github.com/ipfs/boxo/bitswap/client"
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

const perBlockTimeout = time.Second * 10

func setupBitswapExchange(ctx context.Context, cfg Config, h host.Host, cr routing.ContentRouting, bstore blockstore.Blockstore) exchange.Interface {
	bsctx := metri.CtxScope(ctx, "ipfs_bitswap")

	bn := bsnet.NewFromIpfsHost(h)

	// Custom query manager with the content router and the host
	// and our custom options to overwrite the default.
	pqm, err := providerquerymanager.New(ctx, h, cr,
		providerquerymanager.WithMaxInProcessRequests(cfg.RoutingMaxRequests),
		providerquerymanager.WithMaxProviders(cfg.RoutingMaxProviders),
		providerquerymanager.WithMaxTimeout(cfg.RoutingMaxTimeout),
	)
	if err != nil {
		panic(err)
	}
	pqm.Startup()

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
		bsclient.WithDefaultProviderQueryManager(false), // we pass it in manually
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
		bswap := bitswap.New(bsctx, bn, pqm, bstore, opts...)
		bn.Start(bswap)
		return &noNotifyExchange{bswap}
	}

	// By default, rainbow runs with bitswap client alone
	bswap := bsclient.New(bsctx, bn, pqm, bstore, clientOpts...)
	bn.Start(bswap)

	wrapExch := &timeoutBlockExchange{
		inner:           bswap,
		perBlockTimeout: perBlockTimeout,
	}

	return wrapExch
}

type noNotifyExchange struct {
	exchange.Interface
}

func (e *noNotifyExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	// Rainbow does not notify when we get new blocks in our Blockservice.
	return nil
}

type timeoutBlockExchange struct {
	inner           exchange.Interface
	perBlockTimeout time.Duration
}

func (t *timeoutBlockExchange) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	ctx, cancel := context.WithTimeout(ctx, t.perBlockTimeout)
	defer cancel()
	return t.inner.GetBlock(ctx, c)
}

func (t *timeoutBlockExchange) GetBlocks(ctx context.Context, cids []cid.Cid) (<-chan blocks.Block, error) {
	ctx, cancel := context.WithCancel(ctx)
	blocksCh, err := t.inner.GetBlocks(ctx, cids)
	if err != nil {
		cancel()
		return nil, err
	}

	retCh := make(chan blocks.Block)
	go func() {
		defer close(retCh)
		defer cancel()
		timer := time.NewTimer(t.perBlockTimeout)
		for b := range blocksCh {
			select {
			case retCh <- b:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(t.perBlockTimeout)
			case <-timer.C:
				return
			}
		}
		if !timer.Stop() {
			<-timer.C
		}
	}()

	return retCh, nil
}

func (t *timeoutBlockExchange) NotifyNewBlocks(ctx context.Context, blocks ...blocks.Block) error {
	return t.inner.NotifyNewBlocks(ctx, blocks...)
}

func (t *timeoutBlockExchange) Close() error {
	return t.inner.Close()
}

var _ exchange.Interface = (*timeoutBlockExchange)(nil)
