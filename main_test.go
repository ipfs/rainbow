package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http/httptest"
	"testing"

	chunker "github.com/ipfs/boxo/chunker"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs/importer/balanced"
	uih "github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	"github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustTestPeer(t *testing.T) (ic.PrivKey, peer.ID) {
	sk, _, err := ic.GenerateKeyPairWithReader(ic.Ed25519, 2048, rand.Reader)
	require.NoError(t, err)

	pid, err := peer.IDFromPrivateKey(sk)
	require.NoError(t, err)

	return sk, pid
}

func mustTestPeerFromSeed(t *testing.T, seed string, index int) (ic.PrivKey, peer.ID) {
	sk, err := deriveKey(seed, deriveKeyInfo(index))
	require.NoError(t, err)

	pid, err := peer.IDFromPrivateKey(sk)
	require.NoError(t, err)

	return sk, pid
}

func mustTestNode(t *testing.T, cfg Config) *Node {
	sk, _ := mustTestPeer(t)
	return mustTestNodeWithKey(t, cfg, sk)
}

func mustTestNodeWithKey(t *testing.T, cfg Config, sk ic.PrivKey) *Node {
	// Set necessary fields if not defined.
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.BlockstoreType == "" {
		cfg.BlockstoreType = "flatfs"
	}
	if cfg.DHTRouting == "" {
		cfg.DHTRouting = DHTOff
	}

	ctx := context.Background()
	cdns := newCachedDNS(dnsCacheRefreshInterval)

	t.Cleanup(func() {
		_ = cdns.Close()
	})

	nd, err := Setup(ctx, cfg, sk, cdns)
	require.NoError(t, err)
	return nd
}

func mustTestServer(t *testing.T, cfg Config) (*httptest.Server, *Node) {
	nd := mustTestNode(t, cfg)

	handler, err := setupGatewayHandler(cfg, nd)
	if err != nil {
		require.NoError(t, err)
	}

	ts := httptest.NewServer(handler)
	return ts, nd
}

func mustAddFile(t *testing.T, gnd *Node, content []byte) cid.Cid {
	dsrv := merkledag.NewDAGService(gnd.bsrv)

	// Create a UnixFS graph from our file, parameters described here but can be visualized at https://dag.ipfs.tech/
	ufsImportParams := uih.DagBuilderParams{
		Maxlinks:  uih.DefaultLinksPerBlock, // Default max of 174 links per block
		RawLeaves: true,                     // Leave the actual file bytes untouched instead of wrapping them in a dag-pb protobuf wrapper
		CidBuilder: cid.V1Builder{ // Use CIDv1 for all links
			Codec:    uint64(multicodec.DagPb),
			MhType:   uint64(multicodec.Sha2_256), // Use SHA2-256 as the hash function
			MhLength: -1,                          // Use the default hash length for the given hash function (in this case 256 bits)
		},
		Dagserv: dsrv,
		NoCopy:  false,
	}
	ufsBuilder, err := ufsImportParams.New(chunker.NewSizeSplitter(bytes.NewReader(content), chunker.DefaultBlockSize)) // Split the file up into fixed sized 256KiB chunks
	require.NoError(t, err)

	nd, err := balanced.Layout(ufsBuilder) // Arrange the graph with a balanced layout
	require.NoError(t, err)

	return nd.Cid()
}

func TestReplaceRainbowSeedWithPeer(t *testing.T) {
	t.Parallel()

	seed, err := newSeed()
	require.NoError(t, err)

	_, pid0 := mustTestPeerFromSeed(t, seed, 0)
	_, pid1 := mustTestPeerFromSeed(t, seed, 1)
	_, pid42 := mustTestPeerFromSeed(t, seed, 42)

	testCases := []struct {
		input  string
		output string
	}{
		{"/dns/example/tcp/4001", "/dns/example/tcp/4001"},
		{"/dns/example/tcp/4001/p2p/" + pid0.String(), "/dns/example/tcp/4001/p2p/" + pid0.String()},
		{"/dns/example/tcp/4001/p2p/rainbow-seed/0", "/dns/example/tcp/4001/p2p/" + pid0.String()},
		{"/dns/example/tcp/4001/p2p/rainbow-seed/1", "/dns/example/tcp/4001/p2p/" + pid1.String()},
		{"/dns/example/tcp/4001/p2p/rainbow-seed/42", "/dns/example/tcp/4001/p2p/" + pid42.String()},
	}

	for _, tc := range testCases {
		res, err := replaceRainbowSeedWithPeer(tc.input, seed)
		assert.NoError(t, err)
		assert.Equal(t, tc.output, res)
	}
}
