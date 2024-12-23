package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ipfs/boxo/gateway"
	blocks "github.com/ipfs/go-block-format"
	ci "github.com/libp2p/go-libp2p-testing/ci"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func mustFreePort(t *testing.T) (int, *net.TCPListener) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)

	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)

	return l.Addr().(*net.TCPAddr).Port, l
}

func mustFreePorts(t *testing.T, n int) []int {
	ports := make([]int, 0)
	for i := 0; i < n; i++ {
		port, listener := mustFreePort(t)
		defer listener.Close()
		ports = append(ports, port)
	}

	return ports
}

func mustListenAddrWithPort(t *testing.T, port int) multiaddr.Multiaddr {
	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	require.NoError(t, err)
	return ma
}

// mustPeeredNodes creates a network of [Node]s with the given configuration.
// The configuration contains as many elements as there are nodes. Each element
// indicates to which other nodes it is connected.
//
//	Example configuration: [][]int{
//	 {1, 2},
//	 {0},
//	 {0},
//	}
//
// - Node 0 is connected to nodes 1 and 2.
// - Node 1 is connected to node 0.
// - Node 2 is connected to node 1.
func mustPeeredNodes(t *testing.T, configuration [][]int, peeringShareCache bool) []*Node {
	n := len(configuration)

	// Generate ports, secrets keys, peer IDs and multiaddresses.
	ports := mustFreePorts(t, n)
	keys := make([]ic.PrivKey, n)
	pids := make([]peer.ID, n)
	mas := make([]multiaddr.Multiaddr, n)
	addrInfos := make([]peer.AddrInfo, n)

	for i := 0; i < n; i++ {
		keys[i], pids[i] = mustTestPeer(t)
		mas[i] = mustListenAddrWithPort(t, ports[i])
		addrInfos[i] = peer.AddrInfo{
			ID:    pids[i],
			Addrs: []multiaddr.Multiaddr{mas[i]},
		}
	}

	cfgs := make([]Config, n)
	nodes := make([]*Node, n)
	for i := 0; i < n; i++ {
		cfgs[i] = Config{
			DHTRouting:         DHTOff,
			RoutingV1Endpoints: []string{},
			ListenAddrs:        []string{mas[i].String()},
			Peering:            []peer.AddrInfo{},
			PeeringSharedCache: peeringShareCache,
			Bitswap:            true,
		}

		for _, j := range configuration[i] {
			cfgs[i].Peering = append(cfgs[i].Peering, addrInfos[j])
		}

		nodes[i] = mustTestNodeWithKey(t, cfgs[i], keys[i])

		t.Log("Node", i, "Addresses", nodes[i].host.Addrs(), "Peering", cfgs[i].Peering)
	}

	require.Eventually(t, func() bool {
		for i, node := range nodes {
			for _, peer := range cfgs[i].Peering {
				if node.host.Network().Connectedness(peer.ID) != network.Connected {
					return false
				}
			}
		}

		return true
	}, time.Second*30, time.Millisecond*100)

	return nodes
}

func TestPeering(t *testing.T) {
	_ = mustPeeredNodes(t, [][]int{
		{1, 2},
		{0, 2},
		{0, 1},
	}, false)
}

func TestPeeringSharedCache(t *testing.T) {
	nodes := mustPeeredNodes(t, [][]int{
		{1}, // 0 peered to 1
		{0}, // 1 peered to 0
		{},  // 2 not peered to anyone
	}, true)

	bl := blocks.NewBlock([]byte(string("peering-cache-test")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checkBitswap := func(i int, success bool) {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()

		_, err := nodes[i].bsrv.GetBlock(ctx, bl.Cid())
		if success {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}

	err := nodes[0].bsrv.AddBlock(ctx, bl)
	require.NoError(t, err)

	// confirm peering enables cache sharing, and bitswap retrieval from safe-listed node works
	checkBitswap(1, true)
	// confirm bitswap providing is disabled by default (no peering)
	checkBitswap(2, false)
}

func testSeedPeering(t *testing.T, n int, dhtRouting DHTRouting, dhtSharedHost bool) ([]ic.PrivKey, []peer.ID, []*Node) {
	cdns := newCachedDNS(dnsCacheRefreshInterval)
	defer cdns.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed, err := newSeed()
	require.NoError(t, err)

	keys := make([]ic.PrivKey, n)
	pids := make([]peer.ID, n)

	for i := 0; i < n; i++ {
		keys[i], pids[i] = mustTestPeerFromSeed(t, seed, i)
	}

	cfgs := make([]Config, n)
	nodes := make([]*Node, n)

	for i := 0; i < n; i++ {
		dnslinkResolver, err := gateway.NewDNSResolver(nil)
		require.NoError(t, err)
		cfgs[i] = Config{
			DataDir:             t.TempDir(),
			BlockstoreType:      "flatfs",
			DHTRouting:          dhtRouting,
			DHTSharedHost:       dhtSharedHost,
			Bitswap:             true,
			Seed:                seed,
			SeedIndex:           i,
			SeedPeering:         true,
			SeedPeeringMaxIndex: n,
			DNSLinkResolver:     dnslinkResolver,
		}

		nodes[i], err = SetupWithLibp2p(ctx, cfgs[i], keys[i], cdns)
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		for i, node := range nodes {
			for j, pid := range pids {
				if i == j {
					continue
				}

				if node.host.Network().Connectedness(pid) != network.Connected {
					return false
				}
			}
		}

		return true
	}, time.Second*120, time.Millisecond*100)

	return keys, pids, nodes
}

func TestSeedPeering(t *testing.T) {
	if ci.IsRunning() {
		t.Skip("don't run seed peering tests in ci")
	}

	t.Run("DHT disabled", func(t *testing.T) {
		testSeedPeering(t, 3, DHTOff, false)
	})

	t.Run("DHT enabled with shared host disabled", func(t *testing.T) {
		testSeedPeering(t, 3, DHTStandard, false)
	})

	t.Run("DHT enabled with shared host enabled", func(t *testing.T) {
		testSeedPeering(t, 3, DHTStandard, true)
	})
}
