package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/ipfs/go-bitswap"
	bsnet "github.com/ipfs/go-bitswap/network"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	levelds "github.com/ipfs/go-ds-leveldb"
	"github.com/ipfs/go-fetcher"
	bsfetcher "github.com/ipfs/go-fetcher/impl/blockservice"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-ipfs-provider/batched"
	logging "github.com/ipfs/go-log"
	metri "github.com/ipfs/go-metrics-interface"
	mprome "github.com/ipfs/go-metrics-prometheus"
	"github.com/ipfs/go-namesys"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/fullrt"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
)

var log = logging.Logger("rainbow")

var bootstrappers = []string{
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
}

var BootstrapPeers []peer.AddrInfo

func init() {
	if err := mprome.Inject(); err != nil {
		panic(err)
	}

	for _, bsp := range bootstrappers {
		ma, err := multiaddr.NewMultiaddr(bsp)
		if err != nil {
			log.Errorf("failed to parse bootstrap address: ", err)
			continue
		}

		ai, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			log.Errorf("failed to create address info: ", err)
			continue
		}

		BootstrapPeers = append(BootstrapPeers, *ai)
	}
}

type Node struct {
	Dht      *dht.IpfsDHT
	Provider *batched.BatchProvidingSystem
	FullRT   *fullrt.FullRT
	Host     host.Host

	Datastore    datastore.Batching
	Blockstore   blockstore.Blockstore
	Bitswap      *bitswap.Bitswap
	Blockservice blockservice.BlockService

	Namesys namesys.NameSystem

	Bwc *metrics.BandwidthCounter

	unixFSFetcherFactory fetcher.Factory

	mux *http.ServeMux
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

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(connmgr.NewConnManager(cfg.ConnMgrLow, cfg.ConnMgrHi, cfg.ConnMgrGrace)),
		libp2p.Identity(peerkey),
		libp2p.BandwidthReporter(bwc),
		libp2p.DefaultTransports,
		libp2p.Transport(libp2pquic.NewTransport),
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

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	dhtopts := fullrt.DHTOption(
		//dht.Validator(in.Validator),
		dht.Datastore(ds),
		dht.BootstrapPeers(BootstrapPeers...),
		dht.BucketSize(20),
	)

	frt, err := fullrt.NewFullRT(h, dht.DefaultPrefix, dhtopts)
	if err != nil {
		return nil, xerrors.Errorf("constructing fullrt: %w", err)
	}

	ipfsdht, err := dht.New(ctx, h, dht.Datastore(ds))
	if err != nil {
		return nil, xerrors.Errorf("constructing dht: %w", err)
	}

	blkst, err := loadBlockstore("flatfs", cfg.Blockstore)
	if err != nil {
		return nil, err
	}

	bsctx := metri.CtxScope(ctx, "rainbow")

	// TODO: Ideally we could configure bitswap to not call provide on blocks
	// it has fetched. The gateways job is not to reprovide content, just to
	// serve it over http
	bsnet := bsnet.NewFromIpfsHost(h, frt)

	bswap := bitswap.New(bsctx, bsnet, blkst,
		bitswap.EngineBlockstoreWorkerCount(600),
		bitswap.TaskWorkerCount(600),
		bitswap.MaxOutstandingBytesPerPeer(5<<20),
	)

	nsys, err := namesys.NewNameSystem(frt)
	if err != nil {
		return nil, err
	}

	bserv := blockservice.New(blkst, bswap)
	ff := bsfetcher.NewFetcherConfig(bserv)

	return &Node{
		Dht:                  ipfsdht,
		FullRT:               frt,
		Host:                 h,
		Blockstore:           blkst,
		Datastore:            ds,
		Bitswap:              bswap.(*bitswap.Bitswap),
		Namesys:              nsys,
		Blockservice:         bserv,
		Bwc:                  bwc,
		unixFSFetcherFactory: ff,
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

		return blockstore.NewBlockstoreNoPrefix(ds), nil
	default:
		return nil, fmt.Errorf("unsupported blockstore type: %s", spec)
	}
}

func loadOrInitPeerKey(kf string) (crypto.PrivKey, error) {
	data, err := ioutil.ReadFile(kf)
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

		if err := ioutil.WriteFile(kf, data, 0600); err != nil {
			return nil, err
		}

		return k, nil
	}
	return crypto.UnmarshalPrivateKey(data)
}
