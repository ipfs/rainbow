package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chunker "github.com/ipfs/boxo/chunker"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs/importer/balanced"
	uih "github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	util "github.com/ipfs/boxo/util"
	"github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustTestNode(t *testing.T, cfg Config) *Node {
	cfg.DataDir = t.TempDir()
	cfg.BlockstoreType = "flatfs"
	cfg.DHTRouting = DHTStandard
	cfg.RoutingV1Endpoints = []string{cidContactEndpoint}

	ctx := context.Background()

	sr := util.NewTimeSeededRand()
	sk, _, err := ic.GenerateKeyPairWithReader(ic.Ed25519, 2048, sr)
	require.NoError(t, err)

	cdns := newCachedDNS(dnsCacheRefreshInterval)

	t.Cleanup(func() {
		_ = cdns.Close()
	})

	gnd, err := Setup(ctx, cfg, sk, cdns)
	require.NoError(t, err)
	return gnd
}

func mustTestServer(t *testing.T, cfg Config) (*httptest.Server, *Node) {
	gnd := mustTestNode(t, cfg)

	handler, err := setupGatewayHandler(cfg, gnd)
	if err != nil {
		require.NoError(t, err)
	}

	ts := httptest.NewServer(handler)

	return ts, gnd
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

func TestTrustless(t *testing.T) {
	t.Parallel()

	ts, gnd := mustTestServer(t, Config{
		TrustlessGatewayDomains: []string{"trustless.com"},
	})

	content := "hello world"
	cid := mustAddFile(t, gnd, []byte(content))
	url := ts.URL + "/ipfs/" + cid.String()

	t.Run("Non-trustless request returns 406", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Host = "trustless.com"

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotAcceptable, res.StatusCode)
	})

	t.Run("Trustless request with query parameter returns 200", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url+"?format=raw", nil)
		require.NoError(t, err)
		req.Host = "trustless.com"

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Trustless request with accept header returns 200", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Host = "trustless.com"
		req.Header.Set("Accept", "application/vnd.ipld.raw")

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}
